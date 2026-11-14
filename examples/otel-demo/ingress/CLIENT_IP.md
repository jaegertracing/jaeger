<!--
Copyright (c) 2026 The Jaeger Authors.
SPDX-License-Identifier: Apache-2.0
-->

# Client IPs behind the OCI load balancer

The public demo uses an OCI load balancer with TCP listeners on ports 80 and
443, followed by the shared ingress-nginx controller. TLS terminates in NGINX.
Without PROXY protocol, NGINX sees a private load-balancer address, so the AI
ingress's per-IP allowances are shared by unrelated visitors.

Configure PROXY protocol v2 on the existing LoadBalancer Service and on the
controller. Trust only the load balancer's verified subnet CIDRs. Keep
`use-forwarded-headers` disabled so visitors cannot choose their rate-limit
identity through HTTP headers. `externalTrafficPolicy: Local` alone does not
recover the client address behind an OCI TCP load balancer.

Set the Service's `oci.oraclecloud.com/ingress-ip-mode` annotation to `proxy`.
With the default `VIP` mode, in-cluster calls to the public load-balancer IP can
bypass OCI and reach NGINX without a PROXY header. Routing those calls through
OCI preserves public-hostname access for pods, including certificate checks.
See [OCI IP-mode routing](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

This is a one-time infrastructure prerequisite, separate from the Jaeger Helm
release. It affects every hostname using this controller. Arrange a short
maintenance window: the listener and controller changes are asynchronous, and
requests can fail while their protocol settings differ. Keep Cloud Shell open
until both sides and public browsing have been verified.

Oracle documents that PROXY protocol cannot be disabled on an existing classic
load-balancer listener. Removing its Service annotation does not restore the
old listener behavior. Do not use a blind ConfigMap rollback after enabling it.
See [OCI listener configuration](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm)
and [ingress-nginx PROXY protocol](https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/configmap/#use-proxy-protocol).

## Prepare and validate

Use an explicit context and the OCI load balancer whose public address matches
`ingress-nginx/ingress-nginx-controller`. The commands require `oci`, `kubectl`,
and `jq`. Save the original controller, Service, and listener configuration:

```bash
set -euo pipefail
demo_context=YOUR_OKE_CONTEXT
demo_lb_id=YOUR_LOAD_BALANCER_OCID
demo_backup=$(umask 077; mktemp -d)
kubectl --context "$demo_context" -n ingress-nginx get service ingress-nginx-controller -o json >"$demo_backup/service.json"
kubectl --context "$demo_context" -n ingress-nginx get configmap ingress-nginx-controller -o json >"$demo_backup/controller.json"
oci lb load-balancer get --load-balancer-id "$demo_lb_id" >"$demo_backup/load-balancer.json"
```

Check the public IP, TCP listeners, backend sets, and health checks in these
files before proceeding. Determine trusted CIDRs from OCI's actual subnet
resources, rather than inferring a subnet mask from an access-log address:

```bash
demo_proxy_cidrs=$(
  jq -er '.data."subnet-ids"[]' "$demo_backup/load-balancer.json" |
    while IFS= read -r demo_subnet; do
      oci network subnet get --subnet-id "$demo_subnet" \
        --query 'data."cidr-block"' --raw-output || exit 1
    done | paste -sd, -
)
test -n "$demo_proxy_cidrs"
jq -n --arg cidrs "$demo_proxy_cidrs" '{data: {
  "use-proxy-protocol": "true",
  "use-forwarded-headers": "false",
  "proxy-real-ip-cidr": $cidrs
}}' >"$demo_backup/controller-patch.json"
jq -n '{metadata: {annotations: {
  "service.beta.kubernetes.io/oci-load-balancer-connection-proxy-protocol-version": "2",
  "oci.oraclecloud.com/ingress-ip-mode": "proxy"
}}}' >"$demo_backup/service-patch.json"
kubectl --context "$demo_context" -n ingress-nginx patch service ingress-nginx-controller \
  --type=merge --patch-file "$demo_backup/service-patch.json" --dry-run=server >/dev/null
kubectl --context "$demo_context" -n ingress-nginx patch configmap ingress-nginx-controller \
  --type=merge --patch-file "$demo_backup/controller-patch.json" --dry-run=server >/dev/null
```

Review the generated patches. Never use `0.0.0.0/0` or `::/0` as the trusted
proxy range. These merge patches preserve unrelated settings, Service ports,
the public IP, TLS certificates, and the controller Deployment.

## Coordinate the cutover

Run the two prepared updates together during the maintenance window:

```bash
kubectl --context "$demo_context" -n ingress-nginx patch service ingress-nginx-controller \
  --type=merge --patch-file "$demo_backup/service-patch.json"
kubectl --context "$demo_context" -n ingress-nginx patch configmap ingress-nginx-controller \
  --type=merge --patch-file "$demo_backup/controller-patch.json"
```

Wait for OCI's TCP-80 and TCP-443 listeners to report
`backend-tcp-proxy-protocol-version: 2`, and for ingress-nginx to reload its
configuration successfully. Inspect the running `/etc/nginx/nginx.conf` for
PROXY-enabled listeners, `real_ip_header proxy_protocol`, and `set_real_ip_from`
entries matching the verified subnet CIDRs. Keep the saved files until all
public checks pass.

The Service status must also report `ipMode: Proxy`. Test HTTPS to the public
Jaeger hostname from a pod as well as from an external client; external success
alone does not establish that in-cluster routing works.

If one update fails, inspect both sides before recovering. Prefer completing
the matching v2 configuration. Restore the original ConfigMap only when the
load balancer still uses its original protocol. Returning an enabled classic
listener to its original protocol requires recreating that listener from its
saved configuration, with another maintenance window; removing the annotation
is insufficient. Do not delete the load balancer or its Service as a shortcut.

## Verify through the public endpoint

1. Search for a service and open a trace in the browser. Verify `/search`,
   `/api/v3/services`, trace search, and trace retrieval return
   successful responses. Check the other hostnames on the shared controller.
2. Send two service-list requests with distinct test user agents, one with
   forged `X-Forwarded-For`, `X-Real-IP`, and `Forwarded` values. Inspect only
   those requests in the controller access log. Both must show the caller's
   actual public address, with the forged values having no effect. Repeat from
   a second external client to establish separate client identities.
3. Before enabling public AI, apply and verify the exact chat ingress policy.
   With the assistant disabled, malformed or empty chat bodies can exercise
   ingress throttling without invoking a model. A short burst should reach the
   configured rejection status, normally HTTP 503. NGINX's RPM limit includes
   its configured burst allowance; it is not a strict rolling-window counter.
4. Hold two request bodies open after sending complete HTTP headers, then send
   a third request from the same IP. Confirm a connection-limit rejection in
   the controller log, rather than attributing an RPM rejection to concurrency.
   Close the held requests afterward. Keep the test bounded to a few seconds.
5. While chat is throttled, repeat normal service queries and trace browsing.
   They must remain available through their separate ingress location.

Keep the assistant disabled if source-IP handling or either limit fails.
Use the Jaeger-only upgrade path and repeat the public checks against each
deployed configuration. Verify one bounded
MCP-backed answer and project-attributed usage as described in
[AI_ASSISTANT.md](../AI_ASSISTANT.md).
