# Modification Tags

A modification tag represents a single version of a route object. Whenever any
state change occurs on that object, the modification tag will be updated to
reflect the new state. This tag is necessary in order to enable clients of the
Routing API, e.g. the TCP router and the GoRouter, to make decisions about
whether a state change should be applied to their routing table.

### Maintaining Routing Tables

The essential purpose of the route endpoints on the Routing API is to enable
clients to construct an up-to-date routing table of all known backends. This is
achieved through two endpoints, a `GET` endpoint that will list all current
route mappings and a [SSE](https://www.w3.org/TR/eventsource/)
endpoint that allows for push notification capability.

A router will, on startup, initiate a `SSE` connection to the Routing API,
receiving and applying any events coming through the connection. Periodically,
the router will also resync with the Routing API through the `GET` endpoint,
essentially taking a snapshot of the API's state at that point in time.

### Ensuring Consistency

A Cloud Foundry router has only an eventually consistent view of the routing
information of the system, but all routers must attempt to provide the most
update-to-date information available to them. To help acheive this goal,
modification tags were introduced into all route objects in order to allow
routers to establish a temporal ordering of any state changes.

#### Modification Tag

```json
{
  "route": "api.bosh-lite.com/routing",
  ...
  "modification_tag": {
    "guid": "cbdhb4e3-141d-4259-b0ac-99140e8998l0",
    "index": 10
  }
}
```

##### Guid

Represents a single route object, stays the same for the entire lifetime of that
object.

The `guid` is used to solve the [ABA
problem](https://en.wikipedia.org/wiki/ABA_problem) that occurs when a route is
deleted and then quickly created again.

##### Index

Monotonically incremented on every state change to the route object, used to
determine the latest version of a given `guid`.

#### Example Usage

A modification tag "succeeds" another if their `guid`s are not equal or the
older modification tag's index is less than the newer tag's index. A different
`guid` succeeds another in all cases because a new `guid` indicates that the
object associated with the older `guid` has been deleted and another route
object has been created with the same key.

##### Applying `Upsert` events

An `UPSERT` event should be applied **only if** the modification tag of the
route in the routing table is "succeeded by" the new event waiting to be
applied.

Given a routing table such as:

| Object   | Guid   | Index |
|:--------:|:------:|:-----:|
| Route1   | aaaa   | 1     |
| Route2   | zzzz   | 10    |

And events received on the push notification are:

```
UPSERT Route1' guid: aaaa index: 0
UPSERT Route2' guid: yyyy index: 0
```

A router **should not** apply Route1' to the routing table, since both the event
and object in the routing table have the same `guid`, 'aaaa', but the route in
the routing table has an index greater than the index associated with the event.
The router **should** apply Route2' to the routing table, since the two
associated `guid`s are not equal, 'zzzz' versus 'yyyy'.

##### Applying `Delete` events

A `DELETE` event should be applied if the modification tag of the route in the
routing table is "succeeded by" the new event waiting to be applied **or** the
two modification tags are equal to each other.

Given a routing table such as:

| Object   | Guid   | Index |
|:--------:|:------:|:-----:|
| Route1   | aaaa   | 1     |
| Route2   | zzzz   | 10    |
| Route3   | gggg   | 14    |

And events received on the push notification are:

```
DELETE Route1 guid: aaaa index: 1
DELETE Route2 guid: zzzz index: 0
DELETE Route3 guid: hhhh index: 6
```

A router **should** delete Route1 since the two associated modification tags are
equal to each other. The router **should not** delete Route2, since the event
has the same `guid` but a smaller index. The router **should** delete Route3,
because the event's modification tag has a different `guid`, and thus succeeds
the modification tag in the routing table.
