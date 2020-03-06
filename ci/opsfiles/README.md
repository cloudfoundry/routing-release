# Ops-files

This is the README for our Ops-files. To learn more about `routing-release`, go to the main [README](../README.md).

| Name | Purpose | Notes |
| --- | --- | --- |
| add-cipher-suites.yml | Sets the supported cipher suites for gorouter to ECDSA ciphers. | We use this in CI with tls-pem.yml when setting ECDSA certs. |
| add-lb-ca-cert.yml | Gives gorouter the LB cert to trust when resolving a route service hosted on platform. | |
| add-routing-api-mtls-certificates.yml | Configures routing-api to listen with mTLS and provide certificates for clients. | |
| create-indicator-protocol-uaa-client.yml | | |
| disable-bbr-non-routing.yml | | |
| enable-locket-isolated-diego.yml | | |
| enable-nats-tls-for-cf.yml | | |
| frontend-idle-timeout.yml | | |
| isolation-in-z3.yml | | |
| routing-acceptance-tests.yml | | |
| routing-smoke-tests.yml | | |
| scale-for-cats.yml | | |
| smoke-tests.yml | | |
| syslog.yml | | |
| tls-pem.yml | | |
| update-watch.yml | | |
| use-latest-capi-release.yml | | |
| use-latest-cf-networking-release.yml | | |
| use-latest-routing-release.yml | | |
| use-latest-silk-release.yml | | |
| validate-ssl.yml | | |
| xfcc.yml | | |
