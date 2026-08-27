#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

SCRIPT="$(cd "$(dirname "$0")" && pwd)/opensearch-expand-volume.sh"

testResolvesDataPvcFromExpectedContainerMount() {
  pod_json='{
    "spec": {
      "containers": [{
        "name": "opensearch",
        "volumeMounts": [{"name": "data", "mountPath": "/usr/share/opensearch/data"}]
      }],
      "volumes": [{"name": "data", "persistentVolumeClaim": {"claimName": "data-opensearch-0"}}]
    }
  }'

  output=$(bash -c 'source "$1"; get_source_pvc "$2"' _ "$SCRIPT" "$pod_json")
  assertEquals "data-opensearch-0" "$output"
}

testVerifiesPodControllerUid() {
  pod_json='{
    "metadata": {"ownerReferences": [{
      "kind": "StatefulSet", "name": "opensearch-cluster-single",
      "controller": true, "uid": "expected"
    }]}
  }'
  statefulset_json='{"metadata":{"uid":"expected"}}'

  bash -c 'source "$1"; verify_pod_owner "$2" "$3"' \
    _ "$SCRIPT" "$pod_json" "$statefulset_json"
  assertEquals 0 $?
}

testRejectsUnexpectedPodController() {
  pod_json='{
    "metadata": {"ownerReferences": [{
      "kind": "StatefulSet", "name": "opensearch-cluster-single",
      "controller": true, "uid": "wrong"
    }]}
  }'
  statefulset_json='{"metadata":{"uid":"expected"}}'

  output=$(bash -c 'source "$1"; verify_pod_owner "$2" "$3"' \
    _ "$SCRIPT" "$pod_json" "$statefulset_json" 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "not controlled by the expected StatefulSet"
}

testRejectsPodWithoutOwnerReferencesWithSanitizedMessage() {
  pod_json='{"metadata":{}}'
  statefulset_json='{"metadata":{"uid":"expected"}}'

  output=$(bash -c 'source "$1"; verify_pod_owner "$2" "$3"' \
    _ "$SCRIPT" "$pod_json" "$statefulset_json" 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "not controlled by the expected StatefulSet"
  assertNotContains "$output" "Cannot iterate over null"
}

testVerifiesPvcIdentityAndBinding() {
  pvc_json='{
    "metadata":{"uid":"pvc-uid"},
    "spec":{"volumeName":"pv-name"}
  }'

  bash -c 'source "$1"; verify_pvc_identity "$2" pvc-uid pv-name' \
    _ "$SCRIPT" "$pvc_json"
  assertEquals 0 $?
}

testRejectsChangedPvcIdentity() {
  pvc_json='{
    "metadata":{"uid":"new-pvc-uid"},
    "spec":{"volumeName":"new-pv-name"}
  }'

  output=$(bash -c 'source "$1"; verify_pvc_identity "$2" pvc-uid pv-name' \
    _ "$SCRIPT" "$pvc_json" 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "PVC identity or binding changed"
}

testVerifiesExpectedCsiStorage() {
  pvc_json='{
    "metadata":{"name":"data","namespace":"opensearch","uid":"pvc-uid"},
    "spec": {"storageClassName":"oci-bv","volumeMode":"Filesystem","volumeName":"redacted"},
    "status":{"phase":"Bound"}
  }'
  storage_class_json='{
    "provisioner":"blockvolume.csi.oraclecloud.com","allowVolumeExpansion":true
  }'
  pv_json='{
    "spec":{
      "csi":{"driver":"blockvolume.csi.oraclecloud.com"},
      "claimRef":{"name":"data","namespace":"opensearch","uid":"pvc-uid"}
    },
    "status":{"phase":"Bound"}
  }'

  bash -c 'source "$1"; verify_storage "$2" "$3" "$4"' \
    _ "$SCRIPT" "$pvc_json" "$storage_class_json" "$pv_json"
  assertEquals 0 $?
}

testRejectsStorageClassWithoutExpansion() {
  pvc_json='{
    "metadata":{"name":"data","namespace":"opensearch","uid":"pvc-uid"},
    "spec": {"storageClassName":"oci-bv","volumeMode":"Filesystem","volumeName":"redacted"},
    "status":{"phase":"Bound"}
  }'
  storage_class_json='{
    "provisioner":"blockvolume.csi.oraclecloud.com","allowVolumeExpansion":false
  }'
  pv_json='{
    "spec":{
      "csi":{"driver":"blockvolume.csi.oraclecloud.com"},
      "claimRef":{"name":"data","namespace":"opensearch","uid":"pvc-uid"}
    },
    "status":{"phase":"Bound"}
  }'

  output=$(bash -c 'source "$1"; verify_storage "$2" "$3" "$4"' \
    _ "$SCRIPT" "$pvc_json" "$storage_class_json" "$pv_json" 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "does not support expected CSI expansion"
}

testRejectsMismatchedPvClaimRef() {
  pvc_json='{
    "metadata":{"name":"data","namespace":"opensearch","uid":"pvc-uid"},
    "spec":{"storageClassName":"oci-bv","volumeMode":"Filesystem","volumeName":"redacted"},
    "status":{"phase":"Bound"}
  }'
  storage_class_json='{
    "provisioner":"blockvolume.csi.oraclecloud.com","allowVolumeExpansion":true
  }'
  pv_json='{
    "spec":{
      "csi":{"driver":"blockvolume.csi.oraclecloud.com"},
      "claimRef":{"name":"other","namespace":"opensearch","uid":"other-uid"}
    },
    "status":{"phase":"Bound"}
  }'

  output=$(bash -c 'source "$1"; verify_storage "$2" "$3" "$4"' \
    _ "$SCRIPT" "$pvc_json" "$storage_class_json" "$pv_json" 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "PVC and PV binding do not match"
}

testPodIdentityDetectsRestartOrReplacement() {
  pod_json='{
    "metadata":{"uid":"pod-uid"},
    "status":{"containerStatuses":[{
      "name":"opensearch","containerID":"containerd://one","restartCount":2
    }]}
  }'
  assertEquals $'pod-uid\tcontainerd://one\t2' \
    "$(bash -c 'source "$1"; get_pod_identity "$2"' _ "$SCRIPT" "$pod_json")"

  output=$(bash -c 'source "$1"; verify_pod_identity "$2" pod-uid containerd://one 1' \
    _ "$SCRIPT" "$pod_json" 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "restarted or was replaced"
}

testOpenSearchHealthAcceptsYellowWithoutShardMovement() {
  output=$(bash -c '
    source "$1"
    fixture=$2
    kubectl() { printf "%s" "$fixture"; }
    verify_opensearch_health
  ' _ "$SCRIPT" '{"status":"yellow","number_of_nodes":1,"initializing_shards":0,"relocating_shards":0}' 2>&1)
  assertEquals "" "$output"
}

testOpenSearchHealthRejectsRelocatingShards() {
  output=$(bash -c '
    source "$1"
    fixture=$2
    kubectl() { printf "%s" "$fixture"; }
    verify_opensearch_health
  ' _ "$SCRIPT" '{"status":"yellow","number_of_nodes":1,"initializing_shards":0,"relocating_shards":1}' 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "not healthy enough"
}

testJaegerHealthRequiresCheckout() {
  bash -c '
    source "$1"
    fixture=$2
    curl() { printf "%s" "$fixture"; }
    verify_jaeger_health
  ' _ "$SCRIPT" '{"services":[{"name":"checkout"}]}'
  assertEquals 0 $?

  output=$(bash -c '
    source "$1"
    fixture=$2
    curl() { printf "%s" "$fixture"; }
    verify_jaeger_health
  ' _ "$SCRIPT" '{"services":[{"name":"frontend"}]}' 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "cannot query the checkout service"
}

testWaitForExpansionRequiresRequestCapacityAndNoConditions() {
  output=$(bash -c '
    source "$1"
    TIMEOUT_SECONDS=1
    fixture=$2
    kubectl() { printf "%s" "$fixture"; }
    sleep() { :; }
    wait_for_expansion data
  ' _ "$SCRIPT" '{
    "spec":{"resources":{"requests":{"storage":"100Gi"}}},
    "status":{"capacity":{"storage":"100Gi"},"conditions":[]}
  }' 2>&1)
  assertEquals "" "$output"

  output=$(bash -c '
    source "$1"
    TIMEOUT_SECONDS=1
    fixture=$2
    kubectl() { printf "%s" "$fixture"; }
    sleep() { :; }
    wait_for_expansion data
  ' _ "$SCRIPT" '{
    "spec":{"resources":{"requests":{"storage":"100Gi"}}},
    "status":{"capacity":{"storage":"100Gi"},"conditions":[{"status":"True"}]}
  }' 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "did not finish expanding"
}

testFilesystemHeadroomRequiresSizeAndThirtyPercentFree() {
  bash -c '
    source "$1"
    fixture=$2
    kubectl() { printf "%s" "$fixture"; }
    verify_filesystem_headroom
  ' _ "$SCRIPT" '{"nodes":{"one":{"fs":{"total":{"total_in_bytes":107374182400,"available_in_bytes":32212254720}}}}}'
  assertEquals 0 $?

  output=$(bash -c '
    source "$1"
    fixture=$2
    kubectl() { printf "%s" "$fixture"; }
    verify_filesystem_headroom
  ' _ "$SCRIPT" '{"nodes":{"one":{"fs":{"total":{"total_in_bytes":107374182400,"available_in_bytes":21474836480}}}}}' 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "does not have at least 30% free capacity"
}

testPvReadFailureIsSanitized() {
  output=$(bash -c '
    source "$1"
    kubectl() { printf "forbidden: secret-pv-name\n" >&2; return 1; }
    get_bound_pv_json secret-pv-name
  ' _ "$SCRIPT" 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "Cannot read the bound OpenSearch PV"
  assertNotContains "$output" "secret-pv-name"
}

testClassifiesKnownCapacityStates() {
  assertEquals "patch" "$(bash -c 'source "$1"; classify_capacity_state 10Gi 50Gi' _ "$SCRIPT")"
  assertEquals "patch" "$(bash -c 'source "$1"; classify_capacity_state 50Gi 50Gi' _ "$SCRIPT")"
  assertEquals "wait" "$(bash -c 'source "$1"; classify_capacity_state 100Gi 50Gi' _ "$SCRIPT")"
  assertEquals "verify" "$(bash -c 'source "$1"; classify_capacity_state 100Gi 100Gi' _ "$SCRIPT")"
}

testRejectsUnknownCapacityState() {
  output=$(bash -c 'source "$1"; classify_capacity_state 75Gi 60Gi' _ "$SCRIPT" 2>&1)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" "Unexpected OpenSearch PVC capacity state"
}

testPatchUsesIdentityVersionAndCurrentSizePreconditions() {
  output=$(bash -c 'source "$1"; render_storage_patch uid-1 rv-2 50Gi' _ "$SCRIPT")

  assertEquals "uid-1" "$(jq -r '.[0].value' <<<"$output")"
  assertEquals "/metadata/uid" "$(jq -r '.[0].path' <<<"$output")"
  assertEquals "rv-2" "$(jq -r '.[1].value' <<<"$output")"
  assertEquals "50Gi" "$(jq -r '.[2].value' <<<"$output")"
  assertEquals "replace" "$(jq -r '.[3].op' <<<"$output")"
  assertEquals "/spec/resources/requests/storage" "$(jq -r '.[3].path' <<<"$output")"
  assertEquals "100Gi" "$(jq -r '.[3].value' <<<"$output")"
}

testWaitAndVerifyStatesNeverPatch() {
  pvc_json='{
    "metadata":{"uid":"pvc-uid","resourceVersion":"rv-1"},
    "spec":{"resources":{"requests":{"storage":"100Gi"}}}
  }'

  output=$(bash -c '
    source "$1"
    kubectl() { printf "unsafe patch\n"; }
    apply_capacity_state wait data "$2" 100Gi
    apply_capacity_state verify data "$2" 100Gi
  ' _ "$SCRIPT" "$pvc_json")
  assertNotContains "$output" "unsafe patch"
  assertContains "$output" "request already exists"
  assertContains "$output" "already expanded"
}

testPatchStateCallsOnlyPreconditionedPvcPatch() {
  pvc_json='{
    "metadata":{"uid":"pvc-uid","resourceVersion":"rv-1"},
    "spec":{"resources":{"requests":{"storage":"50Gi"}}}
  }'

  output=$(bash -c '
    source "$1"
    called=0
    arguments=""
    kubectl() { called=1; arguments=$*; }
    apply_capacity_state patch data "$2" 50Gi
    printf "called=%s\narguments=%s\n" "$called" "$arguments"
  ' _ "$SCRIPT" "$pvc_json")
  assertContains "$output" "called=1"
  assertContains "$output" "patch pvc data --type=json"
  assertContains "$output" '"path":"/metadata/uid","value":"pvc-uid"'
  assertContains "$output" '"path":"/metadata/resourceVersion","value":"rv-1"'
  assertContains "$output" '"path":"/spec/resources/requests/storage","value":"100Gi"'
  assertContains "$output" "do not retry blindly"
}

testScriptAllowsOnlyPvcPatchMutation() {
  contents=$(cat "$SCRIPT")

  # shellcheck disable=SC2016
  assertContains "$contents" 'patch pvc "$pvc" --type=json'
  assertNotContains "$contents" "kubectl delete"
  assertNotContains "$contents" "kubectl create"
  assertNotContains "$contents" "kubectl apply"
  assertNotContains "$contents" "helm "
  assertNotContains "$contents" "oci "
  assertNotContains "$contents" "volumeHandle"
  assertNotContains "$contents" "describe"
  assertNotContains "$contents" "events"
  assertNotContains "$contents" "set -x"
}

testScriptKeepsTargetFixedAt100Gi() {
  contents=$(cat "$SCRIPT")

  assertContains "$contents" 'TARGET_STORAGE="100Gi"'
  assertContains "$contents" "at least 30% free capacity"
  # shellcheck disable=SC2016
  assertNotContains "$contents" 'TARGET_STORAGE="${'
}

# shellcheck disable=SC1091
source "${SHUNIT2}/shunit2"
