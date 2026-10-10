#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

SCRIPT="$(cd "$(dirname "$0")" && pwd)/opensearch-recovery.sh"

testResolvesSourcePvcAndImmutableImage() {
  pod_json='{
    "spec": {
      "containers": [{
        "name": "opensearch",
        "image": "opensearchproject/opensearch:2.11.0",
        "volumeMounts": [{"name": "data", "mountPath": "/usr/share/opensearch/data"}]
      }],
      "volumes": [{"name": "data", "persistentVolumeClaim": {"claimName": "data-opensearch-0"}}]
    },
    "status": {
      "containerStatuses": [{
        "name": "opensearch",
        "imageID": "docker-pullable://opensearchproject/opensearch@sha256:0123456789abcdef"
      }]
    }
  }'

  output=$(bash -c 'source "$1"; get_source_pvc "$2"; get_live_image "$2"' _ "$SCRIPT" "$pod_json")
  expected='data-opensearch-0
opensearchproject/opensearch@sha256:0123456789abcdef'
  assertEquals "$expected" "$output"
}

testDoesNotFallBackToMutableImageTag() {
  pod_json='{
    "spec": {
      "containers": [{
        "name": "opensearch",
        "image": "opensearchproject/opensearch:2.11.0"
      }]
    },
    "status": {"containerStatuses": []}
  }'

  output=$(bash -c 'source "$1"; get_live_image "$2"' _ "$SCRIPT" "$pod_json")
  assertEquals "" "$output"
}

testResolvesLiveJaegerImageByImmutableDigest() {
  pods_json='{
    "items": [{
      "status": {
        "phase": "Running",
        "containerStatuses": [{
          "name": "jaeger",
          "imageID": "docker-pullable://jaegertracing/jaeger-snapshot@sha256:fedcba9876543210"
        }]
      }
    }]
  }'

  output=$(bash -c 'source "$1"; get_live_jaeger_image "$2"' _ "$SCRIPT" "$pods_json")
  assertEquals "jaegertracing/jaeger-snapshot@sha256:fedcba9876543210" "$output"
}

testSelectsNewestRecentTestedSnapshotForLiveVersion() {
  snapshots='{
    "items": [
      {
        "metadata": {
          "name": "wrong-version",
          "creationTimestamp": "2026-08-12T07:00:00Z",
          "annotations": {
            "jaegertracing.io/restore-tested": "true",
            "jaegertracing.io/source-version": "2.19.6",
            "jaegertracing.io/source-image-digest": "opensearch@sha256:abc"
          }
        },
        "status": {"readyToUse": true}
      },
      {
        "metadata": {
          "name": "untested",
          "creationTimestamp": "2026-08-12T07:10:00Z",
          "annotations": {
            "jaegertracing.io/restore-tested": "false",
            "jaegertracing.io/source-version": "2.11.0",
            "jaegertracing.io/source-image-digest": "opensearch@sha256:abc"
          }
        },
        "status": {"readyToUse": true}
      },
      {
        "metadata": {
          "name": "tested-current",
          "creationTimestamp": "2026-08-12T07:20:00Z",
          "annotations": {
            "jaegertracing.io/restore-tested": "true",
            "jaegertracing.io/source-version": "2.11.0",
            "jaegertracing.io/source-image-digest": "opensearch@sha256:abc"
          }
        },
        "status": {"readyToUse": true}
      }
    ]
  }'

  output=$(bash -c \
    'source "$1"; select_recovery_snapshot "$2" 2.11.0 "opensearch@sha256:abc" 0' \
    _ "$SCRIPT" "$snapshots")
  assertEquals "tested-current" "$output"
}

testSnapshotManifestIsRetainedAndVersionBound() {
  output=$(bash -c \
    'source "$1"; render_snapshot recovery-1 source-pvc 2.11.0 "opensearch@sha256:abc"' \
    _ "$SCRIPT")

  assertContains "$output" "kind: VolumeSnapshot"
  assertContains "$output" "volumeSnapshotClassName: jaeger-opensearch-oci-retain"
  assertContains "$output" "persistentVolumeClaimName: source-pvc"
  assertContains "$output" 'jaegertracing.io/source-version: "2.11.0"'
  assertContains "$output" 'jaegertracing.io/source-image-digest: "opensearch@sha256:abc"'
  assertContains "$output" 'jaegertracing.io/restore-tested: "false"'

  class=$(cat "$(dirname "$SCRIPT")/opensearch-volume-snapshot-class.yaml")
  assertContains "$class" "driver: blockvolume.csi.oraclecloud.com"
  assertContains "$class" "deletionPolicy: Retain"
  assertContains "$class" "backupType: full"
}

testRestorePvcUsesNewClaimAndSnapshotDataSource() {
  output=$(bash -c 'source "$1"; render_restore_pvc restored recovery-1 oci-bv 50Gi' _ "$SCRIPT")

  assertContains "$output" "name: restored"
  assertContains "$output" "storageClassName: oci-bv"
  assertContains "$output" "storage: 50Gi"
  assertContains "$output" "name: recovery-1"
  assertContains "$output" "kind: VolumeSnapshot"
  assertContains "$output" "apiGroup: snapshot.storage.k8s.io"
}

testRestorePodUsesExactImagesAndNoNetworkService() {
  config=$(bash -c 'source "$1"; render_restore_configmap config' _ "$SCRIPT")
  output=$(bash -c \
    'source "$1"; render_restore_pod verify restored "opensearchproject/opensearch@sha256:abc" "jaegertracing/jaeger@sha256:def" config' \
    _ "$SCRIPT")

  assertContains "$config" "kind: ConfigMap"
  assertContains "$config" "server_urls:"
  assertContains "$config" "http://127.0.0.1:9200"
  assertContains "$config" "endpoint: 127.0.0.1:16686"
  assertContains "$config" "endpoint: 127.0.0.1:16685"
  assertContains "$config" 'index_prefix: "jaeger-main"'
  assertContains "$output" "kind: Pod"
  assertContains "$output" "image: opensearchproject/opensearch@sha256:abc"
  assertContains "$output" "image: jaegertracing/jaeger@sha256:def"
  assertContains "$output" "automountServiceAccountToken: false"
  assertContains "$output" "requiredDuringSchedulingIgnoredDuringExecution"
  assertContains "$output" "memory: 6Gi"
  assertContains "$output" 'value: "-Xms3g -Xmx3g"'
  assertContains "$output" "-Enetwork.host=127.0.0.1"
  assertContains "$output" "/cmd/jaeger/jaeger-linux"
  assertNotContains "$output" "kind: Service"
  assertNotContains "$output" "kind: Ingress"
}

testRecoveryScriptOnlyDeletesDisposableRestorePod() {
  contents=$(cat "$SCRIPT")
  assertContains "$contents" "kubectl -n \"\$NAMESPACE\" delete pod \"\$restore_pod\""
  assertContains "$contents" "trap cleanup_restore_pod EXIT"
  assertContains "$contents" "delete pod \"\$ACTIVE_RESTORE_POD\""
  assertNotContains "$contents" "delete pvc"
  assertNotContains "$contents" "delete volumesnapshot"
  assertNotContains "$contents" "delete namespace"
  assertNotContains "$contents" "delete statefulset"
  assertNotContains "$contents" "helm uninstall"
  assertNotContains "$contents" "rm -"
  assertNotContains "$contents" "kubectl -n \"\$NAMESPACE\" logs"
}

testRecoveryValidationUsesHealthWaitAndJaegerReadPath() {
  contents=$(cat "$SCRIPT")
  assertContains "$contents" "wait_for_status=yellow"
  assertContains "$contents" "api/v3/services"
  assertContains "$contents" "api/v3/traces/\$expected_trace_id"
  assertContains "$contents" "volumesnapshotcontent/\$snapshot_content"
  assertContains "$contents" "jaegertracing.io/source-image-digest"
}

testInventoryValidationAllowsPostBaselineAdditions() {
  bash -c 'source "$1"; assert_inventory_contains indexes "$2" "$3"' \
    _ "$SCRIPT" $'jaeger-2026-08-11\njaeger-2026-08-12' \
    $'jaeger-2026-08-11\njaeger-2026-08-12\njaeger-2026-08-13'
  assertEquals 0 $?
}

testInvalidRecoveryIdIsRejected() {
  output=$(bash -c 'source "$1"; RECOVERY_ID="Unsafe_ID"; validate_recovery_id' _ "$SCRIPT" 2>&1)
  rc=$?

  assertNotEquals 0 "$rc"
  assertContains "$output" "OPENSEARCH_RECOVERY_ID must be"
}

# shellcheck disable=SC1091
source "${SHUNIT2}/shunit2"
