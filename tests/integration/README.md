# Local integration fixtures

Run from the repository root:

```sh
python3 tests/integration/run.py             # All seven connection profiles
python3 tests/integration/run.py --basic     # MySQL + Redis only
python3 tests/integration/run.py --keychain  # Also verify the real OS keychain
```

Requires Python 3, Go, Docker Compose v2, and a local Docker daemon. The runner chooses `desktop-linux` when available, otherwise `default`; override with `DEVOPSPAD_DOCKER_CONTEXT`. Remote Docker endpoints are rejected. No existing kubeconfig or cluster is used.

The fixed Compose project `devopspad-integration` is disposable. Only its containers and volumes are created or removed. Services bind to loopback on ports 49306 (MySQL), 49379 (Redis), 49017 (MongoDB), 48123 (ClickHouse), 49092 (Kafka), and 49443 (K3s). A port conflict fails instead of stopping another service. Fixture passwords are public test-only values; never use them in a real deployment.

The suite seeds dedicated databases, Redis DB 14, a dedicated Kafka topic, and fixture ConfigMaps, Pods, Events and RBAC objects in the isolated K3s server. MongoDB test users and cluster objects are removed after tests. Tests verify authentication, MySQL cancellation and read-only transactions, database scope, table schema/index/DDL inspection, Redis SCAN pagination and six value types, MongoDB authSource, Extended JSON, projection, sort and bidirectional batches, Kafka metadata, and ClickHouse reads/metadata/read-only enforcement. Kubernetes and K3s profiles are both tested against the same isolated K3s API server. Context discovery, namespace isolation, resource details, UID-filtered events, and an impersonated reader with restricted event/log RBAC are also verified. An unscheduled Pod can return an empty HTTP 200 log stream. The server runs without an agent or privileged container; this covers API connectivity and resource listing, not workload scheduling, real running-container log output, other Kubernetes distributions, or all authentication providers. Positive log output, tail/previous/container options, truncation and cancellation use an HTTP test server in `cluster_inspector_test.go`; a real kubelet log smoke test remains outstanding.

The suite also starts an in-process SSH protocol server on an ephemeral loopback port and runs all seven live engine profiles through authenticated `direct-tcpip` channels. Private `.invalid` target names resolve only inside this fixture, verifying remote DNS forwarding. It covers test/query/browse/scope discovery, MySQL query cancellation, and kubeconfig CA/TLS identity preservation. It does not require or alter a system SSH daemon. Unit/protocol tests separately cover password, encrypted/unencrypted private keys, SSH Agent, known_hosts and fingerprint rejection, channel deadlines, cancellation, no direct fallback, and tunneled cluster events/log output.

The optional keychain test creates a random entry under the `devopspad` service, reads it, and deletes it. It does not inspect or modify saved connection credentials. Interactive OS authorization may be required. Native file-dialog interactions remain a manual desktop check.

By default, containers and volumes are removed in `finally`, including after test failure. Add `--keep` for manual inspection. To clean up kept fixtures:

```sh
docker --context desktop-linux compose -p devopspad-integration \
  -f tests/integration/compose.yaml --profile extended --profile cluster \
  down --volumes --remove-orphans
```

The temporary kubeconfig lives in `.runtime/` with owner-only permissions, is Git-ignored, and is removed by the runner during cleanup. Run the runner without `--keep` to also clean up credentials after a manual session.
