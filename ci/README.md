## Routing Release CI
This folder contains all release specific pipeline materials for running our CI.
You can find the CI [here](https://networking.ci.cf-app.com/teams/ga/pipelines/routing)

### Summary

#### manifests/
* Special manifests for altered deployments used in CI. For example, in perf
  testing, we make special deployments that contain a standalone GoRouter. The
  manifests for these deployments live here.

#### opsfiles/
* Opsfiles specific to routing-release that are used for deploying CI
  environments. Opsfiles like `routing-smoke-tests.yml` live here.

#### pipelines/
* All routing pipeline yaml files.

#### scripts/
* Helper scripts related to CI. Right now, we have some scripts to help create a
  local bosh lite deployment.

  For local bosh-lite management:
  ```bash
  export PATH=$PATH:$(pwd)/scripts
  local_bosh_lite_create
  ```

#### tasks/
* Routing-release specific tasks used in the routing pipeline. Tasks like
  `create-integration-configs` live here.
