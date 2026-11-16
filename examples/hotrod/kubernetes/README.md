# HotROD Kubernetes test fixture

This core-owned fixture tests locally built Jaeger and HotROD images in a disposable
Kind cluster. It contains one memory-backed Jaeger instance, HotROD and their
Services in the `hotrod-test` namespace. It needs no Helm chart, hosted-demo
configuration, external storage, monitoring stack or production credentials.

`config.yaml` and `ui-config.json` are independent test inputs. The Jaeger config
accepts OTLP/HTTP traces and serves the query API/UI under `/jaeger`; the empty UI
config uses the UI defaults without hosted analytics. HotROD runs under `/hotrod`.
These files belong to core CI, not the hosted demo deployment.

## Build and test

The full image test requires a Linux environment with Docker (Compose and Buildx),
Bash 4.4+, Go, Node.js/Yarn, Kind, kubectl, curl and jq. Initialize the repository
submodules, create a disposable Kind cluster and set the build variables from
the checkout being tested:

```bash
git submodule update --init --recursive
kind create cluster --name hotrod-test
export BRANCH="$(git rev-parse --abbrev-ref HEAD)"
export GITHUB_SHA="$(git rev-parse HEAD)"
bash scripts/build/build-hotrod-image.sh -l -p linux/amd64 -r k8s
```

Use `linux/arm64` on an ARM Linux host. The image builder requires `BRANCH` even
with `-l`, which restricts publication to the local registry. The build script
creates the local registry, builds both images, loads them into Kind, applies this fixture, waits for both Deployments, and forwards
ports 8080 and 16686. It checks the HotROD page, dispatches a ride, and verifies
that the returned trace contains at least 35 spans. Its exit trap removes the
fixture namespace and port-forwards; the Kind cluster remains available for reuse.
Use a single Kind cluster, since the existing build script loads the first cluster
returned by `kind get clusters`.

The Docker test path remains available with `-r docker`. Core's image build and
publishing workflow continues to own both paths and publishes HotROD only after
its functional checks pass.

## Use already-built images

Run from the repository root with a disposable cluster selected. Both images must
already exist in the local registry under the same tag:

```bash
export GITHUB_SHA="$(git rev-parse HEAD)"
docker pull "localhost:5000/jaegertracing/jaeger:${GITHUB_SHA}"
docker pull "localhost:5000/jaegertracing/example-hotrod:${GITHUB_SHA}"
kind load docker-image --name hotrod-test \
  "localhost:5000/jaegertracing/jaeger:${GITHUB_SHA}" \
  "localhost:5000/jaegertracing/example-hotrod:${GITHUB_SHA}"
bash examples/hotrod/kubernetes/deploy.sh render "$GITHUB_SHA"
bash examples/hotrod/kubernetes/deploy.sh apply "$GITHUB_SHA"
kubectl -n hotrod-test rollout status deployment/jaeger --timeout=180s
kubectl -n hotrod-test rollout status deployment/jaeger-hotrod --timeout=180s
```

`render` prints the complete manifest without invoking kubectl. Both containers
use `imagePullPolicy: Never`, so a missing loaded image fails instead of falling
back to a published image. To browse the fixture, run each port-forward in its own
terminal:

```bash
kubectl -n hotrod-test port-forward svc/jaeger-query 16686:16686
kubectl -n hotrod-test port-forward svc/jaeger-hotrod 8080:80
```

Open [Jaeger](http://localhost:16686/jaeger/) or
[HotROD](http://localhost:8080/hotrod).

## Cleanup and troubleshooting

Stop manual port-forwards, then remove only the test namespace:

```bash
bash examples/hotrod/kubernetes/deploy.sh delete
# Optional: remove the disposable cluster too.
kind delete cluster --name hotrod-test
```

For a rollout failure, inspect the pod events and both application logs:

```bash
kubectl -n hotrod-test get pods
kubectl -n hotrod-test describe pods
kubectl -n hotrod-test logs deployment/jaeger
kubectl -n hotrod-test logs deployment/jaeger-hotrod
```

`ErrImageNeverPull` means the exact registry/tag was not loaded into the selected
Kind cluster. Connection failures commonly indicate that kubectl targets a
different cluster or that local ports 8080/16686 are occupied. Missing traces
require checking the HotROD OTLP endpoint and Jaeger logs.

The shell regression tests use command stubs and require no cluster or Docker:

```bash
SHUNIT2=/path/to/shunit2 bash examples/hotrod/kubernetes/deploy.test.sh
```

The full shell-test runner also includes this suite. The Docker/Kubernetes CI
matrix provides the real image, networking and trace-ingestion checks.
