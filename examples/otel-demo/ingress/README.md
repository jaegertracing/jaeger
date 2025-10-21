# Ingress Configuration for OpenTelemetry Demo Stack

This directory contains the HTTPS ingress configurations for exposing the observability stack services via NGINX ingress controller with Let's Encrypt SSL certificates.



## Files

- **`clusterissuer-letsencrypt-prod.yaml`** - Let's Encrypt certificate issuer (already deployed)
- **`ingress-jaeger.yaml`** - Exposes Jaeger UI and HotROD demo
- **`ingress-opensearch.yaml`** - Exposes OpenSearch Dashboards
- **`ingress-otel-demo.yaml`** - Exposes OTel Demo Shop (frontend-proxy)

## Exposed Services

| Service | URL | Backend Service | Port |
|---------|-----|-----------------|------|
| Jaeger UI | https://jaeger.demo.jaegertracing.io | jaeger-query-clusterip | 16686 |
| HotROD Demo | https://hotrod.demo.jaegertracing.io | jaeger-hotrod | 80 |
| OpenSearch Dashboards | https://opensearch.demo.jaegertracing.io | opensearch-dashboards | 5601 |
| OTel Demo Shop | https://shop.demo.jaegertracing.io | frontend-proxy | 8080 |
| Load Generator | https://shop.demo.jaegertracing.io/loadgen/ | (via frontend-proxy) | 8080 |

## Certificate Management

Certificates are automatically managed by cert-manager using the Let's Encrypt production issuer.

### View Certificate Status
```bash
kubectl get certificates --all-namespaces
```

### Certificate Secrets
- `jaeger-demo-tls` (namespace: jaeger)
- `opensearch-demo-tls` (namespace: opensearch)
- `otel-demo-tls` (namespace: otel-demo)

### Force Certificate Renewal
```bash
kubectl delete certificate <cert-name> -n <namespace>
# Certificate will be automatically recreated
```

## Prerequisites

- NGINX Ingress Controller (deployed)
-  cert-manager (deployed)
-  ClusterIssuer (letsencrypt-prod) configured
-  DNS records pointing to ingress controller IP (170.9.51.232)

## DNS Configuration

All hostnames must resolve to the NGINX ingress controller external IP:

```
jaeger.demo.jaegertracing.io     -> 170.9.51.232
hotrod.demo.jaegertracing.io     -> 170.9.51.232
opensearch.demo.jaegertracing.io -> 170.9.51.232
shop.demo.jaegertracing.io       -> 170.9.51.232
```

Verify DNS:
```bash
dig jaeger.demo.jaegertracing.io +short
```



## Troubleshooting

### Ingress Not Working
```bash
kubectl get ingress --all-namespaces
kubectl describe ingress <name> -n <namespace>
```

### Certificate Issues
```bash
kubectl describe certificate <cert-name> -n <namespace>
kubectl get certificaterequest -n <namespace>
kubectl get challenge -n <namespace>
```

### Ingress Controller Logs
```bash
kubectl logs -n ingress-nginx -l app.kubernetes.io/name=ingress-nginx
```

## Security Notes

- Load generator is **not** directly exposed to the internet
- Access load generator via frontend-proxy: https://shop.demo.jaegertracing.io/loadgen/
- All certificates are production Let's Encrypt certificates
- Auto-renewal enabled (certificates valid for 90 days) 

