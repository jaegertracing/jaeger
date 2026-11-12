# Public demo AI assistant

The public demo connects Jaeger Query to Goose over ACP. Goose uses OpenAI
directly and calls Jaeger's MCP tools to inspect real services and traces.
Provider credentials never reach the Jaeger browser client. Ordinary tracing remains
available if the agent is stopped or its OpenAI project exhausts its budget.

## Provision before deployment

1. In the OpenAI organization holding the API credits, create a `jaeger-demo`
   project and a dedicated service-account key. Confirm that the credit grant
   covers direct API usage and that the project can access `gpt-6-luna`.
   Restrict the project to that model, and restrict the key to **List models:
   Read** and **Responses: Write**. Other key permissions can remain disabled.
   Set a key expiration no later than the grant's expiration and record the
   rotation deadline; a 90-day key lifetime is suitable when the grant lasts longer.
2. In the project's Limits settings, set a **$2,000 monthly spending limit**
   and enable **Enforce a hard limit**. Configure spending alerts at $1,000,
   $1,500, and $1,800. A notification-only budget does not stop requests.
   Enforcement can lag slightly, so leave headroom below the available credits.
   See [OpenAI spending controls](https://developers.openai.com/api/docs/guides/spend-limits).

   For grant-funded operation, leave automatic credit reload disabled and verify
   that the organization's billing page says requests stop when its credit
   balance reaches zero. Remove saved payment methods if the organization must
   not purchase credits. Keep the spending cap below the remaining grant and
   review it before the grant expires or the monthly cap resets. A monthly cap
   does not reserve a separate credit balance. OpenAI's approved monthly usage
   limit and model rate limits still apply independently of that cap; a credit
   grant does not necessarily raise them.
3. The existing Docker CI builds Goose on pull requests without publishing.
   Trusted `main` runs publish amd64 and arm64 images to
   `jaegertracing/jaeger-goose-snapshot` on Docker Hub and Quay. The demo workflow
   verifies and resolves a published commit-SHA tag independently of Jaeger and
   HotROD, then passes it as `JAEGER_DEMO_GOOSE_IMAGE`. Scheduled runs may use the
   newest published SHA if the current commit's image is not available yet.

   For a deployment outside CI, build and publish the same Dockerfile and set
   `JAEGER_DEMO_GOOSE_IMAGE` to an explicit digest:

   ```bash
   docker buildx build --platform linux/arm64 \
     --build-arg TARGETARCH=arm64 \
     --tag YOUR_REGISTRY/jaeger-demo-goose:v1.57.0 --push \
     scripts/ai-sidecar/goose
   ```

   The Dockerfile takes the pinned Goose release and disables every bundled
   extension. The deployment's init container copies that configuration into
   its writable runtime volume before Goose starts. Without the copy, mounting
   the volume over `/tmp` would hide the disabled-extension configuration.
4. Prepare a private env file outside the repository, or explicitly ignored by Git, containing
   `OPENAI_API_KEY` and a random `GOOSE_SERVER__SECRET_KEY`. Give the file mode
   `0600`. The latter authenticates Jaeger to Goose and must be distinct from
   the provider key. Verify the intended cluster and its demo ingress before
   installing the Secret. Create the namespace if necessary, then perform the
   one-time installation using an explicit context:

   ```bash
   kubectl --context YOUR_OKE_CONTEXT -n jaeger create secret generic jaeger-demo-ai \
     --from-env-file=/absolute/path/to/private-demo.env
   ```

   Do not echo the env file, use shell tracing, or commit either credential.
   The deployment checks only the names of nonempty Secret entries. It cannot
   verify the OpenAI account's budget or grant; those checks are the operator's
   responsibility before creating the Secret.

   If OKE access is available only in Oracle Cloud Shell, follow the
   [Cloud Shell bootstrap procedure](CLOUD_SHELL.md). Install the Secret once;
   subsequent CI deployments reuse it. The OpenAI and Goose credentials do not
   need to be copied into GitHub Actions secrets.
5. Use the existing Jaeger-only upgrade path:

   ```bash
   JAEGER_DEMO_GOOSE_IMAGE=YOUR_REGISTRY/jaeger-demo-goose@sha256:DIGEST \
   JAEGER_OTEL_DEMO_DEPLOY_SCOPE=jaeger \
     examples/otel-demo/deploy-all.sh upgrade IMAGE_TAG
   ```

   The environment variable is set by the workflow automatically. An empty
   image is rejected during chart rendering before any workloads are changed.

   The normal upgrade preserves the credential Secret. Clean mode saves only
   that Secret to a mode-0600 temporary file, recreates the namespace, and restores
   it before installing the release. The file is removed after successful restore
   and retained at the logged path on failure so an operator can recover it.
   Clean mode still deletes trace storage. OpenSearch-only and OTel-application-only
   upgrade scopes are unaffected.

## Limits and access

Goose uses `GOOSE_PROVIDER=openai`, `GOOSE_MODEL=gpt-6-luna`,
`GOOSE_THINKING_EFFORT=off`, `GOOSE_MAX_TURNS=8`, and `GOOSE_MAX_TOKENS=4096`.
Automatic model-generated session naming is disabled to avoid background calls.
The token setting limits each model response; one question can make multiple
model calls. The monthly project cap is the spending control. Do not enable
automatic fallback to more expensive models or another provider.

Only Jaeger pods can reach Goose's ClusterIP service under the NetworkPolicy.
Goose runs without a Kubernetes service-account token, with a read-only root
filesystem and a bounded writable runtime volume. It receives only the tools
Jaeger announces for a turn, including browser navigation tools.

A separate ingress matches `/api/ai/chat` exactly. It accepts ten requests per
minute and two concurrent connections per client IP, with a burst multiplier
of one. SSE buffering is disabled. These annotations do not cover the tracing
UI, ordinary REST API, HotROD, or the agent's internal MCP callbacks.

Ingress-nginx enforces these limits **per controller replica**. Multiple
replicas multiply the effective allowance, and a distributed client can use
multiple IP addresses. Verify that the ingress sees the real client IP and
trusts forwarding headers only from the actual load balancer. Shared NAT users
share an allowance. Rejected requests normally return HTTP 503 with the
controller's default settings. See
[ingress-nginx rate limiting](https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/annotations/#rate-limiting).

The proxy timeouts limit idle connections, not total turn duration. Output and
turn limits bound ordinary model loops; a canceled request may still incur
charges for work already submitted to OpenAI. Do not automatically replay a
partially consumed streamed answer after an error.

## Verify the rollout

- Confirm the UI advertises `aiAssistant: true` after Goose passes the ACP
  health check. Ask for the available services, then analyze an existing trace
  and navigate to it. Check that the stream contains actual Jaeger MCP calls
  and that the answer uses their results.
- Inspect provider usage in the OpenAI project Usage dashboard. Confirm that
  the requests are billed to `jaeger-demo` and use the intended model.
- Test an over-limit burst and a third concurrent stream from one client IP.
  Confirm that tracing pages and service queries still work.
- Cancel an active stream and verify that the agent stops the turn. Exercise
  quota failures with a simulated provider, rather than exhausting real credits.
- Check browser responses and operational logs for credential leakage. Do not
  deploy the reference stack's credential-logging MCP proxy to the public demo.

Run the configuration tests with the same chart the deployment uses:

```bash
SHUNIT2=/path/to/shunit2 bash examples/otel-demo/jaeger-values.test.sh
SHUNIT2=/path/to/shunit2 bash examples/otel-demo/deploy-all.test.sh
```

`JAEGER_TEST_HELM_CHART` can point at a prepared `v2` chart checkout to avoid
cloning it during rendering tests.

## Operate and disable

- **Spend:** select `jaeger-demo` in OpenAI's Usage dashboard. Change the project
  cap and alerts there, without editing Jaeger. Verify the grant's renewal and
  expiry separately; a monthly spend limit does not reserve a separate wallet
  of credits or necessarily align with the grant's renewal date.
- **Rotate the provider key:** replace the Secret from the private env file,
  restart `deployment/jaeger-goose`, verify a bounded request, then revoke the old
  key. When rotating `GOOSE_SERVER__SECRET_KEY`, restart both `jaeger-goose` and
  `jaeger` because environment references are read at process start.
- **Stop spending immediately:** revoke the project key or disable its API
  access in OpenAI, then scale `deployment/jaeger-goose` to zero. Scaling alone
  is temporary because the next deployment restores the declared replica count.
- **Disable persistently:** set the Goose Deployment's `replicas` to zero in
  `jaeger-values.yaml` and deploy that change. The assistant disappears after
  the agent health check fails. Retain the Secret for subsequent upgrades and
  revoke the provider key to block accidental reuse.
- **Rollback:** restore the previous Helm revision and review whether it still
  enables AI before restoring traffic. Never run a clean deployment as an AI
  rollback; clean mode deletes trace storage.
