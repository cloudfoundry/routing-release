# Migrating from tcp_router (HAProxy) to envoy-tcp-router (Envoy)

The `envoy-tcp-router` job replaces the existing `tcp_router` (HAProxy-based) job for
TCP routing in Cloud Foundry. Both jobs consume TCP route mappings from the CF Routing
API; `envoy-tcp-router` uses Envoy as the data plane and an xDS control plane written
in Go instead of HAProxy.

## Why migrate?

| Capability | tcp_router (HAProxy) | envoy-tcp-router (Envoy) |
|---|---|---|
| Data plane | HAProxy | Envoy |
| Dynamic config | Polling + config file rewrite | xDS (gRPC streaming) |
| SNI-based multiplexing | Limited | Native (`FilterChainMatch`) |
| L4 Go filter extensibility | No | Yes — `filter.so` |
| mTLS identity (RFC 0055) | No | Phase 2 (planned) |

## Migration steps

### 1. Add the Envoy binary blob

Download the static Envoy binary for Linux amd64 and add it to the BOSH blobstore:

```bash
bosh add-blob <path>/envoy-1.33.0-linux-x86_64.tar.gz envoy/envoy-1.33.0.tar.gz
```

### 2. Update your BOSH deployment manifest

Add the `envoy-tcp-router` job alongside (or instead of) `tcp_router`:

```yaml
instance_groups:
  - name: tcp-router
    jobs:
      # Disable the HAProxy-based router
      - name: tcp_router
        release: routing
        properties:
          tcp_router:
            disable: true          # stops HAProxy from starting

      # Enable the Envoy-based router
      - name: envoy-tcp-router
        release: routing
        properties:
          envoy_tcp_router:
            xds_port: 18000
            refresh_interval: 30s
          routing_api:
            uri: https://routing-api.service.cf.internal
            port: 3000
            ca_cert: ((routing_api_tls.ca))
            client_cert: ((routing_api_client_tls.certificate))
            client_private_key: ((routing_api_client_tls.private_key))
          uaa:
            token_endpoint: uaa.service.cf.internal
            tls_port: 8443
            ca_cert: ((uaa_ssl.ca))
```

### 3. Verify traffic

After deploying, confirm Envoy is receiving xDS updates and routing traffic:

```bash
# Check the control plane logs
bosh -d <deployment> ssh tcp-router -c 'sudo tail -f /var/vcap/sys/log/envoy-tcp-router/envoy-tcp-router-control-plane.log'

# Check Envoy logs
bosh -d <deployment> ssh tcp-router -c 'sudo tail -f /var/vcap/sys/log/envoy-tcp-router/envoy.log'
```

### 4. Remove tcp_router from the manifest (optional)

Once confident, remove the `tcp_router` job entry and the `haproxy` and `tcp_router`
package references from your deployment manifest. The packages will remain in the
release for backwards compatibility until a future major version removes them.

## Rollback

Set `tcp_router.disable: false` on the `tcp_router` job and redeploy. HAProxy will
start again immediately; `envoy-tcp-router` can be left in place or removed.
