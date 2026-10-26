#!/usr/bin/env bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="${OPENSEARCH_RECOVERY_NAMESPACE:-opensearch}"
STATEFULSET="${OPENSEARCH_RECOVERY_STATEFULSET:-opensearch-cluster-single}"
POD="${STATEFULSET}-0"
JAEGER_NAMESPACE="${OPENSEARCH_RECOVERY_JAEGER_NAMESPACE:-jaeger}"
JAEGER_SELECTOR="${OPENSEARCH_RECOVERY_JAEGER_SELECTOR:-app.kubernetes.io/instance=jaeger,app.kubernetes.io/component=all-in-one}"
SNAPSHOT_CLASS="${OPENSEARCH_RECOVERY_SNAPSHOT_CLASS:-jaeger-opensearch-oci-retain}"
SNAPSHOT_CLASS_FILE="$SCRIPT_DIR/opensearch-volume-snapshot-class.yaml"
MAX_AGE_SECONDS="${OPENSEARCH_RECOVERY_MAX_AGE_SECONDS:-259200}"
TIMEOUT_SECONDS="${OPENSEARCH_RECOVERY_TIMEOUT_SECONDS:-3600}"
RECOVERY_ID="${OPENSEARCH_RECOVERY_ID:-${GITHUB_RUN_ID:-manual}-${GITHUB_RUN_ATTEMPT:-1}}"
DATA_PATH="/usr/share/opensearch/data"
ACTIVE_RESTORE_POD=""

log() { echo "[$(date -u +"%F %T UTC")] $*"; }
err() { echo "[$(date -u +"%F %T UTC")] ERROR: $*" >&2; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || err "$1 is required but not installed"
}

validate_recovery_id() {
  [[ "$RECOVERY_ID" =~ ^[a-z0-9]([a-z0-9-]{0,28}[a-z0-9])?$ ]] ||
    err "OPENSEARCH_RECOVERY_ID must be 1-30 lowercase letters, digits, or hyphens"
}

require_snapshot_api() {
  kubectl api-resources --api-group=snapshot.storage.k8s.io -o name |
    grep -qx 'volumesnapshots.snapshot.storage.k8s.io' ||
    err "VolumeSnapshot API is unavailable; do not install unpinned snapshot CRDs"
  kubectl api-resources --api-group=snapshot.storage.k8s.io -o name |
    grep -qx 'volumesnapshotclasses.snapshot.storage.k8s.io' ||
    err "VolumeSnapshotClass API is unavailable; do not install unpinned snapshot CRDs"
}

get_pod_json() {
  kubectl -n "$NAMESPACE" get pod "$POD" -o json
}

get_source_pvc() {
  local pod_json=$1
  local data_volume

  data_volume=$(jq -r --arg path "$DATA_PATH" '
    .spec.containers[]
    | select(.name == "opensearch")
    | .volumeMounts[]
    | select(.mountPath == $path)
    | .name
  ' <<<"$pod_json")
  [[ -n "$data_volume" && "$data_volume" != "null" ]] || err "Cannot resolve the OpenSearch data volume"

  jq -r --arg volume "$data_volume" '
    .spec.volumes[]
    | select(.name == $volume)
    | .persistentVolumeClaim.claimName
  ' <<<"$pod_json"
}

get_live_image() {
  local pod_json=$1
  local image

  image=$(jq -r '.status.containerStatuses[] | select(.name == "opensearch") | .imageID // empty' <<<"$pod_json")
  image=${image#docker-pullable://}
  printf '%s\n' "$image"
}

get_live_jaeger_image() {
  local pods_json=$1
  local image

  image=$(jq -r '
    [.items[]
      | select(.status.phase == "Running")
      | .status.containerStatuses[]
      | select(.name == "jaeger")
      | .imageID // empty]
    | first // empty
  ' <<<"$pods_json")
  image=${image#docker-pullable://}
  printf '%s\n' "$image"
}

opensearch_api() {
  local method=$1
  local path=$2
  kubectl -n "$NAMESPACE" exec "$POD" -c opensearch --
    curl --fail --silent --show-error -X "$method" "http://127.0.0.1:9200/$path"
}

get_live_version() {
  opensearch_api GET '' | jq -r '.version.number'
}

ensure_snapshot_class() {
  local driver
  local deletion_policy
  local backup_type

  if kubectl get volumesnapshotclass "$SNAPSHOT_CLASS" >/dev/null 2>&1; then
    driver=$(kubectl get volumesnapshotclass "$SNAPSHOT_CLASS" -o jsonpath='{.driver}')
    deletion_policy=$(kubectl get volumesnapshotclass "$SNAPSHOT_CLASS" -o jsonpath='{.deletionPolicy}')
    backup_type=$(kubectl get volumesnapshotclass "$SNAPSHOT_CLASS" -o jsonpath='{.parameters.backupType}')
    [[ "$driver" == "blockvolume.csi.oraclecloud.com" ]] ||
      err "Existing VolumeSnapshotClass $SNAPSHOT_CLASS uses unexpected driver $driver"
    [[ "$deletion_policy" == "Retain" ]] ||
      err "Existing VolumeSnapshotClass $SNAPSHOT_CLASS does not retain backups"
    [[ "$backup_type" == "full" ]] ||
      err "Existing VolumeSnapshotClass $SNAPSHOT_CLASS does not set parameters.backupType=full; create a new correctly configured class"
    return
  fi

  kubectl create -f "$SNAPSHOT_CLASS_FILE"
}

wait_for_jsonpath() {
  local namespace=$1
  local resource=$2
  local jsonpath=$3
  local expected=$4
  local elapsed=0
  local value

  while ((elapsed < TIMEOUT_SECONDS)); do
    value=$(kubectl -n "$namespace" get "$resource" -o "jsonpath=$jsonpath" 2>/dev/null || true)
    if [[ "$value" == "$expected" ]]; then
      return 0
    fi
    sleep 10
    elapsed=$((elapsed + 10))
  done

  err "$resource did not reach $jsonpath=$expected within ${TIMEOUT_SECONDS}s"
}

live_inventory_exact() {
  local path=$1
  opensearch_api GET "$path" | awk 'NF' | sort
}

live_document_count() {
  local pattern=$1
  opensearch_api GET "_cat/indices/${pattern}?h=docs.count" |
    awk '$1 ~ /^[0-9]+$/ {sum += $1} END {printf "%.0f\n", sum + 0}'
}

live_representative_field() {
  local pattern=$1
  local field=$2
  local sort=${3:-_doc}

  opensearch_api GET "${pattern}/_search?size=1&_source=${field}&sort=${sort}" |
    jq -r --arg field "$field" '.hits.hits[0]._source | getpath($field | split(".")) // empty'
}

render_snapshot() {
  local snapshot=$1
  local source_pvc=$2
  local source_version=$3
  local source_image=$4

  cat <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: $snapshot
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/part-of: jaeger-demo
    jaegertracing.io/recovery-source: opensearch
  annotations:
    jaegertracing.io/source-version: "$source_version"
    jaegertracing.io/source-image-digest: "$source_image"
    jaegertracing.io/restore-tested: "false"
spec:
  volumeSnapshotClassName: $SNAPSHOT_CLASS
  source:
    persistentVolumeClaimName: $source_pvc
EOF
}

render_restore_pvc() {
  local pvc=$1
  local snapshot=$2
  local storage_class=$3
  local capacity=$4

  cat <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: $pvc
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/part-of: jaeger-demo
    jaegertracing.io/recovery-source: opensearch
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: $storage_class
  resources:
    requests:
      storage: $capacity
  dataSource:
    name: $snapshot
    kind: VolumeSnapshot
    apiGroup: snapshot.storage.k8s.io
EOF
}

render_restore_configmap() {
  local configmap=$1

  cat <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: $configmap
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/part-of: jaeger-demo
    jaegertracing.io/recovery-source: opensearch
data:
  jaeger.yaml: |
    service:
      extensions: [jaeger_storage, jaeger_query]
      pipelines:
        traces:
          receivers: [nop]
          exporters: [nop]
    extensions:
      jaeger_query:
        http:
          endpoint: 127.0.0.1:16686
        grpc:
          endpoint: 127.0.0.1:16685
        storage:
          traces: restored
      jaeger_storage:
        backends:
          restored:
            opensearch:
              server_urls:
                - http://127.0.0.1:9200
              indices:
                index_prefix: "jaeger-main"
                spans:
                  date_layout: "2006-01-02"
                  rollover_frequency: "day"
                  shards: 1
                  replicas: 0
                services:
                  date_layout: "2006-01-02"
                  rollover_frequency: "day"
                  shards: 1
                  replicas: 0
                dependencies:
                  date_layout: "2006-01-02"
                  rollover_frequency: "day"
                  shards: 1
                  replicas: 0
                sampling:
                  date_layout: "2006-01-02"
                  rollover_frequency: "day"
                  shards: 1
                  replicas: 0
    receivers:
      nop:
    exporters:
      nop:
EOF
}

render_restore_pod() {
  local pod=$1
  local pvc=$2
  local opensearch_image=$3
  local jaeger_image=$4
  local configmap=$5

  cat <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: $pod
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/part-of: jaeger-demo
    jaegertracing.io/recovery-source: opensearch
spec:
  restartPolicy: OnFailure
  terminationGracePeriodSeconds: 120
  automountServiceAccountToken: false
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              app.kubernetes.io/name: opensearch
              app.kubernetes.io/instance: opensearch
          topologyKey: kubernetes.io/hostname
  securityContext:
    fsGroup: 1000
  containers:
    - name: opensearch
      image: $opensearch_image
      imagePullPolicy: IfNotPresent
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]
        runAsNonRoot: true
        runAsUser: 1000
      resources:
        requests:
          cpu: 1000m
          memory: 6Gi
        limits:
          memory: 6Gi
      env:
        - name: DISABLE_INSTALL_DEMO_CONFIG
          value: "true"
        - name: DISABLE_SECURITY_PLUGIN
          value: "true"
        - name: OPENSEARCH_JAVA_OPTS
          value: "-Xms3g -Xmx3g"
      command: ["bash", "-c"]
      args:
        - |
          exec /usr/share/opensearch/opensearch-docker-entrypoint.sh \
            -Ediscovery.type=single-node \
            -Enetwork.host=127.0.0.1 \
            -Eplugins.security.disabled=true
      volumeMounts:
        - name: data
          mountPath: $DATA_PATH
    - name: jaeger-query
      image: $jaeger_image
      imagePullPolicy: IfNotPresent
      command: ["/cmd/jaeger/jaeger-linux"]
      args: ["--config", "/etc/jaeger/jaeger.yaml"]
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]
        runAsNonRoot: true
        runAsUser: 10001
      resources:
        requests:
          cpu: 250m
          memory: 512Mi
        limits:
          memory: 2Gi
      volumeMounts:
        - name: jaeger-config
          mountPath: /etc/jaeger
          readOnly: true
        - name: tmp
          mountPath: /tmp
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: $pvc
    - name: jaeger-config
      configMap:
        name: $configmap
    - name: tmp
      emptyDir: {}
EOF
}

restored_api() {
  local pod=$1
  local path=$2
  kubectl -n "$NAMESPACE" exec "$pod" -c opensearch --
    curl --fail --silent --show-error "http://127.0.0.1:9200/$path"
}

restored_inventory_exact() {
  local pod=$1
  local path=$2
  restored_api "$pod" "$path" | awk 'NF' | sort
}

restored_document_count() {
  local pod=$1
  local pattern=$2
  restored_api "$pod" "_cat/indices/${pattern}?h=docs.count" |
    awk '$1 ~ /^[0-9]+$/ {sum += $1} END {printf "%.0f\n", sum + 0}'
}

wait_for_restored_opensearch() {
  local pod=$1
  local deadline=$((SECONDS + TIMEOUT_SECONDS))
  local health

  while ((SECONDS < deadline)); do
    health=$(restored_api "$pod" '_cluster/health?wait_for_status=yellow&timeout=20s&filter_path=status' 2>/dev/null || true)
    if jq -e '.status == "yellow" or .status == "green"' >/dev/null 2>&1 <<<"$health"; then
      return 0
    fi
    sleep 10
  done
  err "Restored OpenSearch did not recover its primary shards within ${TIMEOUT_SECONDS}s"
}

jaeger_api() {
  local pod=$1
  local path=$2
  kubectl -n "$NAMESPACE" exec "$pod" -c opensearch --
    curl --fail --silent --show-error "http://127.0.0.1:16686/$path"
}

wait_for_jaeger_service() {
  local pod=$1
  local expected_service=$2
  local elapsed=0
  local response

  while ((elapsed < TIMEOUT_SECONDS)); do
    response=$(jaeger_api "$pod" 'api/v3/services' 2>/dev/null || true)
    if jq -e --arg expected "$expected_service" '.. | strings | select(. == $expected)' >/dev/null 2>&1 <<<"$response"; then
      return 0
    fi
    sleep 10
    elapsed=$((elapsed + 10))
  done
  err "Isolated Jaeger could not query the representative restored service within ${TIMEOUT_SECONDS}s"
}

assert_inventory_contains() {
  local label=$1
  local expected=$2
  local actual=$3
  local item

  while IFS= read -r item; do
    [[ -z "$item" ]] && continue
    grep -Fqx -- "$item" <<<"$actual" ||
      err "Restored $label is missing an item from the live pre-snapshot inventory"
  done <<<"$expected"
}

cleanup_restore_pod() {
  [[ -n "$ACTIVE_RESTORE_POD" ]] || return 0
  kubectl -n "$NAMESPACE" delete pod "$ACTIVE_RESTORE_POD" \
    --ignore-not-found=true --wait=false >/dev/null 2>&1 || true
}

select_recovery_snapshot() {
  local snapshots_json=$1
  local source_version=$2
  local source_image=$3
  local cutoff_epoch=$4

  jq -r --arg version "$source_version" --arg image "$source_image" --argjson cutoff "$cutoff_epoch" '
    [.items[]
      | select(.status.readyToUse == true)
      | select(.metadata.annotations["jaegertracing.io/restore-tested"] == "true")
      | select(.metadata.annotations["jaegertracing.io/source-version"] == $version)
      | select(.metadata.annotations["jaegertracing.io/source-image-digest"] == $image)
      | select((.metadata.creationTimestamp | fromdateiso8601) >= $cutoff)]
    | sort_by(.metadata.creationTimestamp)
    | last
    | .metadata.name // empty
  ' <<<"$snapshots_json"
}

validate_snapshot_content() {
  local content=$1
  local source_version=$2
  local source_image=$3
  local content_json

  content_json=$(kubectl get volumesnapshotcontent "$content" -o json)
  jq -e --arg version "$source_version" --arg image "$source_image" '
    .spec.driver == "blockvolume.csi.oraclecloud.com"
    and .spec.deletionPolicy == "Retain"
    and ((.status.snapshotHandle // .spec.source.snapshotHandle // "") | length > 0)
    and .metadata.annotations["jaegertracing.io/restore-tested"] == "true"
    and .metadata.annotations["jaegertracing.io/source-version"] == $version
    and .metadata.annotations["jaegertracing.io/source-image-digest"] == $image
  ' >/dev/null <<<"$content_json" ||
    err "The tested snapshot is not backed by a retained OCI VolumeSnapshotContent"
}

verify_recovery_gate() {
  local pod_json
  local source_version
  local source_image
  local snapshots_json
  local cutoff_epoch
  local snapshot
  local content

  require_snapshot_api
  pod_json=$(get_pod_json)
  source_version=$(get_live_version)
  [[ "$source_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || err "Cannot determine live OpenSearch version"
  source_image=$(get_live_image "$pod_json")
  [[ "$source_image" == *@sha256:* ]] || err "Cannot resolve the live OpenSearch image by immutable digest"
  snapshots_json=$(kubectl -n "$NAMESPACE" get volumesnapshots \
    -l jaegertracing.io/recovery-source=opensearch -o json)
  cutoff_epoch=$(($(date -u +%s) - MAX_AGE_SECONDS))
  snapshot=$(select_recovery_snapshot "$snapshots_json" "$source_version" "$source_image" "$cutoff_epoch")
  [[ -n "$snapshot" ]] ||
    err "No recent tested recovery snapshot matches live OpenSearch $source_version; run the recovery workflow first"
  content=$(jq -r --arg snapshot "$snapshot" '
    .items[] | select(.metadata.name == $snapshot) | .status.boundVolumeSnapshotContentName // empty
  ' <<<"$snapshots_json")
  [[ -n "$content" ]] || err "Tested snapshot $snapshot is not bound to a VolumeSnapshotContent"
  validate_snapshot_content "$content" "$source_version" "$source_image"
  log "Recovery gate passed with retained snapshot $snapshot for OpenSearch $source_version"
}

create_and_test_recovery() {
  local pod_json
  local jaeger_pods_json
  local source_pvc
  local source_pv
  local source_driver
  local source_version
  local source_image
  local jaeger_image
  local storage_class
  local capacity
  local snapshot="jaeger-os-recovery-$RECOVERY_ID"
  local restore_pvc="jaeger-os-restore-$RECOVERY_ID"
  local restore_pod="jaeger-os-verify-$RECOVERY_ID"
  local restore_configmap="jaeger-os-query-$RECOVERY_ID"
  local snapshot_content
  local snapshot_content_json
  local tested_at
  local expected_index_inventory
  local expected_alias_inventory
  local expected_template_inventory
  local expected_documents
  local expected_services
  local expected_service
  local expected_trace_id
  local actual_index_inventory
  local actual_alias_inventory
  local actual_template_inventory
  local actual_documents
  local actual_services
  local restored_version
  local trace_response

  validate_recovery_id
  require_snapshot_api
  pod_json=$(get_pod_json)
  source_pvc=$(get_source_pvc "$pod_json")
  [[ -n "$source_pvc" && "$source_pvc" != "null" ]] || err "Cannot resolve the OpenSearch PVC"
  source_image=$(get_live_image "$pod_json")
  [[ "$source_image" == *@sha256:* ]] ||
    err "Cannot resolve the live OpenSearch image by immutable digest"
  source_version=$(get_live_version)
  [[ "$source_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || err "Cannot determine live OpenSearch version"
  jaeger_pods_json=$(kubectl -n "$JAEGER_NAMESPACE" get pods -l "$JAEGER_SELECTOR" -o json)
  jaeger_image=$(get_live_jaeger_image "$jaeger_pods_json")
  [[ "$jaeger_image" == *@sha256:* ]] || err "Cannot resolve the live Jaeger image by immutable digest"

  source_pv=$(kubectl -n "$NAMESPACE" get pvc "$source_pvc" -o jsonpath='{.spec.volumeName}')
  source_driver=$(kubectl get pv "$source_pv" -o jsonpath='{.spec.csi.driver}')
  [[ "$source_driver" == "blockvolume.csi.oraclecloud.com" ]] ||
    err "OpenSearch PV $source_pv is not an OCI CSI Block Volume"
  storage_class=$(kubectl -n "$NAMESPACE" get pvc "$source_pvc" -o jsonpath='{.spec.storageClassName}')
  capacity=$(kubectl -n "$NAMESPACE" get pvc "$source_pvc" -o jsonpath='{.status.capacity.storage}')
  [[ -n "$storage_class" && -n "$capacity" ]] || err "Cannot resolve source PVC storage class and capacity"

  log "Flushing OpenSearch before the crash-consistent OCI snapshot"
  opensearch_api POST '_flush?wait_if_ongoing=true' >/dev/null
  expected_index_inventory=$(live_inventory_exact '_cat/indices/jaeger-*?h=index')
  expected_alias_inventory=$(live_inventory_exact '_cat/aliases/jaeger*?h=alias,index')
  expected_template_inventory=$(live_inventory_exact '_cat/templates/jaeger*?h=name')
  expected_documents=$(live_document_count 'jaeger-*')
  expected_services=$(live_document_count 'jaeger-main-jaeger-service-*')
  expected_service=$(live_representative_field 'jaeger-main-jaeger-service-*' 'serviceName')
  expected_trace_id=$(live_representative_field 'jaeger-main-jaeger-span-*' 'traceID' 'startTime:desc')
  [[ -n "$expected_index_inventory" ]] || err "No Jaeger indexes found in the live cluster"
  ((expected_documents > 0)) || err "No Jaeger documents found in the live cluster"
  ((expected_services > 0)) || err "No Jaeger service documents found in the live cluster"
  [[ -n "$expected_template_inventory" ]] || err "No Jaeger templates found in the live cluster"
  [[ -n "$expected_service" ]] || err "Cannot select a representative Jaeger service"
  [[ "$expected_trace_id" =~ ^[[:xdigit:]]+$ ]] || err "Cannot select a representative Jaeger trace"

  ensure_snapshot_class
  render_snapshot "$snapshot" "$source_pvc" "$source_version" "$source_image" | kubectl create -f -
  wait_for_jsonpath "$NAMESPACE" "volumesnapshot/$snapshot" '{.status.readyToUse}' 'true'
  snapshot_content=$(kubectl -n "$NAMESPACE" get "volumesnapshot/$snapshot" \
    -o jsonpath='{.status.boundVolumeSnapshotContentName}')
  [[ -n "$snapshot_content" ]] || err "Recovery snapshot is not bound to a VolumeSnapshotContent"
  snapshot_content_json=$(kubectl get "volumesnapshotcontent/$snapshot_content" -o json)
  jq -e '
    .spec.driver == "blockvolume.csi.oraclecloud.com"
    and .spec.deletionPolicy == "Retain"
    and ((.status.snapshotHandle // .spec.source.snapshotHandle // "") | length > 0)
  ' >/dev/null <<<"$snapshot_content_json" || err "Recovery snapshot is not backed by a retained OCI backup"

  render_restore_pvc "$restore_pvc" "$snapshot" "$storage_class" "$capacity" | kubectl create -f -
  render_restore_configmap "$restore_configmap" | kubectl create -f -
  ACTIVE_RESTORE_POD=$restore_pod
  trap cleanup_restore_pod EXIT
  render_restore_pod "$restore_pod" "$restore_pvc" "$source_image" "$jaeger_image" "$restore_configmap" |
    kubectl create -f -
  wait_for_jsonpath "$NAMESPACE" "pvc/$restore_pvc" '{.status.phase}' 'Bound'
  wait_for_restored_opensearch "$restore_pod"

  restored_version=$(restored_api "$restore_pod" '' | jq -r '.version.number')
  [[ "$restored_version" == "$source_version" ]] || err "Restored OpenSearch version does not match the source"
  actual_index_inventory=$(restored_inventory_exact "$restore_pod" '_cat/indices/jaeger-*?h=index')
  actual_alias_inventory=$(restored_inventory_exact "$restore_pod" '_cat/aliases/jaeger*?h=alias,index')
  actual_template_inventory=$(restored_inventory_exact "$restore_pod" '_cat/templates/jaeger*?h=name')
  actual_documents=$(restored_document_count "$restore_pod" 'jaeger-*')
  actual_services=$(restored_document_count "$restore_pod" 'jaeger-main-jaeger-service-*')
  assert_inventory_contains "Jaeger index inventory" "$expected_index_inventory" "$actual_index_inventory"
  assert_inventory_contains "Jaeger alias mappings" "$expected_alias_inventory" "$actual_alias_inventory"
  assert_inventory_contains "Jaeger template inventory" "$expected_template_inventory" "$actual_template_inventory"
  ((actual_documents >= expected_documents)) || err "Restored Jaeger document count is below the pre-snapshot count"
  ((actual_services >= expected_services)) || err "Restored Jaeger service count is below the pre-snapshot count"

  wait_for_jaeger_service "$restore_pod" "$expected_service"
  trace_response=$(jaeger_api "$restore_pod" "api/v3/traces/$expected_trace_id")
  jq -e --arg expected "$expected_trace_id" '
    .. | objects | .traceId? |
    select(. != null and (ascii_downcase | endswith($expected | ascii_downcase)))
  ' >/dev/null <<<"$trace_response" || err "Isolated Jaeger could not return the representative restored trace"

  kubectl -n "$NAMESPACE" delete pod "$restore_pod" --wait=true --timeout=2m
  ACTIVE_RESTORE_POD=""
  trap - EXIT
  log "Removed the disposable isolated restore pod after successful verification"
  tested_at=$(date -u +"%FT%TZ")
  kubectl -n "$NAMESPACE" annotate "volumesnapshot/$snapshot" \
    jaegertracing.io/restore-tested=true \
    "jaegertracing.io/restore-tested-at=$tested_at" \
    "jaegertracing.io/restore-pvc=$restore_pvc" \
    "jaegertracing.io/restore-pod=$restore_pod" \
    "jaegertracing.io/tested-jaeger-image-digest=$jaeger_image" \
    --overwrite
  kubectl annotate "volumesnapshotcontent/$snapshot_content" \
    jaegertracing.io/restore-tested=true \
    "jaegertracing.io/restore-tested-at=$tested_at" \
    "jaegertracing.io/source-version=$source_version" \
    "jaegertracing.io/source-image-digest=$source_image" \
    "jaegertracing.io/tested-jaeger-image-digest=$jaeger_image" \
    --overwrite
  log "Recovery point verified: exact inventories, document floors, Jaeger service query, and Jaeger trace query passed"
  log "The retained snapshot, restored PVC, and config require separate cleanup approval"
}

main() {
  local action=${1:-verify}

  need kubectl
  need jq
  need awk
  need sort
  need grep
  case "$action" in
    verify)
      verify_recovery_gate
      ;;
    backup-test)
      create_and_test_recovery
      ;;
    *)
      err "Usage: $0 [verify|backup-test]"
      ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
