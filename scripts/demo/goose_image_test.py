# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from urllib.error import HTTPError, URLError

import goose_image


class GooseImageTests(unittest.TestCase):
    def setUp(self):
        self.env = {"GITHUB_REF": "refs/heads/main"}

    @patch("goose_image.subprocess.run")
    @patch("goose_image.urlopen")
    def test_published_version_skips_login_and_build(self, lookup, run):
        image = goose_image.ensure_image(self.env)
        self.assertEqual(goose_image.image_reference(self.env), image)
        self.assertIn(image.replace(":", "/tags/"), lookup.call_args.args[0])
        self.assertEqual(30, lookup.call_args.kwargs["timeout"])
        run.assert_not_called()

    @patch("goose_image.subprocess.run")
    @patch("goose_image.urlopen", side_effect=HTTPError("url", 404, "absent", {}, None))
    def test_missing_version_publishes_both_registries_and_architectures(self, lookup, run):
        image = goose_image.ensure_image(self.env)
        self.assertTrue(run.call_args_list[0].args[0][1].endswith("docker-login.sh"))
        command = run.call_args_list[1].args[0]
        self.assertEqual(["docker", "buildx", "build", "--platform", "linux/amd64,linux/arm64",
                          "--tag", image, "--tag", "quay.io/" + image, "--push"], command[:-1])
        self.assertTrue(run.call_args.kwargs["check"])
        self.assertEqual(self.env, run.call_args.kwargs["env"])

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
