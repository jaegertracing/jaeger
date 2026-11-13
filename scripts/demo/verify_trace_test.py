# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import base64
import json
import unittest
from unittest.mock import MagicMock, patch
import urllib.error

import verify_trace


class TraceVerificationTests(unittest.TestCase):
    def test_finds_hex_and_base64_trace_ids_in_streaming_responses(self):
        trace_id = "ab" * 16
        for value in (trace_id, base64.b64encode(bytes.fromhex(trace_id)).decode()):
            self.assertTrue(verify_trace.contains_trace({"result": [{"traceId": value}]}, trace_id))
        self.assertFalse(verify_trace.contains_trace({"traceId": "other"}, trace_id))
        self.assertFalse(verify_trace.contains_trace(None, trace_id))

    @patch("verify_trace.urllib.request.urlopen")
    def test_json_transport(self, urlopen):
        urlopen.return_value.__enter__.return_value.read.return_value = b'{"ok":true}'
        self.assertEqual({"ok": True}, verify_trace.request("http://test", {"span": 1}))
        self.assertEqual({"span": 1}, json.loads(urlopen.call_args.args[0].data))

    @patch("verify_trace.time.sleep")
    @patch("verify_trace.request")
    def test_writes_once_then_waits_for_exact_trace(self, request, sleep):
        trace_id = "12" * 16
        request.side_effect = [urllib.error.URLError("not ready"), {}, {"result": []}, {"result": [{"traceId": trace_id}]}]
        with patch("verify_trace.uuid.uuid4") as uuid:
            uuid.return_value.hex = trace_id
            verify_trace.check_trace("collector", "query", attempts=3)
        self.assertEqual(4, request.call_count)
        span = request.call_args_list[1].args[1]["resourceSpans"][0]["scopeSpans"][0]["spans"][0]
        self.assertEqual(trace_id, span["traceId"])
        self.assertEqual(2, sleep.call_count)

    @patch("verify_trace.time.sleep")
    @patch("verify_trace.request")
    def test_missing_trace_and_rejected_ingestion_fail(self, request, sleep):
        request.return_value = {}
        with self.assertRaises(RuntimeError):
            verify_trace.check_trace("collector", "query", attempts=1)
        request.return_value = {"partialSuccess": {"rejectedSpans": "1"}}
        with self.assertRaisesRegex(RuntimeError, "rejected"):
            verify_trace.check_trace("collector", "query", attempts=1)

    @patch("verify_trace.subprocess.run")
    @patch("verify_trace.request")
    def test_rejects_wrong_server_or_dashboard_version(self, request, run):
        run.return_value.stdout = '{"version":{"number":"3.7.0"}}'
        with self.assertRaisesRegex(RuntimeError, "OpenSearch"):
            verify_trace.verify("3.9.0")
        run.return_value.stdout = '{"version":{"number":"3.9.0"}}'
        request.return_value = {"version": {"number": "3.7.0"}}
        with self.assertRaisesRegex(RuntimeError, "Dashboards"):
            verify_trace.verify("3.9.0")

    @patch("verify_trace.check_trace")
    @patch("verify_trace.subprocess.Popen")
    @patch("verify_trace.subprocess.run")
    @patch("verify_trace.request")
    def test_cleans_up_port_forwards_even_when_trace_verification_fails(self, request, run, popen, check):
        run.return_value.stdout = '{"version":{"number":"3.9.0"}}'
        request.return_value = {"version": {"number": "3.9.0"}}
        first, second = MagicMock(), MagicMock()
        popen.side_effect = [first, second]
        check.side_effect = RuntimeError("trace failed")
        with self.assertRaisesRegex(RuntimeError, "trace failed"):
            verify_trace.verify("3.9.0")
        first.__enter__.return_value.terminate.assert_called_once()
        second.__enter__.return_value.terminate.assert_called_once()


if __name__ == "__main__":
    unittest.main()
