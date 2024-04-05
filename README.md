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
See the [Contributing.md](./.github/CONTRIBUTING.md) for more information on how to contribute.

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

