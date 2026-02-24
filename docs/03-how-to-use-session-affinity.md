---
title: How To Use Session Affinity
expires_at: never
tags: [routing-release]
---

# How To Use Session Affinity

## What is it?
Session affinity, also known as sticky sessions, enables requests from a
particular client to consistently reach the same application instance when multiple
app instances are deployed. This allows applications to store data specific to a user
session in memory without requiring external session storage.

## Architecture
<img src="images/sticky_sessions.png" alt="Sticky Sessions Architecture Diagram" width="800">

Sticky sessions are initiated by applications by setting a sticky session cookie in the
response. The default sticky session cookie name is `JSESSIONID`. You can configure 
additional cookie names that the routing tier recognizes for sticky sessions by editing 
the `router.sticky_session_cookie_names` configuration key in the deployment manifest.

When Gorouter receives a response from an application with a `JSESSIONID` cookie (or other 
configured sticky session cookie), Gorouter sets a `__VCAP_ID__` cookie containing the 
instance GUID of the responding application. The `__VCAP_ID__` cookie inherits all relevant 
attributes from the `JSESSIONID` cookie, including `Max-Age`, `Expires`, `SameSite`, and 
`Partitioned`.

In subsequent requests, the client sends both the `__VCAP_ID__` and `JSESSIONID` cookies 
(web browsers do this automatically). Gorouter uses the `__VCAP_ID__` cookie to forward 
the request to the same application instance that originally set the session cookie.


## Try it out!
Using the example [Dora app](https://github.com/cloudfoundry/cf-acceptance-tests/tree/db3503add82d01163318d5d1c5f30603efb81055/assets/dora#sticky-sessions),
you can try sticky sessions for yourself!

## FAQ

### What happens when the app instance GUID in the `__VCAP_ID__` cookie is no longer valid?
If the application instance referenced by the `__VCAP_ID__` cookie no longer exists (e.g., due to 
scaling down or instance restart), Gorouter will route the request to another available instance 
of the same application.

**Important**: Gorouter will **not** automatically create a new `__VCAP_ID__` cookie unless the 
application instance sets a new `JSESSIONID` cookie in its response. This ensures that the 
application maintains control over session lifecycle. If the application does not set a new 
`JSESSIONID` cookie, subsequent requests will continue to be routed randomly across available 
instances until the application explicitly establishes a new session.

### What happens if only one of `JSESSIONID` or `__VCAP_ID__` cookies is set on a request?
Gorouter requires both cookies to be present for sticky session routing. If only one cookie is 
present, Gorouter will route the request to a random available application instance.

### What if an application sets its own `__VCAP_ID__` cookie?
If the application response already contains a `__VCAP_ID__` cookie, Gorouter will not add another 
one. The application's `__VCAP_ID__` cookie will be forwarded to the client as-is. This allows 
applications to have full control over the sticky session cookie if needed.

### Does Gorouter support sticky sessions with Negotiate authentication (Kerberos/SPNEGO)?
Yes. When the `router.sticky_sessions_for_auth_negotiate` BOSH property is enabled, Gorouter will 
automatically create sticky sessions for responses containing a `WWW-Authenticate: Negotiate` header. 
This ensures that Kerberos/SPNEGO authentication handshakes complete successfully by routing 
subsequent requests to the same application instance.

In this case, Gorouter sets the `__VCAP_ID__` cookie with predefined attributes (`Max-Age` and 
`SameSite=Strict`) since there is no `JSESSIONID` cookie to inherit attributes from.

### How is the `X-CF-App-Instance` header different from sticky sessions?
The `X-CF-App-Instance` header allows routing to a specific app instance _index_ (e.g., the 3rd 
instance). This is useful for debugging specific instances but is meant only for testing and 
debugging scenarios. Sticky sessions, on the other hand, are meant for production use and route 
based on instance GUID, allowing automatic failover when an instance becomes unavailable. See 
[these docs](https://docs.cloudfoundry.org/concepts/http-routing.html#app-instance-routing) for 
more information.

### Is `vcap_request_id` related to sticky sessions?
No. The `vcap_request_id` header is a random GUID set by Gorouter on every request for tracing 
and correlation purposes. Despite the naming similarity, it is not related to the `__VCAP_ID__` 
cookie used for sticky sessions.

### What happens when I restart my browser?
This depends on the cookie lifecycle attributes `Expires` and `Max-Age`. Gorouter copies all cookie 
attributes from the `JSESSIONID` cookie to the `__VCAP_ID__` cookie.

When the `JSESSIONID` cookie does not specify `Max-Age` or `Expires`, it is treated as a session 
cookie and will expire when the browser is closed, ending the session. The `__VCAP_ID__` cookie 
will expire at the same time.

When either attribute is specified, both cookies will expire based on the configured lifetime. 
Note that `Max-Age` takes precedence over `Expires` per the HTTP cookie specification.

### How can I use a route service (a platform-deployed reverse proxy) in front of an application that relies on sticky sessions?
Sticky sessions work correctly with both platform-deployed Cloud Foundry route services and 
externally deployed route services. The `__VCAP_ID__` cookie is properly forwarded through 
the route service chain.

### What cookie attributes does `__VCAP_ID__` inherit from `JSESSIONID`?
The `__VCAP_ID__` cookie inherits most attributes from the `JSESSIONID` cookie set by the application. 
[See implementation](https://github.com/cloudfoundry/routing-release/blob/e9683b43dec98ee211390c28cc8611d6869de801/src/code.cloudfoundry.org/gorouter/proxy/round_tripper/proxy_round_tripper.go#L508-L512).

#### Expires, SameSite, Max-Age, Partitioned 
These attributes are always copied directly from the `JSESSIONID` cookie to the `__VCAP_ID__` cookie 
without modification.

#### Secure 
The `Secure` attribute on the `__VCAP_ID__` cookie is controlled by two factors:
1. The `Secure` attribute on the `JSESSIONID` cookie set by the application
2. The value of the [`router.secure_cookies`](https://github.com/cloudfoundry/routing-release/blob/f03f47b1dfe43a90e0717afd0d111e017e8d0fe1/jobs/gorouter/spec#L67-L69) BOSH property

The behavior is as follows:

| `router.secure_cookies` | `JSESSIONID` is secure? | `__VCAP_ID__` is secure? |
|-------------------------|-------------------------|--------------------------|
| false                   | true                    | true                     |
| false                   | false                   | false                    |
| true                    | true                    | true                     |
| true                    | false                   | true                     |

When `router.secure_cookies` is set to `true`, the `__VCAP_ID__` cookie will always be secure, 
regardless of the application's `JSESSIONID` cookie settings.

