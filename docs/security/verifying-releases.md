# Verifying Jaeger Releases

All Jaeger releases are cryptographically signed. Users should verify signatures before using release artifacts to ensure they have not been tampered with.

## Signed Artifacts

| Artifact Type | Signing Method |
|---------------|----------------|
| Git tags | GPG signed (`git tag -s`) |
| Binary archives | GPG detached signatures (`.asc` files) |
| Container images | Verify image digest from official Docker Hub and Quay.io repositories |
| SBOM | Included with each release |

## Verifying Container Image Authenticity

Jaeger container images are published to official repositories on Docker Hub and Quay.io. To verify that you are using the intended image:

1. Pull images from the official Jaeger organization repositories on Docker Hub or Quay.io.
2. Use image digests (for example, `jaegertracing/all-in-one@sha256:<digest>`) rather than mutable tags where possible.
3. Compare the digest you deploy with the expected digest published in your deployment configuration, automation, or release notes.

## Verifying Binary Signatures

1. **Import the Jaeger GPG public key**:
   The Jaeger public key (`C043A4D2B3F2AC31`) is available on all major key servers. See [SECURITY.md](../../SECURITY.md#our-public-key) for the full key block.

   ```bash
   gpg --keyserver keyserver.ubuntu.com --recv-keys C043A4D2B3F2AC31
   ```

2. **Download the release artifact and its signature**:
   ```bash
   # Example for version v1.55.0
   wget https://github.com/jaegertracing/jaeger/releases/download/v1.55.0/jaeger-1.55.0-linux-amd64.tar.gz
   wget https://github.com/jaegertracing/jaeger/releases/download/v1.55.0/jaeger-1.55.0-linux-amd64.tar.gz.asc
   ```

3. **Verify the signature**:
   ```bash
   gpg --verify jaeger-1.55.0-linux-amd64.tar.gz.asc jaeger-1.55.0-linux-amd64.tar.gz
   ```

## Verifying Git Tag Signatures

You can verify the signature of any Jaeger Git tag using the following commands:

```bash
git fetch --tags
git tag -v v1.55.0
```
