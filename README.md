# Routing Release

This repository is a [BOSH release](https://github.com/cloudfoundry/bosh) for
deploying Gorouter, TCP Routing, and other associated tasks that provide HTTP and TCP routing in Cloud Foundry foundations.

## Downloads

Our BOSH release is available on [bosh.io](http://bosh.io/releases/github.com/cloudfoundry/routing-release)
and on our [GitHub Releases page](https://github.com/cloudfoundry/routing-release/releases).

## Getting Help

If you have a concrete issue to report or a change to request, please create a
[Github issue on
routing-release](https://github.com/cloudfoundry/routing-release/issues/new/choose).

Issues with any related submodules
([Gorouter](https://github.com/cloudfoundry/gorouter), [Routing
API](https://github.com/cloudfoundry/routing-api), [Route
Registrar](https://github.com/cloudfoundry/route-registrar), [CF TCP
Router](https://github.com/cloudfoundry/cf-tcp-router)) should be created here
instead.

You can also reach us on Slack at
[cloudfoundry.slack.com](https://cloudfoundry.slack.com) in the
[`#cf-for-vms-networking`](https://cloudfoundry.slack.com/app_redirect?channel=C01ABMVNE9E).
channel.

## Contributing
See the [Routing Contributing Resources](#routing-contributor-resources) section for more information on how to contribute.

## Table of Contents
1. [Routing Operator Resources](#routing-operator-resources)
1. [Routing App Developer Resources](#routing-app-developer-resources)
1. [Routing Contributor Resources](#routing-contributor-resources)

---
## <a name="routing-operator-resources"></a> Routing Operator Resources
### <a name="high-availability"></a> High Availability

The TCP Router and Routing API are stateless and horizontally scalable. The TCP
Routers must be fronted by a load balancer for high-availability. The Routing
API depends on a database, that can be clustered for high-availability. For high
availability, deploy multiple instances of each job, distributed across regions
of your infrastructure.

### <a name="routing-api"></a> Routing API
For details refer to [Routing API](https://github.com/cloudfoundry/routing-api/blob/master/README.md).

### <a name="metrics"></a> Metrics
For documentation on metrics available for streaming from Routing components
through the Loggregator
[Firehose](https://docs.cloudfoundry.org/loggregator/architecture.html), visit
the [CloudFoundry
Documentation](http://docs.cloudfoundry.org/loggregator/all_metrics.html#routing).
You can use the [NOAA Firehose sample app](https://github.com/cloudfoundry/noaa)
to quickly consume metrics from the Firehose.
## <a name="routing-app-developer-resources"></a> Routing App Developer Resources

### <a name="session-affinity"></a> Session Affinity
For more information on how Routing release accomplishes session affinity, i.e.
sticky sessions, refer to the [Session Affinity document](docs/session-affinity.md).

### <a name="headers"></a> Headers
[X-CF Headers](/docs/x_cf_headers.md) describes the X-CF headers that are set on requests and responses inside of CF.

## <a name="routing-contributor-resources"></a> Routing Contributor Resources

### <a name="developer-workflow"></a> Developer Workflow

- Clone [CI repository](https://github.com/cloudfoundry/wg-app-platform-runtime-ci) (next to where this code is cloned), and make sure latest
is pulled by running `git pull`

  ```bash
  mkdir -p ~/workspace
  cd ~/workspace
  git clone https://github.com/cloudfoundry/wg-app-platform-runtime-ci.git
  ```
- [Git](https://git-scm.com/) - Distributed version control system
- [Go](https://golang.org/doc/install#install) - The Go programming
  language

Run the appropriate unit tests (see
[Testing](#running-unit-and-integration-tests)).

The `release` branch contains code that has been released. All development work
happens on the `develop` branch.

#### Get the code

1. Clone the repository

  ```bash
  mkdir -p ~/workspace
  cd ~/workspace
  git clone https://github.com/cloudfoundry/routing-release.git
  cd routing-release/
  ```

1. Initialize and sync submodules.

  ```bash
  ./scripts/update
  ```

#### <a name="running-bosh-job-templating-tests"></a> Running BOSH Job Templating Tests

Template tests can be run with `./scripts/test-in-docker-locally.bash`. This will also run unit + integration tests.

If you wish to run template tests on their own,  you can enter the docker env with `./scripts/start-docker-for-testing.bash` and then run the tests via `cd /repo && ./scripts/docker/tests-templates.bash` 

#### <a name="running-unit-and-integration-tests"></a> Running Unit and Integration Tests

##### With Docker

Running tests for this release requires a `DB` flavor. The following scripts with default to `mysql` DB. Set `DB` environment variable for alternate DBs e.g. <mysql-8.0(or mysql),mysql-5.7,postgres>

- `./scripts/create-docker-container.bash`: This will create a docker container with appropriate mounts.
- `./scripts/test-in-docker-locally.bash`: Create docker container and run all tests and setup in a single script.
  - `./scripts/test-in-docker-locally.bash <package> <sub-package>`: For running tests under a specific package and/or sub-package: e.g. `./scripts/test-in-docker-locally.bash gorouter router`

When inside docker container: 
- `/repo/scripts/docker/build-binaries.bash`: This will build binaries required for running tests e.g. nats-server and rtr
- `/repo/scripts/docker/test.bash`: This will run all tests in this release
- `/repo/scripts/docker/test.bash gorouter`: This will only run `gorouter` tests
- `/repo/scripts/docker/test.bash gorouter router`: This will only run `router` sub-package tests for `gorouter` package
- `/repo/scripts/docker/tests-templates.bash`: This will run all of tests for bosh tempalates
- `/repo/scripts/docker/lint.bash`: This will run all of linting defined for this repo.

#### <a name="running-acceptance-tests"></a> Running Acceptance tests

The Routing Acceptance Tests must run against a full Cloud Foundry deployment. One
method is to [deploy Cloud
Foundry](https://github.com/cloudfoundry/cf-deployment/tree/master/iaas-support/bosh-lite)
on a BOSH lite with cf-deployment.

To run the [Routing Acceptance
Tests](https://github.com/cloudfoundry/routing-acceptance-tests), see the
README.md.
