# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import deploy


class RecoveryCommandTests(unittest.TestCase):
    def execute(self, helper, arguments, status=0):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            fake = root / "command"
            fake.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
command = Path(sys.argv[0]).name
with open(os.environ["TEST_COMMAND_LOG"], "a") as output:
    output.write(json.dumps([command] + sys.argv[1:]) + "\\n")
if command == "curl":
    raise SystemExit("curl must run in the pod, not on the deployment host")
print('{"status":"ok"}')
raise SystemExit(int(os.environ["TEST_COMMAND_STATUS"]))
''')
            fake.chmod(0o755)
            for name in ("kubectl", "curl"):
                (root / name).symlink_to(fake)
            log = root / "commands.jsonl"
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"],
                       TEST_COMMAND_LOG=str(log), TEST_COMMAND_STATUS=str(status),
                       OPENSEARCH_RECOVERY_NAMESPACE="test-namespace",
                       OPENSEARCH_RECOVERY_STATEFULSET="source-search")
            script = deploy.ROOT / "examples/otel-demo/opensearch-recovery.sh"
            result = subprocess.run(["bash", "-c", 'source "$1"; shift; "$@"', "_",
                                     str(script), helper, *arguments], env=env, text=True, capture_output=True)
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            return result, calls

    def test_api_commands_execute_in_the_selected_container(self):
        cases = (
            ("opensearch_api", ["POST", "_flush?wait_if_ongoing=true"], "source-search-0",
             ["-X", "POST", "http://127.0.0.1:9200/_flush?wait_if_ongoing=true"]),
            ("restored_api", ["restore-test", "_cat/indices/jaeger-*?h=index"], "restore-test",
             ["http://127.0.0.1:9200/_cat/indices/jaeger-*?h=index"]),
            ("jaeger_api", ["restore-test", "api/v3/traces/012345"], "restore-test",
             ["http://127.0.0.1:16686/api/v3/traces/012345"]),
        )
        for helper, arguments, pod, suffix in cases:
            with self.subTest(helper=helper):
                result, calls = self.execute(helper, arguments)
                self.assertEqual(0, result.returncode, result.stderr)
                self.assertEqual('{"status":"ok"}\n', result.stdout)
                self.assertEqual([["kubectl", "-n", "test-namespace", "exec", pod, "-c", "opensearch", "--",
                                   "curl", "--fail", "--silent", "--show-error", *suffix]], calls)

    def test_remote_command_failure_is_propagated_without_local_curl(self):
        for helper, arguments in (("opensearch_api", ["GET", ""]),
                                  ("restored_api", ["restore-test", ""]),
                                  ("jaeger_api", ["restore-test", "api/v3/services"])):
            with self.subTest(helper=helper):
                result, calls = self.execute(helper, arguments, status=23)
                self.assertEqual(23, result.returncode)
                self.assertEqual(["kubectl"], [call[0] for call in calls])


if __name__ == "__main__":
    unittest.main()
