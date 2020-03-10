# Ops-files

This is the README for our Ops-files. To learn more about `routing-release`, go to the main [README](../README.md).

| Name | Purpose | Notes |
| --- | --- | --- |
| add-cipher-suites.yml | Sets the supported cipher suites for gorouter to ECDSA ciphers. | |
| add-lb-ca-cert.yml | Gives gorouter the LB cert to trust when resolving a route service hosted on platform. | |
| add-routing-api-mtls-certificates.yml | Configures routing-api to listen with mTLS and provide certificates for clients. | |
| create-indicator-protocol-uaa-client.yml | Creates a uaa client for the indicator protocol tests | We use this to run the indicator protocol tests in the routing pipeline. This tests the indicator protocol file responsible for health watch metrics. |
| disable-bbr-non-routing.yml | Disables backups for components that aren't routing. |  We use this when running drats to only test our components, and cut down on time. |
| enable-nats-tls-for-cf.yml | Used for deploying routing components with NATS TLS | Eventually will be moved to cf-deployment when NATS TLS work is complete |
| routing-acceptance-tests.yml | Adds instance group and job for routing-acceptance tests. | |
| routing-smoke-tests.yml | Adds instance group and job for routing smoke tests | |
| scale-for-cats.yml | Add diego cells and increase their size. | This is used for testing with a cf-deployment pooled env. They are provisioned pretty light, so we need to scale them up. Otherwise, CATS becomes very flakey. |
| smoke-tests.yml | Configures the CF smoke tests. | Enables isolation segment smoke tests. |
| syslog.yml | Configures deployment so the syslogs are forwarded via syslog-forwarded to the syslog-storer. | |
