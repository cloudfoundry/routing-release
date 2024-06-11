---
title: How To Use Session Affinity
expires_at: never
tags: [routing-release]
---

# How To Use Session Affinity

## What is it?

Session affinity, also known as sticky sessions, enables requests from a
particular client to always reach the same application instance when multiple
app instances are deployed. This allows apps to store data specific to a user
session.

## Architecture

```
+-----------+                                   +-----------+                                    +-----------+
|           |                                   |           |                                    |           |
|           | 1. Sends request                  |           | 2. Gorouter forwards to app/1      |           |
|           | +-------------------------------> |           | +--------------------------------> |           |
|           |                                   |           |                                    |           |
|           |                                   |           |                                    |           |
|           | 4. Gorouter adds __VCAP_ID__      |           | 3. App adds JSESSIONID cookie      |           |
| End User  |    cookie to response             | Gorouter  | to response                        |   app/1   |
|           | <-------------------------------+ |           | <--------------------------------+ |           |
|           |                                   |           |                                    |           |
|           |                                   |           |                                    |           |
|           | 5. Sends subsequent request       |           | 6. Gorouter routes to the same app |           |
|           |    with __VCAP_ID__ cookie	|           | instance (app/1)                   |           |
|           | +-------------------------------> |           | +--------------------------------> |           |
+-----------+                                   +-----------+                                    +-----------+

```


In the simplest case, an end user can start a sticky session by setting the
`__VCAP_ID__` cookie to desired app instance guid. Gorouter will inspect the
cookie on the request and forward the request to the correct app instance.
However, it is unlikely that the end user will know the desired app instance
guid that they want to send traffic to.

In the most common case, an app initiates a sticky session. In order for an app
to start a sticky session, the app must return a sticky session cookie in its
response (Step 3 in the diagram). The default sticky session cookie name is
`JSESSIONID`. You can configure the cookie names that the routing tier uses for
sticky sessions by editing the `router.sticky_session_cookie_names` config key
in the deployment manifest.

When Gorouter receives a response from an app with `JSESSIONID` set, then
Gorouter will set the `__VCAP_ID__` cookie to the instance guid of the
responding app with the same expiry as that of `JSESSIONID` (Step 4 in the diagram).
In subsequent requests, the end user should include the `__VCAP_ID__` cookie
(Step 5 in the diagram), which is done automatically in web browsers.
When an end user sens a request to Gorouter with the `__VCAP_ID__` cookie,
Gorouter will forward the request to the same application instance that
originally responded (Step 6 in the diagram).


## Try it out!

Using the example [Dora app](https://github.com/cloudfoundry/cf-acceptance-tests/tree/db3503add82d01163318d5d1c5f30603efb81055/assets/dora#sticky-sessions),
you can try sticky sessions for yourself!!!!

## FAQ

### What happens when the app instance guid set in the `__VCAP_ID__` cookie is not valid?

The request will not fail. Gorouter will forward the request to another app instance for
that route.

### What happens if `JSESSIONID` is set on a request from an end user, but `__VCAP_ID__` is not?

The Gorouter will forward request to a random app instance. It does not route
based on the value of the `JESSIONID` cookie.

### How is `X-CF-App-Instance` header different from sticky sessions?

Using the `X-CF-App-Instance` header a user can route to a specific app _index_.
For example, an app developer might be trying to debug the 3rd instance of their
app, which is mysteriously failing. With this header, they can send traffic only
to the 3rd instance. See [these
docs](https://docs.cloudfoundry.org/concepts/http-routing.html#app-instance-routing)
for more information on how to use this header.

### Is `vcap_request_id` related to sticky sessions?

Nope. The `vcap_request_id` header is a random guid that is set by Gorouter on every
request through the system. The header is used for tracing and correlating specific
requests. It is not related to the `__VCAP_ID__` cookie, despite the naming
similarity.

### What happens when I restart my browser?

Restarting your browser will likely clear the `__VCAP_ID__` cookie, effectively
ending the sticky session. Restarting the browser might or might not clear the
`JSESSIONID` cookie.  Since the `__VCAP_ID__` cookie is the only header inspected
for forwarding sticky session requests, only sending the `JSESSIONID` will not
result in sticky session behavior and may actually result in undesired behavior.

### How can I use a route service (a platform-deployed reverse proxy) in front of an application that relies on sticky sessions?
Routing Release 0.211.0+: Sticky sessions will now work with platform deployed route services.
Sticky sessions will continue to work for non-CF deployed route services.

Routing Release <0.211.0: If you deploy a reverse proxy to the platform in front of your app, the backend
app must return the `JSESSIONID` on every response in order to sticky sessions to
work.

### How can I tell if I got routed to a different instance than I was expecting?

If the app instance requested in the `__VCAP_ID__` does not exist, then the
Gorouter will route to another instance of the app. If you want to determine
when this happens, the app can compare the `__VCAP_ID__` in the response to the
one in the request.


### What about attributes?
The `__VCAP_ID__` cookie will take _some_ of the attributes from the `JSESSIONID` cookie that is set by the app. [See code here](https://github.com/cloudfoundry/gorouter/blob/379860daa83a162ffe0b6039eafb7c8bfa1eaccf/proxy/round_tripper/proxy_round_tripper.go#L312-L355).
#### Expiry 
The `__VCAP_ID__` cookie will always have the same expiry attribute as the `JSESSIONID` cookie that is set by the app.
#### Samesite 
The `__VCAP_ID__` cookie will always have the same samesite attribute as the `JSESSIONID` cookie that is set by the app.
#### Secure 
The secure attribute on the `__VCAP_ID__` cookie is controlled by two levers: what is set on the `JSESSIONID` cookie by the app and the value of the [router.secure_cookies](https://github.com/cloudfoundry/routing-release/blob/f03f47b1dfe43a90e0717afd0d111e017e8d0fe1/jobs/gorouter/spec#L67-L69) bosh property.
| router.secure_cookies | `JSESSIONID` is secure? | `__VCAP_ID__` is secure? |
|-----------------------|-----------------------|------------------------|
| false | true | true |
| false | false | false |
| true | true | true|
| true | false | true |

#### Max Age 
If the max age attribute on the `JSESSIONID` cookie that is set by the app is less than 0, then the same value will be set on the `__VCAP_ID__`.
