#!/usr/bin/env bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

NAMESPACE="opensearch"
STATEFULSET="opensearch-cluster-single"
POD="opensearch-cluster-single-0"
CONTAINER="opensearch"
DATA_PATH="/usr/share/opensearch/data"
STORAGE_CLASS="oci-bv"
CSI_DRIVER="blockvolume.csi.oraclecloud.com"
TARGET_STORAGE="100Gi"
TIMEOUT_SECONDS="${OPENSEARCH_EXPAND_TIMEOUT_SECONDS:-3000}"
JAEGER_URL="${JAEGER_OTEL_DEMO_JAEGER_URL:-https://jaeger.demo.jaegertracing.io}"

log() { echo "[$(date -u +"%F %T UTC")] $*"; }
err() { echo "[$(date -u +"%F %T UTC")] ERROR: $*" >&2; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || err "$1 is required but not installed"
}

get_pod_json() {
  kubectl -n "$NAMESPACE" get pod "$POD" -o json
}

get_statefulset_json() {
  kubectl -n "$NAMESPACE" get statefulset "$STATEFULSET" -o json
}

get_bound_pv_json() {
  local pv=$1
  local pv_json

  pv_json=$(kubectl get pv "$pv" -o json 2>/dev/null) ||
    err "Cannot read the bound OpenSearch PV"
  printf '%s\n' "$pv_json"
}

get_source_pvc() {
  local pod_json=$1
  local data_volume

  data_volume=$(jq -r --arg container "$CONTAINER" --arg path "$DATA_PATH" '
    .spec.containers[]
    | select(.name == $container)
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

verify_pod_owner() {
  local pod_json=$1
  local statefulset_json=$2
  local expected_uid
  local owner_uid

  expected_uid=$(jq -r '.metadata.uid' <<<"$statefulset_json")
  owner_uid=$(jq -r --arg name "$STATEFULSET" '
    (.metadata.ownerReferences // [])[]
    | select(.kind == "StatefulSet" and .name == $name and .controller == true)
    | .uid
  ' <<<"$pod_json")
  [[ -n "$expected_uid" && "$owner_uid" == "$expected_uid" ]] ||
    err "OpenSearch pod is not controlled by the expected StatefulSet"
}

verify_pvc_identity() {
  local pvc_json=$1
  local expected_pvc_uid=$2
  local expected_volume_name=$3

  jq -e --arg pvc_uid "$expected_pvc_uid" --arg volume_name "$expected_volume_name" '
    .metadata.uid == $pvc_uid
    and .spec.volumeName == $volume_name
  ' >/dev/null <<<"$pvc_json" || err "OpenSearch PVC identity or binding changed"
}

get_pod_identity() {
  local pod_json=$1

  jq -er --arg container "$CONTAINER" '
    . as $pod
    | ([.status.containerStatuses[]? | select(.name == $container)] | first) as $status
    | select(($pod.metadata.uid // "") | length > 0)
    | select(($status.containerID // "") | length > 0)
    | select(($status.restartCount // -1) >= 0)
    | [$pod.metadata.uid, $status.containerID, ($status.restartCount | tostring)]
    | @tsv
  ' <<<"$pod_json"
}

verify_pod_identity() {
  local pod_json=$1
  local expected_pod_uid=$2
  local expected_container_id=$3
  local expected_restart_count=$4
  local identity

  identity=$(get_pod_identity "$pod_json") || err "Cannot resolve the OpenSearch pod identity"
  [[ "$identity" == "$expected_pod_uid"$'\t'"$expected_container_id"$'\t'"$expected_restart_count" ]] ||
    err "OpenSearch pod or container restarted or was replaced during expansion"
}

verify_statefulset_identity() {
  local statefulset_json=$1
  local expected_uid=$2

  [[ "$(jq -r '.metadata.uid // empty' <<<"$statefulset_json")" == "$expected_uid" ]] ||
    err "OpenSearch StatefulSet was replaced during expansion"
}

verify_opensearch_health() {
  local response

  response=$(kubectl -n "$NAMESPACE" exec "$POD" -c "$CONTAINER" -- \
    curl --fail --silent --show-error \
      'http://127.0.0.1:9200/_cluster/health?filter_path=status,number_of_nodes,initializing_shards,relocating_shards')
  jq -e '
    (.status == "yellow" or .status == "green")
    and .number_of_nodes == 1
    and .initializing_shards == 0
    and .relocating_shards == 0
  ' >/dev/null <<<"$response" || err "OpenSearch is not healthy enough for online expansion"
}

verify_jaeger_health() {
  curl --fail --silent --show-error --max-time 20 "$JAEGER_URL/api/v3/services" |
    jq -e '.. | strings | select(. == "checkout")' >/dev/null ||
    err "Jaeger cannot query the checkout service"
}

verify_storage() {
  local pvc_json=$1
  local storage_class_json=$2
  local pv_json=$3
  local pvc_name
  local pvc_namespace
  local pvc_uid

  pvc_name=$(jq -r '.metadata.name // empty' <<<"$pvc_json")
  pvc_namespace=$(jq -r '.metadata.namespace // empty' <<<"$pvc_json")
  pvc_uid=$(jq -r '.metadata.uid // empty' <<<"$pvc_json")

  jq -e --arg class "$STORAGE_CLASS" '
    .status.phase == "Bound"
    and ((.spec.volumeMode // "Filesystem") == "Filesystem")
    and .spec.storageClassName == $class
    and ((.spec.volumeName // "") | length > 0)
  ' >/dev/null <<<"$pvc_json" || err "OpenSearch PVC is not the expected bound filesystem claim"
  jq -e --arg driver "$CSI_DRIVER" '
    .provisioner == $driver and .allowVolumeExpansion == true
  ' >/dev/null <<<"$storage_class_json" || err "The oci-bv StorageClass does not support expected CSI expansion"
  jq -e --arg driver "$CSI_DRIVER" '
    .status.phase == "Bound" and .spec.csi.driver == $driver
  ' >/dev/null <<<"$pv_json" || err "OpenSearch is not backed by the expected OCI CSI Block Volume"
  jq -e \
    --arg name "$pvc_name" \
    --arg namespace "$pvc_namespace" \
    --arg uid "$pvc_uid" '
      .spec.claimRef.name == $name
      and .spec.claimRef.namespace == $namespace
      and .spec.claimRef.uid == $uid
    ' >/dev/null <<<"$pv_json" || err "OpenSearch PVC and PV binding do not match"
}

classify_capacity_state() {
  local requested=$1
  local capacity=$2

  case "$requested|$capacity" in
    10Gi\|50Gi|50Gi\|50Gi)
      printf 'patch\n'
      ;;
    100Gi\|50Gi)
      printf 'wait\n'
      ;;
    100Gi\|100Gi)
      printf 'verify\n'
      ;;
    *)
      err "Unexpected OpenSearch PVC capacity state; refusing to modify it"
      ;;
  esac
}

render_storage_patch() {
  local uid=$1
  local resource_version=$2
  local requested=$3

  jq -nc \
    --arg uid "$uid" \
    --arg resourceVersion "$resource_version" \
    --arg requested "$requested" \
    --arg target "$TARGET_STORAGE" '[
      {"op":"test","path":"/metadata/uid","value":$uid},
      {"op":"test","path":"/metadata/resourceVersion","value":$resourceVersion},
      {"op":"test","path":"/spec/resources/requests/storage","value":$requested},
      {"op":"replace","path":"/spec/resources/requests/storage","value":$target}
    ]'
}

apply_capacity_state() {
  local state=$1
  local pvc=$2
  local pvc_json=$3
  local requested=$4
  local patch

  case "$state" in
    patch)
      patch=$(render_storage_patch \
        "$(jq -r '.metadata.uid' <<<"$pvc_json")" \
        "$(jq -r '.metadata.resourceVersion' <<<"$pvc_json")" \
        "$requested")
      log "Requesting online expansion of the existing OpenSearch PVC to $TARGET_STORAGE"
      kubectl -n "$NAMESPACE" patch pvc "$pvc" --type=json -p "$patch" >/dev/null 2>&1 ||
        err "OpenSearch PVC expansion request was rejected"
      log "Expansion request accepted; if this run stops before verification, inspect the CSI resize state and do not retry blindly"
      ;;
    wait)
      log "The online expansion request already exists; waiting for completion"
      ;;
    verify)
      log "The OpenSearch PVC is already expanded; verifying capacity and health"
      ;;
    *)
      err "Unknown OpenSearch PVC expansion state"
      ;;
  esac
}

wait_for_expansion() {
  local pvc=$1
  local elapsed=0
  local pvc_json
  local capacity
  local requested
  local conditions

  while ((elapsed < TIMEOUT_SECONDS)); do
    pvc_json=$(kubectl -n "$NAMESPACE" get pvc "$pvc" -o json)
    capacity=$(jq -r '.status.capacity.storage // empty' <<<"$pvc_json")
    requested=$(jq -r '.spec.resources.requests.storage // empty' <<<"$pvc_json")
    conditions=$(jq -r '[.status.conditions[]? | select(.status == "True")] | length' <<<"$pvc_json")
    if [[ "$requested" == "$TARGET_STORAGE" && "$capacity" == "$TARGET_STORAGE" && "$conditions" == "0" ]]; then
      return 0
    fi
    sleep 10
    elapsed=$((elapsed + 10))
  done

  err "OpenSearch PVC did not finish expanding within ${TIMEOUT_SECONDS}s"
}

verify_filesystem_headroom() {
  local response

  response=$(kubectl -n "$NAMESPACE" exec "$POD" -c "$CONTAINER" -- \
    curl --fail --silent --show-error \
      'http://127.0.0.1:9200/_nodes/stats/fs?filter_path=nodes.*.fs.total.total_in_bytes,nodes.*.fs.total.available_in_bytes')
  jq -e '
    [.nodes[].fs.total
      | select(.total_in_bytes >= 96636764160)
      | select((.available_in_bytes / .total_in_bytes) >= 0.30)]
    | length == 1
  ' >/dev/null <<<"$response" || err "Expanded OpenSearch filesystem does not have at least 30% free capacity"
}

main() {
  local pod_json
  local statefulset_json
  local pvc
  local pvc_json
  local expected_pvc_uid
  local pv
  local pv_json
  local storage_class_json
  local requested
  local capacity
  local state
  local expected_statefulset_uid
  local expected_pod_uid
  local expected_container_id
  local expected_restart_count
  local pod_identity
  local final_pvc

  need kubectl
  need jq
  need curl
  pod_json=$(get_pod_json)
  statefulset_json=$(get_statefulset_json)
  expected_statefulset_uid=$(jq -r '.metadata.uid // empty' <<<"$statefulset_json")
  [[ -n "$expected_statefulset_uid" ]] || err "Cannot resolve the OpenSearch StatefulSet identity"
  verify_pod_owner "$pod_json" "$statefulset_json"
  pod_identity=$(get_pod_identity "$pod_json") || err "Cannot resolve the OpenSearch pod identity"
  IFS=$'\t' read -r expected_pod_uid expected_container_id expected_restart_count <<<"$pod_identity"
  pvc=$(get_source_pvc "$pod_json")
  [[ -n "$pvc" && "$pvc" != "null" ]] || err "Cannot resolve the OpenSearch PVC"
  pvc_json=$(kubectl -n "$NAMESPACE" get pvc "$pvc" -o json)
  expected_pvc_uid=$(jq -r '.metadata.uid' <<<"$pvc_json")
  pv=$(jq -r '.spec.volumeName' <<<"$pvc_json")
  [[ -n "$expected_pvc_uid" && -n "$pv" && "$pv" != "null" ]] ||
    err "Cannot resolve the OpenSearch PVC identity and binding"
  verify_pvc_identity "$pvc_json" "$expected_pvc_uid" "$pv"
  storage_class_json=$(kubectl get storageclass "$STORAGE_CLASS" -o json)
  pv_json=$(get_bound_pv_json "$pv")
  verify_storage "$pvc_json" "$storage_class_json" "$pv_json"
  requested=$(jq -r '.spec.resources.requests.storage' <<<"$pvc_json")
  capacity=$(jq -r '.status.capacity.storage' <<<"$pvc_json")
  state=$(classify_capacity_state "$requested" "$capacity")

  verify_opensearch_health
  verify_jaeger_health
  apply_capacity_state "$state" "$pvc" "$pvc_json" "$requested"

  wait_for_expansion "$pvc"
  pod_json=$(get_pod_json)
  statefulset_json=$(get_statefulset_json)
  verify_statefulset_identity "$statefulset_json" "$expected_statefulset_uid"
  verify_pod_owner "$pod_json" "$statefulset_json"
  verify_pod_identity "$pod_json" "$expected_pod_uid" "$expected_container_id" "$expected_restart_count"
  final_pvc=$(get_source_pvc "$pod_json")
  [[ "$final_pvc" == "$pvc" ]] || err "OpenSearch pod data claim changed during expansion"
  pvc_json=$(kubectl -n "$NAMESPACE" get pvc "$pvc" -o json)
  verify_pvc_identity "$pvc_json" "$expected_pvc_uid" "$pv"
  pv_json=$(get_bound_pv_json "$pv")
  verify_storage "$pvc_json" "$storage_class_json" "$pv_json"
  verify_filesystem_headroom
  verify_opensearch_health
  verify_jaeger_health
  log "OpenSearch PVC expansion verified at $TARGET_STORAGE with at least 30% filesystem headroom"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
