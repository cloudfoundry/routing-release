# Running Routing Acceptance Tests (RATS)

This guide explains how to run the routing acceptance tests locally against a CF-on-Kubernetes deployment
using [kind](https://kind.sigs.k8s.io/).

## Overview

The script [start-local-rats.sh](../bin/start-local-rats.sh) automates the full test lifecycle:

1. Resolves the TCP router IP from the running `cfk8s-worker` Docker container
2. Ensures required CF buildpacks and shared domains are present
3. Runs HTTP and TCP routing acceptance tests via [Ginkgo](https://onsi.github.io/ginkgo/)
4. Cleans up shared domains (and optionally test orgs) after the run

### Test Results Summary

With the correct configuration, you should see:

```
HTTP Routing Tests
- 1 spec skipped (IncludeHttpRoutes=false by default)

TCP Routing Tests  
- 6 specs passed in ~8-9 minutes
  ✓ single external port to single app port
  ✓ single external port to two different apps
  ✓ multiple external ports to single app port
  ✓ multiple external ports to single app port with two apps
  ✓ unmap and remap routes
  ✓ multiple apps sharing same external port

Acceptance Tests Complete; exit status: 0
```

---

## Prerequisites

### System Tools

| Tool | Minimum Version | Purpose |
|------|----------------|---------|
| `docker` | 20+ | Inspects the `cfk8s-worker` kind node to resolve its IP |
| `kind` | 0.20+ | Provides the local Kubernetes cluster running CF |
| `kubectl` | 1.26+ | Interacts with the kind cluster |
| `jq` | 1.6+ | Parses and patches the test config JSON |
| `cf` CLI | v7+ | Manages CF resources (orgs, spaces, routes, domains) |
| `ginkgo` | v2+ | Runs the Go-based acceptance test suites |
| `make` | any | Bootstraps CF buildpacks if not yet uploaded |
| `go` | 1.21+ | Required to build the `rtr` binary and compile test assets |
| `kind-deployment` | any | Required to run the CF on Kind setup. Please checkout the following [documentation](https://github.com/cloudfoundry/kind-deployment?tab=readme-ov-file#run-the-installation) |

Install `ginkgo`:
```bash
go install github.com/onsi/ginkgo/v2/ginkgo@latest
```

### Linux Kernel Parameters

Running CF app containers inside Docker requires sufficient inotify limits on the **host**. Without this,
the Envoy sidecar proxy will crash with exit code 134 (SIGABRT).

Check current values:
```bash
sysctl fs.inotify.max_user_instances fs.inotify.max_user_watches
```

Apply immediately (no reboot needed):
```bash
sudo sysctl -w fs.inotify.max_user_instances=512
sudo sysctl -w fs.inotify.max_user_watches=524288
```

Make permanent:
```bash
echo -e "fs.inotify.max_user_instances=512\nfs.inotify.max_user_watches=524288" \
  | sudo tee /etc/sysctl.d/99-inotify.conf
```

### Running CF-on-kind Cluster

A running CF-on-kind deployment is required. Use the
[kind-deployment](https://github.com/cloudfoundry/kind-deployment) project to set it up.
The script expects a Docker container named **`cfk8s-worker`** to be present and running.

Verify:
```bash
docker inspect cfk8s-worker --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'
```

### `rtr` Binary

The `rtr` (Routing API CLI) binary must be built before running the tests:

```bash
cd src/code.cloudfoundry.org/rtr
go build -o rtr .
```

The script automatically prepends the `rtr` directory to `$PATH`.

---

## Configuration

The tests are configured via a JSON file. A template is provided at `config.json`.

### Config Fields

| Field | Description | Example |
|-------|-------------|---------|
| `addresses` | IPs of the TCP router (auto-set by the script from Docker inspect) | `["172.21.0.3"]` |
| `api` | CF API endpoint | `"api.127-0-0-1.nip.io"` |
| `admin_user` | CF admin username | `"ccadmin"` |
| `admin_password` | CF admin password | `"secret"` |
| `apps_domain` | Default HTTP apps domain | `"apps.127-0-0-1.nip.io"` |
| `tcp_apps_domain` | Shared TCP routing domain | `"tcp.127-0-0-1.nip.io"` |
| `tcp_router_group` | Router group name for TCP routes | `"default-tcp"` |
| `skip_ssl_validation` | Skip TLS certificate verification | `true` |
| `include_http_routes` | Enable HTTP route tests | `false` |
| `default_timeout` | Default test timeout in seconds | `120` |
| `cf_push_timeout` | Timeout for `cf push` in seconds | `120` |
| `oauth.token_endpoint` | UAA token endpoint URL | `"https://uaa.127-0-0-1.nip.io"` |
| `oauth.client_name` | UAA client for routing API auth | `"tcp_emitter"` |
| `oauth.client_secret` | UAA client secret | `"secret"` |
| `oauth.port` | UAA port | `443` |

> [!NOTE]
> The `addresses` field is automatically updated by the script on each run — you do not need to set it manually.

### Verified Working Configuration

A tested and working `config.json` looks like:

```json
{
  "addresses": ["172.21.0.3"],
  "api": "api.127-0-0-1.nip.io",
  "admin_user": "ccadmin",
  "admin_password": "<secret>",
  "skip_ssl_validation": true,
  "use_http": false,
  "apps_domain": "apps.127-0-0-1.nip.io",
  "include_http_routes": false,
  "default_timeout": 120,
  "cf_push_timeout": 120,
  "tcp_apps_domain": "tcp.127-0-0-1.nip.io",
  "tcp_router_group": "default-tcp",
  "oauth": {
    "token_endpoint": "https://uaa.127-0-0-1.nip.io",
    "client_name": "tcp_emitter",
    "client_secret": "<secret>",
    "port": 443,
    "skip_ssl_validation": true
  }
}
```

**Key points:**
- `include_http_routes: false` - HTTP tests are skipped by default
- TCP tests require 6 specs × ~8-9 seconds per spec = ~8 minutes total
- The `addresses` array contains only **one** IP (the Docker bridge IP of `cfk8s-worker`)

---

## Required Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `ACCEPTANCE_DIR` | Absolute path to the `routing-acceptance-tests` directory | `/home/user/routing-release/src/code.cloudfoundry.org/routing-acceptance-tests` |
| `KIND_DEPLOYMENT_DIR` | Absolute path to the `kind-deployment` repository | `/home/user/kind-deployment` |
| `CONFIG` | Absolute path to the test config JSON file | `/home/user/routing-acceptance-tests/config.json` |
| `RTR_BIN` | Absolute path to the directory containing the `rtr` binary | `/home/user/routing-release/src/code.cloudfoundry.org/rtr` |

### Optional Environment Variables

| Variable            | Description                                                      | Example                       |
|---------------------|------------------------------------------------------------------|-------------------------------|
| `VERBOSE`           | When set to any value, enables verbose output for ginkgo tests   | `VERBOSE=1` or `VERBOSE=true` |
| `CLEANUP_CATS_ORGS` | When `true`, deletes all CF orgs matching `CATS-*` after the run | `CLEANUP_CATS_ORGS=true`      |

### Basic Usage

```bash
export ACCEPTANCE_DIR=/home/user/routing-release/src/code.cloudfoundry.org/routing-acceptance-tests
export KIND_DEPLOYMENT_DIR=/home/user/kind-deployment
export CONFIG=$ACCEPTANCE_DIR/config.json
export RTR_BIN=/home/user/routing-release/src/code.cloudfoundry.org/rtr

bash ./start-local-rats.sh
```

---

## TCP Router Port Configuration

The CF TCP routing domain requires available ports in the `default-tcp` router group.
The kind cluster worker node exposes ports `32000–32019` for TCP routing.

Verify the current port range:
```bash
cf curl /routing/v1/router_groups
```

If the range is too small and tests fail with _"There are no more ports available for this domain"_,
expand it:
```bash
ROUTER_GROUP_GUID=$(cf curl /routing/v1/router_groups | jq -r '.[0].guid')
cf curl /routing/v1/router_groups/$ROUTER_GROUP_GUID \
  -X PUT -d '{"reservable_ports":"32000-32019"}'
```

---

## Troubleshooting

### HTTP Tests Skipped: `IncludeHttpRoutes is set to false`

This is **expected behavior** when `include_http_routes: false` in `config.json`:

```
[SKIPPED] Skipping this test because Config.IncludeHttpRoutes is set to `false`.
```

The HTTP routing layer is stable; most development focuses on TCP routing. To enable:
```json
"include_http_routes": true
```

### App Logs Show `Error on connection read: EOF`

This is **normal** and happens during TCP route cleanup:

```
[APP/PROC/WEB/0] OUT Error on connection read: EOF
```

The test closes connections after verifying routing works. The app logs these as connection resets,
which is expected behavior for connection-based protocol testing.

### BuildPack Staging Fails: `Error staging application: StagingError`

The `go_buildpack` may be missing or not ready. Verify and bootstrap:
```bash
cf buildpacks | grep go_buildpack || make -C $KIND_DEPLOYMENT_DIR bootstrap
```

Then retry the tests.

### `Domain not found` on `cf create-route`

The TCP shared domain (`tcp_apps_domain`) does not exist in CF yet. The script creates it
automatically, but you can also do it manually:
```bash
cf create-shared-domain tcp.127-0-0-1.nip.io --router-group default-tcp
```

### `There are no more ports available for this domain`

The TCP router group has exhausted its port pool. See [TCP Router Port Configuration](#tcp-router-port-configuration) above.

### `dial tcp <ip>:<port>: i/o timeout`

The test could not reach the TCP router. Verify:
1. The `cfk8s-worker` container is running: `docker ps | grep cfk8s-worker`
2. The Istio Gateway and NodePort service include the port in use:
   ```bash
   kubectl get svc istio-gateway-istio -n default \
     -o jsonpath='{range .spec.ports[*]}{.port}{"\n"}{end}' | grep "^320"
   ```
3. The `addresses` field in `config.json` matches the actual Docker network IP of `cfk8s-worker`.

### Envoy crash / Exit status 134 on app start

The host's inotify limits are too low. See [Linux Kernel Parameters](#linux-kernel-parameters) above.