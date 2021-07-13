# Ops-files

This is the README for our Ops-files. To learn more about `routing-release`, go to the main [README](../README.md).

| Name | Purpose | Notes |
| --- | --- | --- |
| add-cipher-suites.yml | Sets the supported cipher suites for gorouter to ECDSA ciphers. | |
| add-lb-ca-cert.yml | Required for resolving route services hosted on the platform. | Gives gorouter the LB CA cert so it trusts requests coming from inside the platform. |
| add-routing-api-mtls-certificates.yml | Configures routing-api to listen with mTLS and provide certificates for clients. | |
| create-indicator-protocol-uaa-client.yml | Creates a uaa client for the indicator protocol tests | We use this to run the indicator protocol tests in the routing pipeline. This tests the indicator protocol file responsible for health watch metrics. |
| disable-bbr-non-routing.yml | Disables backups for components that aren't routing. |  We use this when running drats to only test our components, and cut down on time. |
| enable-nats-tls-for-cf.yml | Used for deploying routing components with NATS TLS. | Eventually will be moved to cf-deployment when NATS TLS work is complete |
| routing-acceptance-tests.yml | Adds the instance group and job required for routing-acceptance tests. | [Routing Acceptance Tests](https://github.com/cloudfoundry/routing-acceptance-tests) |
| routing-smoke-tests.yml | Adds the instance group and job required for routing smoke tests. | |
| scale-for-cats.yml | This is used for testing with a cf-deployment pooled env. They are provisioned pretty light, so we need to scale them up. Otherwise, CATS becomes very flakey. | Adds diego cells and increase their size. |
| smoke-tests.yml | Configures the CF smoke tests. Required for running CF smoke tests. | Enables isolation segment smoke tests. Required for running CF smoke tests. |
