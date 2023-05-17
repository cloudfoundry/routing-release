# Issue
Cloud Foundry environments may experience many 503 errors with `x_cf_routererror:"no_endpoints"` even though all of the apps appear to up and functional without error.
There is an entry in the route table for the desired route, but there are no healthy endpoints available.

This is caused by Changes introduced in routing-release 0.262.0 to enable Gorouter to retry more types of idempotent requests to failed backends.

# How to detect if your app has experienced this bug

The following commands can be run on the gorouter log file to check for possible occurrences. What you need to look for is when data.error has a value of "context canceled" followed by a prune-failed-endpoint error.

```
# Here is an example from from a gorouter log bundle collected from BOSH.  We find a vcap id of a failed request that meets the criteria using the above command.
find . -name "gorouter.stdout.log*" | while read line; do grep backend-endpoint-failed $line | jq -r '. | select(.data.error | contains("context canceled")) | .data.vcap_request_id'; done | head -1

27116dd3-f047-4a35-7873-e9ef7e1d3f71

# Next we find the log line that has the application ID
find . -name "gorouter.stdout.log*" | xargs egrep  -Hn 27116dd3-f047-4a35-7873-e9ef7e1d3f71

./router.d60e75ac-5459-49f8-b029-543579d74ed0.2023-05-05-18-05-52/gorouter/gorouter.stdout.log:192:{"log_level":3,"timestamp":"2023-05-04T19:38:42.838473790Z","message":"backend-endpoint-failed","source":"vcap.gorouter","data":{"route-endpoint":{"ApplicationId":"d45e4b57-3420-40b3-b13d-9ef0562d58c5",REDACTED,"RouteServiceUrl":""},"error":"incomplete request (context canceled)","attempt":1,"vcap_request_id":"27116dd3-f047-4a35-7873-e9ef7e1d3f71","retriable":true,"num-endpoints":1,"got-connection":false,"wrote-headers":false,"conn-reused":false,"dns-lookup-time":0,"dial-time":0,"tls-handshake-time":0}}

# and verify the endpoint was pruned as a result of this fault
egrep -A5 -Hn 27116dd3-f047-4a35-7873-e9ef7e1d3f71 ./router.d60e75ac-5459-49f8-b029-543579d74ed0.2023-05-05-18-05-52/gorouter/gorouter.stdout.log | egrep "prune-failed-endpoint|d45e4b57-3420-40b3-b13d-9ef0562d58c5" | egrep prune-failed-endpoint

./router.d60e75ac-5459-49f8-b029-543579d74ed0.2023-05-05-18-05-52/gorouter/gorouter.stdout.log-193-{"log_level":3,"timestamp":"2023-05-04T19:38:42.838565797Z","message":"prune-failed-endpoint","source":"vcap.gorouter.registry","data":{"route-endpoint":{"ApplicationId":"d45e4b57-3420-40b3-b13d-9ef0562d58c5",REDACTED,"process_instance_id":"2ea1596c-a745-4fdc-53a4-d885","process_type":"web","source_id":"d45e4b57-3420-40b3-b13d-9ef0562d58c5",REDACTED,"RouteServiceUrl":""}}}
```

# Resolution

To resolve this issue, upgrade routing-release to v0.266.0 or above.
