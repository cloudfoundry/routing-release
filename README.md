# Routing Release

This repository is a [BOSH release](https://github.com/cloudfoundry/bosh) for
deploying Gorouter, TCP Routing, and other associated tasks that provide HTTP and TCP routing in Cloud Foundry foundations.

## Downloads

Our BOSH release is available on [bosh.io](http://bosh.io/releases/github.com/cloudfoundry/routing-release)
and on our [GitHub Releases page](https://github.com/cloudfoundry/routing-release/releases).

## Getting Help

If you have a concrete issue to report or a change to request, please create a [Github issue on routing-release](https://github.com/cloudfoundry/routing-release/issues/new/choose).

Issues with any related submodules ([Gorouter](https://github.com/cloudfoundry/gorouter), [Routing API](https://github.com/cloudfoundry/routing-api), [Route Registrar](https://github.com/cloudfoundry/route-registrar), [CF TCP Router](https://github.com/cloudfoundry/cf-tcp-router)) should be created here instead.

You can also reach us on Slack at [cloudfoundry.slack.com](https://cloudfoundry.slack.com) in the [`#cf-for-vms-networking`](https://cloudfoundry.slack.com/app_redirect?channel=C01ABMVNE9E).
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
### <a name="ci-statues"></a> CI Statuses
Job | Status
--- | ---
unit tests | [![networking.ci.cf-app.com](https://networking.ci.cf-app.com/api/v1/teams/ga/pipelines/routing/jobs/routing-release-unit/badge)](https://networking.ci.cf-app.com/teams/ga/pipelines/routing/jobs/routing-release-unit)
performance tests | [![networking.ci.cf-app.com](https://networking.ci.cf-app.com/api/v1/teams/ga/pipelines/routing/jobs/diana-tcp-perf-tests/badge)](https://networking.ci.cf-app.com/teams/ga/pipelines/routing/jobs/diana-tcp-perf-tests)
smoke tests | [![networking.ci.cf-app.com](https://networking.ci.cf-app.com/api/v1/teams/ga/pipelines/routing/jobs/cf-deployment-smoke-and-indicator-protocol-tests/badge)](https://networking.ci.cf-app.com/teams/ga/pipelines/routing/jobs/cf-deployment-smoke-and-indicator-protocol-tests)

### <a name="developer-workflow"></a> Developer Workflow

When working on individual components of the Routing Release, work out of the
submodules under `src/`.

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
From the root of the repo, run:

##### Run the specs
```bash
rspec ./spec/
```

##### Lint the specs
```bash
rubocop ./spec/
```

If you do not have `rspec` or `rubocop` installed locally, run
`./scripts/start-docker-for-testing.sh` and execute the commands in the docker
container. Prepend "sudo" to the script if you are an unprivileged user.



#### <a name="running-unit-and-integration-tests"></a> Running Unit and Integration Tests

##### In a Docker container

* Run tests using the script provided. This script pulls a docker image and runs
  the tests within a container because integration tests require Linux specific
  features.

  Notice/warning: the script is called `run-unit-tests-in-docker` but it really
  runs unit *and* integration tests, that's why they need to run in a container.

  ```bash
  ./scripts/run-unit-tests-in-docker #sudo for unprivileged users
  ```

* If you'd like to run a specific component's tests in a Docker container,
  the `run-unit-tests` script also takes a package name as an argument:

  ```bash
  ./scripts/run-unit-tests-in-docker gorouter
  ```

##### Locally

* If you'd like to run the unit and integration tests for an individual
  component locally, we recommend you run `bin/test` in that component's
  directory. Please make sure it's a component that doesn't require a Linux
  operating system.

#### <a name="running-acceptance-tests"></a> Running Acceptance tests

The Routing Acceptance Tests must run against a full Cloud Foundry deployment. One
method is to [deploy Cloud
Foundry](https://github.com/cloudfoundry/cf-deployment/tree/master/iaas-support/bosh-lite)
on a BOSH lite with cf-deployment.

To run the [Routing Acceptance
Tests](https://github.com/cloudfoundry/routing-acceptance-tests), see the
README.md.
