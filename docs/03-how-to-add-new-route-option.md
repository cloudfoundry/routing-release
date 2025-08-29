---
title: How To Add a New Route Option
expires_at: never
tags: [routing-release]
---

# How To Add New Route Options

## What are Per-Route Features?

Before this feature was implemented, the Cloud Foundry routing stack did not support configuring features for specific routes. Operators could define most features only at the platform level. The generic per-route features allow defining specific options on a per-route basis.

Generic per-route features were introduced in [RFC-0027](https://github.com/cloudfoundry/community/blob/main/toc/rfc/rfc-0027-generic-per-route-features.md) and are tracked in the [community implementation issue](https://github.com/cloudfoundry/community/issues/909). The implementation became available in routing-release v0.329 and capi-release 1.202.0.

The first per-route feature implemented is the load balancing algorithm, which defines how the load is distributed between Gorouters and backends. The algorithm can be configured on the route level via the application manifest:

```console
applications:
- name: test
  routes:
  - route: example.com
    options:
      loadbalancing: round-robin
  - route: example2.com
    options:
      loadbalancing: least-connection
  - route: example3.com
    options:
      loadbalancing: hash
      hash_header: tenant-id
      hash_balance: 1.25
```

> [!NOTE]
> In the implementation, the `options` property of a route represents per-route features.

## Overview

The picture provides a simplified overview of the participating components. The components marked in green need to be enhanced to support additional per-route features.

![Overview of the participating components](images/components.png)

Implementing per-route features requires changes to multiple components across several working groups, namely Application Platform Runtime and Application Runtime Interface.

## Required Changes

* Identify your use case
* Create an RFC (or an amendment to the existing one) to communicate a new per-route feature
* Create an implementation issue similar to [tracking issue #909](https://github.com/cloudfoundry/community/issues/909)

### Use Cases

#### New Feature for Application Routes

To introduce a new application per-route option, follow the instructions:

* The **CF CLI** requires no changes, as it already accepts and forwards arbitrary key-value pairs for per-route options to the Cloud Controller.
* Extend **Cloud Controller** to accept and process a new per-route option in the manifest and the API. The [Pull-Request #4080](https://github.com/cloudfoundry/cloud_controller_ng/pull/4080/files) which introduced per-route options with the `loadbalancing` option can serve as a starting point. Update the documentation in the `docs` folder accordingly.
* **BBS** requires no code changes. The `route` object is stored as a generic JSON object in BBS, which simply accepts the `options` it receives from Cloud Controller and saves them as a string in its database. For more details, refer to the discussion on [BBS issue #939](https://github.com/cloudfoundry/diego-release/issues/939).
* **Route-emitter** and **routing-info** require no changes. The initial implementation extended the `CFRoute` with `options` as a raw JSON message. The only additional step to consider is implementing tests for these components to ensure the new option is included in the raw JSON.
* **NATS** requires no implementation, as it only forwards route registration messages.
* Implement your logic in **Gorouter** if it does not already exist. Extend the options included in the registration message within the [mbus/subscriber](https://github.com/cloudfoundry/gorouter/blob/b0d88bb6204cf28e476b4ee680a6f5a154885608/mbus/subscriber.go#L1). The structure `RegistryMessageOpts` represents the property `options` of the NATS registration message. This property can contain additional, custom configuration for a route.
* Please remember to update examples and extend the documentation related to your changes.

#### New Feature for BOSH Components

For BOSH system components like Concourse or monitoring tools, route information is transferred via a shorter path.

* Implement the same change as for the use case above in the **Gorouter**
* Enhance **route-register** [configuration](https://github.com/cloudfoundry/route-registrar/blob/96bc622f89bb0366723086d5a5bf89e3ddfe5a39/config/config.go#L1) with a new property within the `options` structure. Add your implementation to correctly map the Route Options in the [messagebus](https://github.com/cloudfoundry/route-registrar/blob/96bc622f89bb0366723086d5a5bf89e3ddfe5a39/messagebus/messagebus.go#L1). 
* Add a new option to the [specification](https://github.com/cloudfoundry/routing-release/blob/65902515fc73bb7662b3d071c68ddc28a69e391c/jobs/route_registrar/spec#L146) of the route-registrar job in the **routing-release**.
