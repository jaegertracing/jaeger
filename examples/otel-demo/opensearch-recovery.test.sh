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

# Runs one wait loop of the recovery script against command doubles and prints
# its output followed by "polls=<number of sleeps>". The environment drives the
# doubles:
#   RESTORE_POD_JSON  the restore pod that kubectl returns; unset fails the read
#   READY_AFTER       number of sleeps after which requests succeed; unset never
#   REQUEST_FAILURE   what a failing request writes to stderr
# The clock starts at 2026-01-01T00:00:00Z and advances only when a loop sleeps.
runRestoreWait() {
  bash -c '
    source "$1"
    polls=0
    clock=1767225600
    trap "echo polls=\$polls" EXIT
    request() {
      if [[ -n "${READY_AFTER:-}" ]] && ((polls >= READY_AFTER)); then
        printf "%s\n" "$1"
        return 0
      fi
      echo "${REQUEST_FAILURE:-request failed}" >&2
      return 7
    }
    kubectl() {
      if [[ " $* " != *" get pod "* ]]; then
        request Bound
      elif [[ -n "${RESTORE_POD_JSON:-}" ]]; then
        printf "%s\n" "$RESTORE_POD_JSON"
      else
        echo "pod read failed" >&2
        return 1
      fi
    }
    restored_api() { request "{\"status\":\"yellow\"}"; }
    jaeger_api() { request "{\"services\":[\"frontend\"]}"; }
    sleep() {
      polls=$((polls + 1))
      clock=$((clock + $1))
      SECONDS=$((SECONDS + $1))
    }
    date() {
      if [[ "$*" == "-u +%s" ]]; then
        echo "$clock"
      else
        command date "$@"
      fi
    }
    case "$2" in
      snapshot) wait_for_jsonpath "$NAMESPACE" volumesnapshot/recovery "{.status.readyToUse}" true ;;
      pvc) wait_for_jsonpath "$NAMESPACE" pvc/restored "{.status.phase}" Bound verify ;;
      opensearch) wait_for_restored_opensearch verify ;;
      jaeger) wait_for_jaeger_service verify frontend ;;
    esac
  ' _ "$SCRIPT" "$1" 2>&1
}

# Prints a restore pod whose container $1 is waiting with reason $2 after its
# previous run, started at $3, exited with an error.
restorePodWaiting() {
  cat <<EOF
{"status": {"containerStatuses": [{
  "name": "$1",
  "state": {"waiting": {"reason": "$2", "message": "kubelet reports $2"}},
  "lastState": {"terminated": {
    "reason": "Error", "exitCode": 1, "message": "fatal startup error",
    "startedAt": "${3:-2026-01-01T00:00:00Z}"
  }}
}]}}
EOF
}

testRestoreWaitsFailFastWhenContainerCannotStart() {
  for wait in pvc opensearch jaeger; do
    for container in opensearch jaeger-query; do
      for reason in CrashLoopBackOff ImagePullBackOff ErrImagePull CreateContainerConfigError; do
        # The next test covers the jaeger-query crash loop, which can recover.
        [[ "$container $reason" != "jaeger-query CrashLoopBackOff" ]] || continue
        output=$(RESTORE_POD_JSON=$(restorePodWaiting "$container" "$reason") \
          REQUEST_FAILURE="curl: (7) Failed to connect" \
          OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=60 runRestoreWait "$wait")
        rc=$?

        assertNotEquals "$wait $container $reason" 0 "$rc"
        assertContains "$wait $container $reason" "$output" \
          "ERROR: Restore pod verify cannot recover by waiting: container $container is waiting in $reason (kubelet reports $reason)"
        assertContains "$wait $container $reason" "$output" "Last request error: curl: (7) Failed to connect"
        assertContains "$wait $container $reason" "$output" \
          "Container $container last terminated: reason=Error, exit code=1, message=fatal startup error"
        assertContains "$wait $container $reason" "$output" "polls=0"
      done
    done
  done
}

testRestoreWaitsJudgeJaegerCrashLoopOnlyOnceOpenSearchIsHealthy() {
  # jaeger-query exits until OpenSearch answers, so it crash-loops while
  # OpenSearch starts and recovers on its own once OpenSearch is healthy.
  output=$(RESTORE_POD_JSON=$(restorePodWaiting jaeger-query CrashLoopBackOff) \
    READY_AFTER=3 OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=60 runRestoreWait opensearch)
  assertEquals "polls=3" "$output"

  # The Jaeger wait starts with OpenSearch healthy. A back-off left over from a
  # run that started earlier still ends in a restart that can succeed.
  output=$(RESTORE_POD_JSON=$(restorePodWaiting jaeger-query CrashLoopBackOff 2025-12-31T23:59:59Z) \
    READY_AFTER=3 OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=60 runRestoreWait jaeger)
  assertEquals "polls=3" "$output"

  # The wait records health at 00:00:00Z in whole seconds, so a run that started
  # in that second may have started before OpenSearch was healthy.
  output=$(RESTORE_POD_JSON=$(restorePodWaiting jaeger-query CrashLoopBackOff 2026-01-01T00:00:00Z) \
    READY_AFTER=3 OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=60 runRestoreWait jaeger)
  assertEquals "polls=3" "$output"

  # A run that started after OpenSearch was healthy and still exited will not recover.
  output=$(RESTORE_POD_JSON=$(restorePodWaiting jaeger-query CrashLoopBackOff 2026-01-01T00:00:01Z) \
    OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=60 runRestoreWait jaeger)
  rc=$?
  assertNotEquals 0 "$rc"
  assertContains "$output" \
    "ERROR: Restore pod verify cannot recover by waiting: container jaeger-query is waiting in CrashLoopBackOff"
  assertContains "$output" "Container jaeger-query last terminated: reason=Error, exit code=1"
  assertContains "$output" "polls=0"

  # A run with no start time cannot be shown to have started with OpenSearch healthy.
  pod_json=$(restorePodWaiting jaeger-query CrashLoopBackOff |
    jq 'del(.status.containerStatuses[].lastState.terminated.startedAt)')
  output=$(RESTORE_POD_JSON=$pod_json READY_AFTER=3 OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=60 runRestoreWait jaeger)
  assertEquals "polls=3" "$output"
}

testRestoreWaitsFailFastWhenPodStaysUnschedulable() {
  pod_json='{"status": {"conditions": [{
    "type": "PodScheduled", "status": "False", "reason": "Unschedulable",
    "lastTransitionTime": "2026-01-01T00:00:00Z",
    "message": "0/3 nodes are available"
  }]}}'

  # The grace period is 300s and every poll sleeps 10s.
  for wait in pvc opensearch jaeger; do
    output=$(RESTORE_POD_JSON=$pod_json OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=600 runRestoreWait "$wait")
    rc=$?

    assertNotEquals "$wait" 0 "$rc"
    assertContains "$wait" "$output" \
      "ERROR: Restore pod verify cannot recover by waiting: it has been Unschedulable for over 300s (0/3 nodes are available)"
    assertContains "$wait" "$output" "polls=30"
  done

  output=$(RESTORE_POD_JSON=$pod_json OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=200 runRestoreWait pvc)
  assertNotContains "$output" "cannot recover by waiting"
  assertContains "$output" "ERROR: pvc/restored did not reach {.status.phase}=Bound within 200s"
  assertContains "$output" "polls=20"

  # No other scheduling state is fatal, however long the wait lasts: the
  # scheduler retries after its own error, such as a volume binding timeout, a
  # condition that is not False is not a failure, and a condition with no
  # transition time has no known age.
  for condition in \
    '"status": "False", "reason": "SchedulerError", "lastTransitionTime": "2026-01-01T00:00:00Z"' \
    '"status": "True", "reason": "Unschedulable", "lastTransitionTime": "2026-01-01T00:00:00Z"' \
    '"status": "False", "reason": "Unschedulable"'; do
    pod_json="{\"status\": {\"conditions\": [{\"type\": \"PodScheduled\", $condition}]}}"
    output=$(RESTORE_POD_JSON=$pod_json OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=600 runRestoreWait pvc)
    assertNotContains "$condition" "$output" "cannot recover by waiting"
    assertContains "$condition" "$output" "ERROR: pvc/restored did not reach {.status.phase}=Bound within 600s"
    assertContains "$condition" "$output" "polls=60"
  done
}

testRestoreWaitTimeoutReportsLastErrorAndTermination() {
  pod_json='{"status": {"containerStatuses": [{
    "name": "opensearch",
    "state": {"running": {}},
    "lastState": {"terminated": {"reason": "OOMKilled", "exitCode": 137}}
  }, {
    "name": "jaeger-query",
    "state": {"terminated": {"reason": "Error", "exitCode": 2, "message": "no OpenSearch"}},
    "lastState": {"terminated": {"reason": "Completed", "exitCode": 0, "message": "previous run"}}
  }]}}'

  for wait in pvc opensearch jaeger; do
    case "$wait" in
      pvc) expected="pvc/restored did not reach {.status.phase}=Bound within 30s" ;;
      opensearch) expected="Restored OpenSearch did not recover its primary shards within 30s" ;;
      jaeger) expected="Isolated Jaeger could not query the representative restored service within 30s" ;;
    esac
    output=$(RESTORE_POD_JSON=$pod_json REQUEST_FAILURE="curl: (7) Failed to connect" \
      OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=30 runRestoreWait "$wait")
    rc=$?

    assertNotEquals "$wait" 0 "$rc"
    assertNotContains "$wait" "$output" "cannot recover by waiting"
    # The EXIT trap deletes the pod, so the diagnostics must precede the error.
    assertContains "$wait" "$output" "ERROR: $expected"
    diagnostics=${output%%"ERROR: $expected"*}
    assertContains "$wait" "$diagnostics" "Last request error: curl: (7) Failed to connect"
    assertContains "$wait" "$diagnostics" \
      "Container opensearch last terminated: reason=OOMKilled, exit code=137, message=none"
    assertContains "$wait" "$diagnostics" \
      "Container jaeger-query last terminated: reason=Error, exit code=2, message=no OpenSearch"
    # A container that exited again reports that newest exit, not the run before.
    assertNotContains "$wait" "$diagnostics" "previous run"
    # Every container line carries its own timestamp.
    assertContains "$wait" "$diagnostics" "UTC] Container opensearch last terminated"
    assertContains "$wait" "$diagnostics" "UTC] Container jaeger-query last terminated"

    # A pod that cannot be parsed costs the termination lines, not the rest.
    output=$(RESTORE_POD_JSON='not json' REQUEST_FAILURE="curl: (7) Failed to connect" \
      OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=30 runRestoreWait "$wait")
    assertContains "$wait" "$output" "Last request error: curl: (7) Failed to connect"
    assertNotContains "$wait" "$output" "last terminated"
    assertContains "$wait" "$output" "ERROR: $expected"
  done
}

testRestoreWaitSuccessIsSilent() {
  for wait in pvc opensearch jaeger; do
    output=$(RESTORE_POD_JSON='{"status": {"phase": "Pending"}}' READY_AFTER=0 \
      OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=60 runRestoreWait "$wait")
    assertEquals "$wait" 0 $?
    assertEquals "$wait" "polls=0" "$output"

    # Neither failed requests nor a pod read that fails or returns something
    # other than a pod end or pollute the wait.
    for pod_json in '' 'not json'; do
      output=$(RESTORE_POD_JSON=$pod_json READY_AFTER=2 \
        OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=60 runRestoreWait "$wait")
      assertEquals "$wait '$pod_json'" 0 $?
      assertEquals "$wait '$pod_json'" "polls=2" "$output"
    done
  done
}

testSnapshotWaitDoesNotDependOnRestorePod() {
  output=$(RESTORE_POD_JSON=$(restorePodWaiting opensearch CrashLoopBackOff) \
    OPENSEARCH_RECOVERY_TIMEOUT_SECONDS=30 runRestoreWait snapshot)

  assertContains "$output" "ERROR: volumesnapshot/recovery did not reach {.status.readyToUse}=true within 30s"
  assertNotContains "$output" "Restore pod"
  assertNotContains "$output" "Last request error"
  assertNotContains "$output" "last terminated"
  assertContains "$output" "polls=3"
}

testRestorePvcWaitWatchesRestorePod() {
  # A claim that binds on first consumer stays Pending until the restore pod is
  # scheduled, so an unschedulable pod shows up in this wait.
  contents=$(cat "$SCRIPT")
  assertContains "$contents" "'{.status.phase}' 'Bound' \"\$restore_pod\""
}

testRestorePodKeepsFinalLogsInTerminationMessage() {
  # Neither OpenSearch nor Jaeger writes /dev/termination-log, so without the
  # fallback the reported termination message would always be empty.
  output=$(bash -c \
    'source "$1"; render_restore_pod verify restored "opensearchproject/opensearch@sha256:abc" "jaegertracing/jaeger@sha256:def" config' \
    _ "$SCRIPT")
  policies=$(printf '%s\n' "$output" | grep -c -- '^      terminationMessagePolicy: FallbackToLogsOnError$')
  containers=$(printf '%s\n' "$output" | grep -c -- '^      image: ')

  assertEquals 2 "$containers"
  assertEquals "$containers" "$policies"
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
