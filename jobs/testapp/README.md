The testapp is provides three endpoints for testing route registrar.

1. HTTP endpoint on port 8081
2. HTTP endpoint on port 8082
3. TCP endpoint on port 8083


It is deployed via this manifest: https://github.com/cloudfoundry/wg-app-platform-runtime-ci/blob/779fc7850daa8982921e7418c47d4ba97b2c3139/routing-release/manifests/old-route-registrar.yml


This is built off of routing release 0.301.0 to test backwards compatibility
issues with route registrar.
