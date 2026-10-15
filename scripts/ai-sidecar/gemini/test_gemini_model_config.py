# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import sys
from unittest.mock import MagicMock, patch

import pytest
from google.genai import errors as genai_errors

import main
from sidecar_config import SidecarConfig


def _parse_model_name(argv: list[str], env: dict[str, str]) -> str:
    with patch.object(sys, "argv", ["main.py", *argv]), patch.dict("os.environ", env, clear=True):
        return main.parse_args().gemini_model_name


class TestGeminiModelNameDefaulting:
    def test_uses_default_when_env_var_unset(self) -> None:
        assert _parse_model_name([], env={}) == main.DEFAULT_GEMINI_MODEL_NAME

    def test_uses_default_when_env_var_is_empty_string(self) -> None:
        # An explicitly-empty GEMINI_MODEL_NAME (e.g. `export GEMINI_MODEL_NAME=`)
        # must fall back to the default rather than being treated as an override.
        assert _parse_model_name([], env={"GEMINI_MODEL_NAME": ""}) == main.DEFAULT_GEMINI_MODEL_NAME

    def test_uses_env_var_when_set(self) -> None:
        assert _parse_model_name([], env={"GEMINI_MODEL_NAME": "gemini-1.5-pro"}) == "gemini-1.5-pro"

    def test_cli_flag_overrides_env_var(self) -> None:
        assert (
            _parse_model_name(
                ["--gemini-model-name", "gemini-2.0-flash"],
                env={"GEMINI_MODEL_NAME": "gemini-1.5-pro"},
            )
            == "gemini-2.0-flash"
        )


class TestSidecarConfigValidation:
    def _config(self, gemini_model_name: str) -> SidecarConfig:
        return SidecarConfig(
            gemini_api_key="fake-key",
            gemini_model_name=gemini_model_name,
            mcp_url="http://127.0.0.1:16687/mcp",
            mcp_discovery_timeout_sec=15.0,
            otlp_endpoint="http://localhost:4317",
            otlp_insecure=True,
        )

    def test_rejects_empty_model_name(self) -> None:
        with pytest.raises(RuntimeError, match="GEMINI_MODEL_NAME"):
            self._config("").validate()

    def test_accepts_non_empty_model_name(self) -> None:
        self._config("gemini-2.5-flash").validate()  # should not raise


class TestValidateGeminiModelName:
    def _config(self) -> SidecarConfig:
        return SidecarConfig(
            gemini_api_key="fake-key",
            gemini_model_name="gemini-2.5-flash",
            mcp_url="http://127.0.0.1:16687/mcp",
            mcp_discovery_timeout_sec=15.0,
            otlp_endpoint="http://localhost:4317",
            otlp_insecure=True,
        )

    def test_passes_for_valid_model(self) -> None:
        fake_client = MagicMock()
        fake_client.models.get.return_value = MagicMock()
        with patch.object(main.genai, "Client", return_value=fake_client):
            main._validate_gemini_model_name(self._config())
        fake_client.models.get.assert_called_once_with(model="gemini-2.5-flash")

    def test_raises_clear_error_for_invalid_model(self) -> None:
        fake_client = MagicMock()
        fake_client.models.get.side_effect = genai_errors.APIError(
            code=404, response_json={"error": {"message": "model not found"}}
        )
        with patch.object(main.genai, "Client", return_value=fake_client):
            with pytest.raises(RuntimeError, match="gemini-2.5-flash"):
                main._validate_gemini_model_name(self._config())