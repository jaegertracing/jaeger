# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from urllib.error import HTTPError, URLError

import goose_image

DIGEST = "sha256:" + "a" * 64


class GooseImageTests(unittest.TestCase):
    def setUp(self):
        self.env = {"GITHUB_REF": "refs/heads/main"}

    @patch("goose_image.subprocess.run")
    @patch("goose_image.urlopen")
    def test_published_version_skips_login_and_build(self, lookup, run):
        lookup.return_value.__enter__.return_value = io.BytesIO(json.dumps({"digest": DIGEST}).encode())
        image = goose_image.ensure_image(self.env)
        tagged = goose_image.image_reference(self.env)
        self.assertEqual(tagged + "@" + DIGEST, image)
        self.assertIn(tagged.replace(":", "/tags/"), lookup.call_args.args[0])
        self.assertEqual(30, lookup.call_args.kwargs["timeout"])
        run.assert_not_called()

    @patch("goose_image.subprocess.run")
    @patch("goose_image.urlopen")
    def test_published_version_without_digest_is_rejected(self, lookup, run):
        lookup.return_value.__enter__.return_value = io.BytesIO(b"{}")
        with self.assertRaises(RuntimeError):
            goose_image.ensure_image(self.env)
        run.assert_not_called()

    @patch("goose_image.subprocess.run")
    @patch("goose_image.urlopen", side_effect=HTTPError("url", 404, "absent", {}, None))
    def test_missing_version_publishes_both_registries_and_architectures(self, lookup, run):
        def build(command, **kwargs):
            if "--metadata-file" in command:
                Path(command[command.index("--metadata-file") + 1]).write_text(
                    json.dumps({"containerimage.digest": DIGEST}))
        run.side_effect = build
        image = goose_image.ensure_image(self.env)
        tagged = goose_image.image_reference(self.env)
        self.assertEqual(tagged + "@" + DIGEST, image)
        self.assertTrue(run.call_args_list[0].args[0][1].endswith("docker-login.sh"))
        command = run.call_args_list[1].args[0]
        self.assertEqual(["docker", "buildx", "build", "--platform", "linux/amd64,linux/arm64",
                          "--tag", tagged, "--tag", "quay.io/" + tagged, "--push", "--metadata-file"], command[:-2])
        self.assertTrue(all(call.kwargs["check"] for call in run.call_args_list[:2]))
        self.assertTrue(all(call.kwargs["env"] == self.env for call in run.call_args_list))
        self.assertEqual([["docker", "logout", "docker.io"], ["docker", "logout", "quay.io"]],
                         [call.args[0] for call in run.call_args_list[2:]])

    @patch("goose_image.subprocess.run")
    @patch("goose_image.urlopen", side_effect=HTTPError("url", 404, "absent", {}, None))
    def test_failed_publication_still_logs_out(self, lookup, run):
        def fail(command, **kwargs):
            if command[:3] == ["docker", "buildx", "build"]:
                raise goose_image.subprocess.CalledProcessError(1, command)
        run.side_effect = fail
        with self.assertRaises(goose_image.subprocess.CalledProcessError):
            goose_image.ensure_image(self.env)
        self.assertEqual(["docker", "logout", "quay.io"], run.call_args.args[0])

    @patch("goose_image.ensure_image", return_value="example/goose:v1@" + DIGEST)
    def test_command_exports_the_reference_to_later_steps(self, ensure):
        with tempfile.NamedTemporaryFile(mode="r") as output:
            goose_image.main(dict(self.env, GITHUB_OUTPUT=output.name))
            self.assertEqual("image=example/goose:v1@" + DIGEST + "\n", output.read())

    @patch("goose_image.subprocess.run")
    @patch("goose_image.urlopen")
    def test_unknown_registry_state_never_publishes(self, lookup, run):
        for error in (HTTPError("url", 429, "throttled", {}, None), URLError("offline"), TimeoutError()):
            with self.subTest(error=error), self.assertRaises(RuntimeError):
                lookup.side_effect = error
                goose_image.ensure_image(self.env)
        run.assert_not_called()

    @patch("goose_image.subprocess.run")
    @patch("goose_image.urlopen")
    def test_other_refs_are_rejected_before_registry_access(self, lookup, run):
        for ref in (None, "refs/heads/feature", "refs/tags/v1.0.0"):
            with self.subTest(ref=ref), self.assertRaises(ValueError):
                goose_image.ensure_image({"GITHUB_REF": ref})
        lookup.assert_not_called()
        run.assert_not_called()

    def test_version_and_dockerfile_content_determine_cache_tag(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            dockerfile = root / goose_image.DOCKERFILE
            dockerfile.parent.mkdir(parents=True)
            dockerfile.write_text("ARG GOOSE_VERSION=v1.57.0\n")
            original = goose_image.image_reference(self.env, root)
            self.assertRegex(original, r":v1\.57\.0-[0-9a-f]{12}$")
            dockerfile.write_text(dockerfile.read_text() + "# changed extension configuration\n")
            self.assertNotEqual(original, goose_image.image_reference(self.env, root))
            dockerfile.write_text("ARG GOOSE_VERSION=v1.58.0\n")
            self.assertIn(":v1.58.0-", goose_image.image_reference(self.env, root))
            dockerfile.write_text("ARG GOOSE_VERSION=latest\n")
            with self.assertRaises(ValueError):
                goose_image.image_reference(self.env, root)
        with self.assertRaises(ValueError):
            goose_image.image_reference(dict(self.env, JAEGER_DEMO_GOOSE_IMAGE_REPOSITORY="invalid:tag"))


if __name__ == "__main__":
    unittest.main()
