# Public demo AI assistant

The public demo connects Jaeger Query to Goose over ACP. Goose uses OpenAI
directly and calls Jaeger's MCP tools to inspect real services and traces.
Provider credentials never reach the Jaeger browser client. Once deployed, ordinary
tracing remains available if the agent stops or OpenAI rejects its requests. A new
Helm rollout still waits for Goose: an image-pull or startup failure blocks the
release and can trigger its atomic rollback.

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
3. The demo deployment workflow publishes Goose only from trusted `main`, when
   the selected components include OTel Jaeger (or a full OTel clean install).
   OCI-only, application-only, and storage-only updates do not require it.
   The image tag combines the Dockerfile's pinned Goose version and content
   hash. A published Docker Hub tag is reused; a missing tag triggers one
   multi-platform build for Docker Hub and Quay. Lookup errors abort that
   component rather than treating an unknown cache state as a missing image.
   The resolved reference is passed directly as `JAEGER_DEMO_GOOSE_IMAGE`.
   Daily reconciliation refreshes both Jaeger stacks and reuses the Goose tag.

   The [Dockerfile](../../scripts/ai-sidecar/goose/Dockerfile) verifies pinned
   release-archive SHA256 checksums before extraction and disables all bundled
   extensions. Update the version and both architecture checksums together.
   Configuration changes produce a new image tag. The init container copies
   the disabled-extension configuration into its writable runtime volume.

   For deployment outside CI, publish that Dockerfile with `docker buildx build`
   for the cluster's architecture and supply its explicit image digest. Image
   publication alone does not enable AI; credentials, account checks, ingress
   prerequisites, and an OTel Jaeger deployment are still required.
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

   In Oracle Cloud Shell, create a private file with
   `demo_env=$(umask 077; mktemp)`, enter the two values using an editor, then run
   the same command with `--from-env-file="$demo_env"` and the verified OKE
   context. Remove the file with `rm "$demo_env"` after installation.
   Subsequent CI deployments reuse the Kubernetes Secret; neither credential
   needs to be stored in GitHub Actions secrets.
5. Complete the shared ingress [client-IP setup](ingress/CLIENT_IP.md) before
   activating the assistant. The OCI TCP load balancer needs PROXY protocol
   paired with the controller's configuration; otherwise unrelated visitors
   share the proxy address's limits. Verify real client addresses and rejection
   of forged forwarding headers through the public endpoint.
6. Use the existing Jaeger-only upgrade path:

   ```bash
   JAEGER_DEMO_GOOSE_IMAGE=YOUR_REGISTRY/jaeger-demo-goose@sha256:DIGEST \
   JAEGER_OTEL_DEMO_DEPLOY_SCOPE=jaeger \
     examples/otel-demo/deploy-all.sh upgrade IMAGE_TAG
   ```

   The environment variable is set by the workflow automatically. An empty
   image is rejected during chart rendering before any workloads are changed.

   Before changing releases, the script applies the Jaeger ingress manifest
   and checks that the IngressClass uses ingress-nginx. A missing manifest,
   failed apply, or wrong controller aborts the upgrade and scales an existing
   Goose deployment to zero. If Kubernetes cannot stop Goose, revoke its OpenAI
   key before retrying. Actual controller enforcement and client-IP handling
   require the rollout checks below.

   Upgrade preserves the credential Secret. Clean mode saves it to a mode-0600
   temporary file and restores it after recreating the namespace. A failed
   restore retains that file only for the lifetime of the host or CI runner.
   Hosted-runner cancellation or loss can destroy the backup; keep an independent
   secure copy and be prepared to repeat bootstrap. Clean mode deletes trace
   storage. OpenSearch-only and application-only upgrade scopes are unaffected.

## Limits and access

The provider, model, reasoning, token, and turn settings are defined in
[`jaeger-values.yaml`](jaeger-values.yaml). Automatic session naming is disabled
so it cannot generate background calls. The token setting bounds each model
response; a question can make multiple model calls. Use the project spending
control as the monetary ceiling. Do not enable fallback to another provider or
more expensive models.

Goose has a ClusterIP service and an ingress NetworkPolicy allowing Jaeger pods.
**The cluster CNI must enforce NetworkPolicy** for that restriction to work;
verify it before public rollout. Goose runs without a Kubernetes service-account
token, with a read-only root filesystem and bounded writable volume. It receives
only Jaeger's tools, including browser navigation tools.

The separate chat ingress and its current request, connection, burst, and timeout
limits are defined in [`ingress-jaeger.yaml`](ingress/ingress-jaeger.yaml).
These annotations apply only to the exact chat path. Normal tracing UI, REST
queries, HotROD, and internal MCP callbacks have separate routes. SSE buffering
is disabled.

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
- Confirm CNI enforcement with two short TCP probes to `jaeger-goose:16688`
  from disposable pods in the Jaeger namespace. A pod carrying both the Jaeger
  release and `all-in-one` component labels must connect; an otherwise identical
  unlabeled pod must time out. Remove both probe pods afterward. Do not infer
  isolation from the mere presence of the NetworkPolicy object.
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

`JAEGER_TEST_HELM_CHART` can point at a prepared chart checkout to avoid cloning.
Tests require the same commit pinned in `scripts/demo/jaeger-chart.sh`.

## Operate and disable

- **Spend:** select `jaeger-demo` in OpenAI's Usage dashboard. Change the project
  cap and alerts there, without editing Jaeger. Verify the grant's renewal and
  expiry separately; a monthly spend limit does not reserve a separate wallet
  of credits or necessarily align with the grant's renewal date.
- **Rotate the provider key:** update the private env file, then apply the Secret
  in place using an explicit context:

  ```bash
  kubectl --context YOUR_OKE_CONTEXT -n jaeger create secret generic jaeger-demo-ai \
    --from-env-file=/absolute/path/to/private-demo.env --dry-run=client -o yaml |
    kubectl --context YOUR_OKE_CONTEXT apply -f - >/dev/null
  ```

  Do not enable shell tracing or log the generated manifest. Then
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
