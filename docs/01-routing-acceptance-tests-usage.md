---
title: Usage
expires_at: never
tags: [routing-release,routing-acceptance-tests]
---

# Usage

## Running Acceptance tests

In order to run tests for this repository. You need to generate a config.json

```json
{
  "addresses": [
    "${CF_TCP_DOMAIN}"
  ],
  "api": "api.${CF_SYSTEM_DOMAIN}",
  "admin_user": "admin",
  "admin_password": "${CF_ADMIN_PASSWORD}",
  "skip_ssl_validation": true,
  "use_http": true,
  "apps_domain": "${CF_SYSTEM_DOMAIN}",
  "include_http_routes": true,
  "default_timeout": 120,
  "cf_push_timeout": 120,
  "tcp_router_group": "default-tcp",
  "tcp_apps_domain": "${CF_TCP_DOMAIN}",
  "oauth": {
    "token_endpoint": "https://uaa.${CF_SYSTEM_DOMAIN}",
    "client_name": "routing_api_client",
    "client_secret": "$(bosh_get_password_from_credhub routing_api_client)",
    "port": 443,
    "skip_ssl_validation": true
  }
}
```

## Description of Config Fields
- `addresses` - contains the IP addresses of the TCP Routers and/or the Load Balancer's IP address. IP `10.24.14.2` is IP address of `tcp_router_z1/0` job in routing-release. If this IP address happens to be different in your deployment then change the entry accordingly. The `addresses` property also accepts DNS entry for tcp router, e.g. `tcp.bosh-lite.com`.
- `admin_user` and `admin_password` - refers to the admin user used to perform a CF login with the cf CLI.
- `skip_ssl_validation` - used for the cf CLI when targeting an environment.
- `include_http_routes` (optional) - a boolean used to run tests for the experimental HTTP routing endpoints of the Routing API.
- `verbose` (optional) - a boolean which allows for the `-v` flag to be passed when running the router acceptance tests errand
- `test_password` (optional) -  By default, users created during the routing acceptance tests are configured with a random name and password. If manually configured, this property enables specifying the password for the user created during the test. `test_password` performs the same function as the manifest property, `user_password`.
- `tcp_router_group` - The router group to use for creating tcp routes.
-  If `tcp_apps_domain` property is empty, smoke tests create a temporary shared domain and use the `addresses` field to connect to TCP application.
- `tcp_router_group` - The router group to use for creating tcp routes.

