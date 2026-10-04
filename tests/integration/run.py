#!/usr/bin/env python3
"""Start isolated local fixtures, run Go integration tests, and clean up."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[2]
PROJECT = "infradex-integration"
FIXTURE_DIR = ROOT / "tests" / "integration"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--basic", action="store_true", help="Only MySQL and Redis")
    parser.add_argument("--keychain", action="store_true", help="Test disposable OS keychain credentials")
    parser.add_argument("--keep", action="store_true", help="Keep fixture containers for manual testing")
    args = parser.parse_args()
    docker = shutil.which("docker") or "/Applications/Docker.app/Contents/Resources/bin/docker"
    contexts = subprocess.check_output([docker, "context", "ls", "--format", "{{.Name}}"], text=True).splitlines()
    context = os.getenv("INFRADEX_DOCKER_CONTEXT", "desktop-linux" if "desktop-linux" in contexts else "default")
    ctx = json.loads(subprocess.check_output([docker, "context", "inspect", context], text=True))[0]
    endpoint = ctx["Endpoints"]["docker"]["Host"]
    if not endpoint.startswith(("unix://", "npipe://")):
        raise SystemExit("Integration fixtures require a local Docker socket, not a remote daemon.")
    base = [docker, "--context", context]
    compose = base + ["compose", "-p", PROJECT, "-f", str(FIXTURE_DIR / "compose.yaml"), "--profile", "extended", "--profile", "cluster"]
    services = ["mysql", "redis"] if args.basic else ["mysql", "redis", "mongo", "clickhouse", "kafka", "k3s"]
    runtime = FIXTURE_DIR / ".runtime"
    runtime.mkdir(mode=0o700, exist_ok=True)
    kubeconfig = runtime / "kubeconfig"
    checks = {
        "mysql": ["mysqladmin", "ping", "-h", "localhost", "-pinfradex-fixture-only"],
        "redis": ["redis-cli", "-a", "infradex-fixture-only", "ping"],
        "mongo": ["mongosh", "--quiet", "--eval", "db.adminCommand({ping:1})"],
        "clickhouse": ["clickhouse-client", "--password", "infradex-fixture-only", "--query", "SELECT 1"],
        "kafka": ["/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "127.0.0.1:49092", "--list"],
        "k3s": ["kubectl", "get", "--raw=/readyz"],
    }
    try:
        subprocess.run(compose + ["up", "-d"] + services, cwd=ROOT, check=True)
        for service in services:
            deadline = time.monotonic() + 120
            while time.monotonic() < deadline:
                try:
                    result = subprocess.run(compose + ["exec", "-T", service] + checks[service], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=15)
                except subprocess.TimeoutExpired:
                    continue
                if result.returncode == 0:
                    print(f"Ready: {service}", flush=True)
                    break
                time.sleep(2)
            else:
                raise RuntimeError(f"{service} did not become ready within 120 seconds")
        env = dict(os.environ)
        env["INFRADEX_TEST_ENGINES"] = "mysql,redis" if args.basic else "mysql,redis,mongo,clickhouse,kafka"
        if not args.basic:
            data = subprocess.check_output(compose + ["exec", "-T", "k3s", "cat", "/etc/rancher/k3s/k3s.yaml"])
            data = data.replace(b"https://127.0.0.1:6443", b"https://127.0.0.1:49443")
            descriptor = os.open(kubeconfig, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            os.fchmod(descriptor, 0o600)
            with os.fdopen(descriptor, "wb") as output:
                output.write(data)
            env["INFRADEX_TEST_KUBECONFIG"] = str(kubeconfig)
        if args.keychain:
            env["INFRADEX_TEST_KEYCHAIN"] = "1"
        result = subprocess.run(["go", "test", "-race", "-tags", "integration", "-run", "TestIntegration", "-v", "-count=1", "-timeout", "120s", "./internal/workspace"], cwd=ROOT, env=env)
        return result.returncode
    finally:
        if not args.keep:
            subprocess.run(compose + ["down", "--volumes", "--remove-orphans"], cwd=ROOT, check=True)
            kubeconfig.unlink(missing_ok=True)
        else:
            print("Fixtures kept. Clean up with the compose down command in tests/integration/README.md.")


if __name__ == "__main__":
    sys.exit(main())
