---
title: How To enable Quotas for TCP Routing
expires_at: never
tags: [routing-release]
---

# How To Enable Quotas for TCP Routing

As ports can be a limited resource in some environments, the default quotas in
Cloud Foundry for IaaS other than BOSH Lite do not allow reservation of route
ports. Route ports are required for the creation of TCP routes. The final step
to enabling TCP routing is to modify quotas to set the maximum number of TCP
routes that may be created by each organization or space.

Determine whether your default org quota allows TCP Routes:

```
$ cf quota default
Getting quota default info as admin...
OK

Total Memory           10G
Instance Memory        unlimited
Routes                 -1
Services               100
Paid service plans     allowed
App instance limit     unlimited
Reserved Route Ports   0
```

If `Reserved Route Ports` is greater than zero, you can skip the following step,
as this attribute determines how many TCP routes can be created within each
organization assigned to this quota.

If `Reserved Route Ports` is zero, you can update the quota with the following
command:

```
$ cf update-quota default --reserved-route-ports 2
Updating quota default as admin...
OK

$ cf quota default
Getting quota default info as admin...
OK

Total Memory           10G
Instance Memory        unlimited
Routes                 -1
Services               100
Paid service plans     allowed
App instance limit     unlimited
Reserved Route Ports   2
```

Configuring `Reserved Route Ports` to `-1` sets the quota attribute to
unlimited. For more information on configuring quotas for TCP Routing, see
[Enabling TCP
Routing](https://docs.cloudfoundry.org/adminguide/enabling-tcp-routing.html#configure-quota).
