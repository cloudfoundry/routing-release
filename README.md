# Cloud Foundry Routing [BOSH release]

This repository is a [BOSH release](https://github.com/cloudfoundry/bosh) that
delivers HTTP and TCP routing for Cloud Foundry.

## Status
Job | Status
--- | ---
unit tests | [![networking.ci.cf-app.com](https://networking.ci.cf-app.com/api/v1/teams/ga/pipelines/routing/jobs/routing-release-unit/badge)](https://networking.ci.cf-app.com/teams/ga/pipelines/routing/jobs/routing-release-unit)
performance tests | [![networking.ci.cf-app.com](https://networking.ci.cf-app.com/api/v1/teams/ga/pipelines/routing/jobs/diana-tcp-perf-tests/badge)](https://networking.ci.cf-app.com/teams/ga/pipelines/routing/jobs/diana-tcp-perf-tests)
smoke tests | [![networking.ci.cf-app.com](https://networking.ci.cf-app.com/api/v1/teams/ga/pipelines/routing/jobs/cf-deployment-smoke-and-indicator-protocol-tests/badge)](https://networking.ci.cf-app.com/teams/ga/pipelines/routing/jobs/cf-deployment-smoke-and-indicator-protocol-tests)

## Getting Help

If you a concrete issue to report or change to request, please create a Github issue.  Issues with any related submodules ([Gorouter](https://github.com/cloudfoundry/gorouter), [Routing API](https://github.com/cloudfoundry/routing-api), [Route Registrar](https://github.com/cloudfoundry/route-registrar), [CF TCP Router](https://github.com/cloudfoundry/cf-tcp-router)) should be created here instead.

You can also reach us on Slack at [cloudfoundry.slack.com](https://cloudfoundry.slack.com) in the `#networking`
channel.

## Developer Workflow

When working on individual components of the Routing Release, work out of the
submodules under `src/`.

Run the appropriate unit tests (see
[Testing](#running-unit-and-integration-tests)).

The `release` branch contains code that has been released. All development work
happens on the `develop` branch.

### Get the code

1. Clone the repository

  ```bash
  mkdir -p ~/workspace
  cd ~/workspace
  git clone https://github.com/cloudfoundry/routing-release.git
  cd routing-release/
  ```

1. Automate `$GOPATH` and `$PATH` setup.

  This BOSH release doubles as a `$GOPATH`. It will automatically be set up for
  you if you have [direnv](http://direnv.net) installed.

  ```bash
  direnv allow
  ```

  If you do not wish to use `direnv`, you can simply `source` the `.envrc` file
  at the root of the repository.  You may manually need to update your `$GOPATH`
  and `$PATH` variables as you switch in and out of the directory.

1. Initialize and sync submodules.

  ```bash
  ./scripts/update
  ```

### Running BOSH Job Templating Tests
From the root of the repo, run:

#### Run the specs
```bash
rspec ./spec/
```

#### Lint the specs
```bash
rubocop ./spec/
```

If you do not have `rspec` or `rubocop` installed locally, run
`./scripts/start-docker-for-testing.sh` and execute the commands in the docker
container.


### Running Unit and Integration Tests

#### In a Docker container

* Run tests using the script provided. This script pulls a docker image and runs
  the tests within a container because integration tests require Linux specific
  features.

  Notice/warning: the script is called `run-unit-tests-in-docker` but it really
  runs unit *and* integration tests, that's why they need to run in a container.

  ```bash
  ./scripts/run-unit-tests-in-docker
  ```

* If you'd like to run a specific component's tests in a Docker container,
  the `run-unit-tests` script also takes a package name as an argument:

  ```bash
  ./scripts/run-unit-tests-in-docker gorouter
  ```

#### Locally

* If you'd like to run the unit and integration tests for an individual
  component locally, we recommend you run `bin/test` in that component's
  directory. Please make sure it's a component that doesn't require a Linux
  operating system.

## Running Acceptance tests

The Routing Acceptance Tests must run on a full Cloud Foundry deployment. One
method is to [deploy Cloud
Foundry](https://github.com/cloudfoundry/cf-deployment/tree/master/iaas-support/bosh-lite)
on a BOSH lite with cf-deployment.

To Run the [Routing Acceptance
Tests](https://github.com/cloudfoundry/routing-acceptance-tests), see the
README.md.

## High Availability

The TCP Router and Routing API are stateless and horizontally scalable. The TCP
Routers must be fronted by a load balancer for high-availability. The Routing
API depends on a database, that can be clustered for high-availability. For high
availability, deploy multiple instances of each job, distributed across regions
of your infrastructure.

## Routing API
For details refer to [Routing API](https://github.com/cloudfoundry/routing-api/blob/master/README.md).

## Metrics Documentation
For documentation on metrics available for streaming from Routing components
through the Loggregator
[Firehose](https://docs.cloudfoundry.org/loggregator/architecture.html), visit
the [CloudFoundry
Documentation](http://docs.cloudfoundry.org/loggregator/all_metrics.html#routing).
You can use the [NOAA Firehose sample app](https://github.com/cloudfoundry/noaa)
to quickly consume metrics from the Firehose.
