# Known Issue: TCP Router Fails when Port Conflicts with Local Process

## 🔥 Affected Versions

* All versions of routing-release 

## ✔️ Operator Checklist
* [ ] Read this doc.
* [ ] Compare the listening ports on your TCP Router VM to the list below. See how [here](#how-to-check).
* [ ] Update your manifest to make `routing_api.reserved_system_component_ports` match the ports you learned about from step 2. See bosh properties details [here](#new-bosh-properties).
* [ ] Upgrade to a version of routing-release with these fixes.
* [ ] Look at the TCP Router logs to see if any exisiting router groups are invalid. See logs to look for [here](#fix).
* [ ] Fix invalid router groups. See routing-api documentation [here](https://github.com/cloudfoundry/routing-api/blob/main/docs/api_docs.md#update-router-group).
* [ ] Re-run the check to make sure all router groups are valid. See how [here](#how-to-rerun).

## 📑 Context

Each TCP route requires one port on the TCP Router VM. Ports for TCP routes are managed via [router groups](https://github.com/cloudfoundry/routing-api/blob/main/docs/api_docs.md#create-router-groups). Each router group has a list of `reservable_ports`. 
The [Cloud Foundry documentation for "Enabling and Configuring TCP Routing"](https://docs.cloudfoundry.org/adminguide/enabling-tcp-routing.html#-modify-tcp-port-reservations) has the following warning and suggestions for valid port ranges:

> Do not enter reservable_ports that conflict with other TCP router instances or ephemeral port ranges. Cloud Foundry recommends using port ranges within 1024-2047 and 18000-32767 on default installations.

These port suggestions do not overlap with any ports used by system components.
However, there is nothing (until now) preventing users from expanding this range into ports that *do* overlap with ports used by system components.

This port conflict can result in two different buggy outcomes.

## 🐛 Bug Variation 1 - TCP Router claims the port first

### Symptoms
1. Some bosh job on the TCP Router VM fails to start. This will likely cause a deployment to fail.
2. There are logs for the failing job that say it was unable to bind to its port. 
```
2020/10/13 22:12:20 Metrics server closing: listen tcp :14726: bind: address already in use
2020/10/13 22:12:20 stopping metrics-agent
```
3. Run `netstat -tlpn | grep PORT` and see that haproxy is running on the port that the bosh job tried to bind to.

### Explanation
If a TCP route gets the port before the bosh job, then the job will fail to bind to its port.


## 🐞 Bug Variation 2 - Internal component claims the port first

### Symptoms
1. You created a tcp route, but it doesnt work.
2. Check the TCP Router logs and see that it failed to bind to the port for the tcp route.
```
{"timestamp":"2020-10-01T21:23:17.526206817Z","level":"info","source":"tcp-router","message":"tcp-router.writing-config","data":{"num-bytes":826}}
{"timestamp":"2020-10-01T21:23:17.526332658Z","level":"info","source":"tcp-router","message":"tcp-router.running-script","data":{}}
{"timestamp":"2020-10-01T21:23:19.581306843Z","level":"info","source":"tcp-router","message":"tcp-router.running-script","data":{"output":"[ALERT] 274/212317 (43) : Starting proxy listen_cfg_2822: cannot bind socket [0.0.0.0:2822]\n"}}
{"timestamp":"2020-10-01T21:23:19.581361142Z","level":"error","source":"tcp-router","message":"tcp-router.failed-to-run-script","data":{"error":"exit status 1"}}
```
3. Run `netstat -tlpn | grep PORT` and see that some other process is running on the port that the TCP route is trying to use.

### Explanation
The TCP Router will fail to load the new config with the new TCP route, because something it bound to the conflicting port. This prevents _ALL_ new TCP routes from working as long as the conflicting port is in the config. This will not cause the bosh job for TCP Router to fail. This bug is dangerous because it is easy to miss and can affect many users.


## 🧰 Fix

### Overview
The fix for this issues focuses on preventing the creation of router groups that conflict with system component ports. We have done this via: 
* a runtime check for creating and updating router groups
* a deploytime check for exising router groups
 
These fixes are available in routing release XYZ+ (will update when released). If you cannot update at this time, you can fix your routing groups manually. See [here](#how-to-manually-fix) for instructions.

### New Bosh Properties

| Bosh Property | Description | Default |
| --- | ----------- | ----------- |
| routing_api.reserved_system_component_ports |   Array of ports that are reserved for system components. Users will not be able to create router_groups with ports that overlap with this value. See Appendix A in this document to see what system components use these ports. If you run anything else on your TCP Router VM you must add its port to this list, or else you run the risk of still running into this bug.  | See Appendix A |
| tcp_router.fail_on_router_port_conflicts | Fail the TCP Router if routing_api.reserved_system_component_ports conflict with ports in existing router groups. We suggest giving your users a chance to update their router groups before turning it to true. | false |
| routing_api.fail_on_router_port_conflicts | By default this is set to the same value as `tcp_router.fail_on_router_port_conflicts`. If true, then API calls to create or update router groups will fail if the reserved_ports conflict with the `routing_api.reserved_system_component_ports`. | false |

### Runtime Check Details

If `routing_api.fail_on_router_port_conflicts` is true, then when a user tries to create or update a router group to include a port in `routing_api.reserved_system_component_ports` they will get a status code 400 and the following error: 
```
{"name":"ProcessRequestError","message":"Cannot process request: Invalid ports. Reservable ports must not include the following reserved system component ports: [2822 2825 3458 3459 3460 3461 8853 9100 14726 14727 14821 14822 14823 14824 14829 15821 17002 35095 39873 40177 42393 46567 53035 53080]."}
```

### Deploytime Check Details

When the TCP Router starts it will check all existing router groups against the `routing_api.reserved_system_component_ports` property. To re-run this check you can monit restart the tcp router.

You will see the following in the TCP Router logs...

**If there are invalid router groups and `tcp_router.fail_on_router_port_conflicts` is false**
1. You will see `tcp-router.router-group-port-checker-error: WARNING! In the future this will cause a deploy failure.` 
2. Plus you will see a list of which router groups contain the conflicting ports.

```
{
  "timestamp": "2021-05-03T20:59:43.127270911Z",
  "level": "error",
  "source": "tcp-router",
  "message": "tcp-router.router-group-port-checker-error: WARNING! In the future this will cause a deploy failure.",
  "data": {
    "error": "The reserved ports for router group 'group-1' contains the following reserved system component port(s): '14726, 14727, 14821, 14822, 14823, 14824, 14829, 15821, 17002'. Please update your router group accordingly.\nThe reserved ports for router group 'group-2' contains the following reserved system component port(s): '40177'. Please update your router group accordingly."
  }
}

```
**If there are invalid router groups and `tcp_router.fail_on_router_port_conflicts` is true**
1. You will see `tcp-router.router-group-port-checker-error: Exiting now.`
2. Plus you will see a list of which router groups contain the conflicting ports.
3. Then monit will report the tcp router as failing

```
{
  "timestamp": "2021-05-03T21:04:02.507129979Z",
  "level": "error",
  "source": "tcp-router",
  "message": "tcp-router.router-group-port-checker-error: Exiting now.",
  "data": {
    "error": "The reserved ports for router group 'group-1' contains the following reserved system component port(s): '14726, 14727, 14821, 14822, 14823, 14824, 14829, 15821, 17002'. Please update your router group accordingly.\nThe reserved ports for router group 'group-2' contains the following reserved system component port(s): '40177'. Please update your router group accordingly."
  }
}
```

**If the seeded router groups in `routing_api.router_groups` are invalid and `routing_api.fail_on_router_port_conflicts` is true**
1. The routing-api job will cause the deployment to fail.
2. You will see the following log in `routing-api.stdout.log`

```
{
  "timestamp": "2021-05-03T21:04:02.507129979Z",
  "source": "routing-api",
  "message": "routing-api.failed-load-config",
  "log_level": 2,
  "data": {
    "error": "Invalid ports. Reservable ports must not include the following reserved system component ports: [2822 2825 3457 3458 3459 3460 3461 8853 9100 14726 14727 14821 14822 14823 14824 14829 14830 14920 14922 15821 17002 53035 53080]."
  }
}

```

**If there are no invalid router groups**
1. You will see `tcp-router.router-group-port-checker-success: No conflicting router group ports.`
```
{
  "timestamp": "2021-05-03T21:08:32.733453194Z",
  "level": "info",
  "source": "tcp-router",
  "message": "tcp-router.router-group-port-checker-success: No conflicting router group ports.",
  "data": {}
}

```

## 🗨️ FAQ

**❓ Do I really need to check the ports running on my TCP Router VM?**

Yes. You might have custom jobs running on your deployment. If you don't include all in-use ports you risk running into this bug that will break TCP routes.

**<a name="how-to-check"></a>❓ How can I see what ports are in use on my TCP Router VM?**
1. Ssh onto your TCP Router VM and become root. 
2. Run `netstat -tlpn | grep -v haproxy`. Ignore haproxy since those are tcp routes and we are looking for system components.
3. To sort them all nicely try this: `netstat -tlpn | grep -v haproxy | cut -d" " -f16 | cut -d":" -f2 | grep -v For | sort -n`

**❓ I see something running on port 22! Why isn't that included in `routing_api.reserved_system_component_ports`?**

Router Groups have never been allowed to use ports 0 - 1023 so you don't need to specifically exclude them.

**❓ Why aren't my ports for udp-forwarder and system-metrics-scraper included in `routing_api.reserved_system_component_ports`?**

Currently these jobs choose any open ephemeral port when they starts. This is problematic for this bug and will be fixed soon. You can track this issue for [udp-forwarder here](https://github.com/cloudfoundry/loggregator-agent-release/issues/44) and [system-metrics-scraper here](https://github.com/cloudfoundry/system-metrics-scraper-release/issues/2). 

<a name="how-to-rerun"></a> **❓ I fixed my router groups. How can I rerun the check?**

You can rerun the check by monit restarting the TCP Router. Or you can wait for the next deploy that will restart the TCP Router. 

**❓ In the logs it says that there is a conflicting port, but everything is running just fine. What's up with that?**

Either (1) you don't have a system component running on that port and everything _is_ fine or (2) you having a ticking time bomb waiting to happen and you will likely run into this bug soon.

To see if there is a system component using that port run `netstat -tlpn | grep PORT` on the TCP Router VM. If there is no system component running there, then you are fine and you can remove the port from `routing_api.reserved_system_component_ports`. If there _is_ a system component running there, then you should update your router group to not include that port ASAP.

<a name="how-to-manually-fix"></a> **❓ I can't upgrade yet. Is there another way I could check to see if there are invalid router groups?**

Yes! You don't need our fancy automation, you can do it yourself. First grab all of the ports from the TCP Router VM (see instructions [here](#how-to-check)). Then grab all of your router groups (see docs [here](https://github.com/cloudfoundry/routing-api/blob/main/docs/api_docs.md#list-router-groups)). Then check all of the router groups to make sure they don't include any of the system component ports.

You will also need to check the router groups seeded in the `routing_api.router_groups` property. Even though this property is only used to seed router groups on the very first deploy, it cannot contain invalid router groups. Either delete these seeded router groups from the manifest (this will have no affect on the current created router groups) or fix the router groups to contain valid ports only.

**❓ Why can't you detect what is running on the VM and see what ports are used? Why is there a deploy time configured list?**

We wanted a runtime _and_ deploytime check for misconfigured router groups. This way we can check all existing router groups and router groups that will be updated and created in the future. It is hard to determine what will be running on a VM at deploytime. We determined that this was the easiest solution.

**❓ Will I ever have to update this list?**

Maybe, but not often. In release notes we will include instructions to update this list if a new system component starts running on the TCP Router VM. Of course if you have your own custom deployment setup then we can't warn you when this happens.

**❓ I got a `router-group-port-checker-error` in the TCP Router logs. What does that mean?**

This error means that the port check was unsuccessful at checking to see if your router groups contain ports that overlap with `routing_api.reserved_system_component_ports`. This can happen for a few reasons: 
* The tcp_router client may not be authorized via UAA to view router groups. See [this PR](https://github.com/cloudfoundry/cf-deployment/pull/923) for an example of how to fix this.
* There could be a problem connecting to uaa. Debug your network connection and then rerun the check.
* There could be a problem connecting to the routing-api. Debug your network connection and then rerun the check.


## 📝 <a name="list-of-ports"></a>Appendix A: Default System Component Ports

This is a list of all of the system components for a default CF-deployment that might be running on the TCP Router VM and their ports. These are the default ports used for the `routing_api.reserved_system_component_ports` property.

Some of these ports are configurable and may not match what is running on your deployment. You are responsible for checking this list against what is running on your deployment.

**Note**: Router Groups have never been allowed to use ports 0 - 1023, so you don't need to specifically exclude them.

| Port | System Component or Job Name | Bosh Property Name | Bosh Link? | Note |
| --- | ----------- |  ---- |  ---- |   ---- | 
| 2822 | monit | n/a | n/a | Not configurable. See [code here](https://github.com/cloudfoundry/bosh-linux-stemcell-builder/blob/add1f114e2aaa19f0cdaf3bc410282d28d683f04/stemcell_builder/stages/bosh_monit/assets/monitrc#L4). |
| 2825 | bosh agent |  n/a | n/a  | Not configurable. See [code here](https://github.com/cloudfoundry/bosh-linux-stemcell-builder/blob/add1f114e2aaa19f0cdaf3bc410282d28d683f04/stemcell_builder/stages/bosh_go_agent/assets/alerts.monitrc#L3). |
| 3457 | loggregator_agent | listening_port | no | See bosh property [here](https://github.com/cloudfoundry/loggregator-agent-release/blob/acfbb6b015d897c11f715ac9e1a226eb5b96875c/jobs/loggregator_agent/spec#L41-L43). |
| 3458 | loggr-forwarder-agent | grpc_port | no | See bosh property [here](https://github.com/cloudfoundry/loggregator-agent-release/blob/acfbb6b015d897c11f715ac9e1a226eb5b96875c/jobs/loggr-forwarder-agent/spec#L18-L20). | 
| 3459 | loggregator_agent | grpc_port | yes | See bosh property [here](https://github.com/cloudfoundry/loggregator-agent-release/blob/acfbb6b015d897c11f715ac9e1a226eb5b96875c/jobs/loggregator_agent/spec#L44-L46). This is overwritten in the default CF-deployment [here](https://github.com/cloudfoundry/cf-deployment/blob/ca5cbab2b9af288cf9c54d9ce13dceeb428fa63c/cf-deployment.yml#L23). | 
| 3460 | loggr-syslog-agent | port | no | This is overwritten in the default CF-deployment [here](https://github.com/cloudfoundry/cf-deployment/blob/ca5cbab2b9af288cf9c54d9ce13dceeb428fa63c/cf-deployment.yml#L67). |
| 3461 | metrics-agent | port | no | See bosh property [here](https://github.com/cloudfoundry/metrics-discovery-release/blob/e8ee61e329b916f0a71274f85fc8b8fcfb8df470/jobs/metrics-agent/spec#L23-L25). |
| 8853 | bosh-dns-health | health.server.port | no | See bosh property [here](https://github.com/cloudfoundry/bosh-dns-release/blob/e8f5ba4233a5fb4b16b5c4ebb203c644fa82db4d/jobs/bosh-dns/spec#L148-L150). |
| 9100 | system-metrics-scraper  | scape_port | no | See bosh property [here](https://github.com/cloudfoundry/system-metrics-scraper-release/blob/473caa08af286e617e7391111639a70846d35de0/jobs/loggr-system-metric-scraper/spec#L42-L44). |
| 14726 | metrics-agent | metrics_exporter_port | no | See bosh property [here](https://github.com/cloudfoundry/metrics-discovery-release/blob/e8ee61e329b916f0a71274f85fc8b8fcfb8df470/jobs/metrics-agent/spec#L45-L47). |
| 14727 | metrics-agent | metrics_exporter_port | no | See bosh property [here](https://github.com/cloudfoundry/metrics-discovery-release/blob/e8ee61e329b916f0a71274f85fc8b8fcfb8df470/jobs/metrics-agent/spec#L48-L50). |
| 14821 | prom-scaper | metrics.port | no | See bosh property [here](https://github.com/cloudfoundry/loggregator-agent-release/blob/acfbb6b015d897c11f715ac9e1a226eb5b96875c/jobs/prom_scraper/spec#L52-L54). |
| 14822 | loggr-syslog-agent | metrics.port | no | See bosh property [here](https://github.com/cloudfoundry/loggregator-agent-release/blob/acfbb6b015d897c11f715ac9e1a226eb5b96875c/jobs/loggr-syslog-agent/spec#L139-L141). |
| 14823 | loggr-forwarder-agent | metrics.port | no | See bosh property [here](https://github.com/cloudfoundry/loggregator-agent-release/blob/acfbb6b015d897c11f715ac9e1a226eb5b96875c/jobs/loggr-forwarder-agent/spec#L51-L53) |
| 14824 | loggregator_agent | metrics.port | no | See bosh property [here](https://github.com/cloudfoundry/loggregator-agent-release/blob/acfbb6b015d897c11f715ac9e1a226eb5b96875c/jobs/loggregator_agent/spec#L78-L80). |
| 14829 | loggr-udp-forwarder | metrics.port | no | See bosh property [here](https://github.com/cloudfoundry/loggregator-agent-release/blob/acfbb6b015d897c11f715ac9e1a226eb5b96875c/jobs/loggr-udp-forwarder/spec#L44-L46). |
| 14830* | loggr-udp-forwarder | n/a | n/a | *Currently this process chooses any open ephemeral port when it starts. This is problematic for this bug. It will be updated soon to always run on 14830. See [this issue](https://github.com/cloudfoundry/loggregator-agent-release/issues/44) for more information. |
| 14920 | system-metrics-scraper | metrics_port | no | See bosh property [here](https://github.com/cloudfoundry/system-metrics-scraper-release/blob/473caa08af286e617e7391111639a70846d35de0/jobs/loggr-system-metric-scraper/spec#L58-L60). |
| 14921* | system-metrics-scraper | n/a | n/a | *Currently this process chooses any open ephemeral port when it starts. This is problematic for this bug. It will be updated soon to always run on 14921. See [this issue](https://github.com/cloudfoundry/system-metrics-scraper-release/issues/2) for more information. |
| 15821 | metrics-discovery-registrar | metrics.port | no | See bosh property [here](https://github.com/cloudfoundry/metrics-discovery-release/blob/e8ee61e329b916f0a71274f85fc8b8fcfb8df470/jobs/metrics-discovery-registrar/spec#L40-L42). |
| 17002 | cf-tcp-router | tcp_router.debug_address| yes | See bosh property [here](https://github.com/cloudfoundry/routing-release/blob/8b00b8ff9ec68802d86425d3ffdcc3e8611aee93/jobs/tcp_router/spec#L32-L34). |
| 53035 | system-metrics-scraper  | metrics_port | no | This is overwritten in the opsfile that enables this in CF-deployment [here](https://github.com/cloudfoundry/cf-deployment/blob/1b2367f37cea2dffa1ab35d5935c08937096bc72/operations/experimental/add-system-metrics-agent.yml#L14). |
| 53080 | bosh-dns| api.port | no | See bosh property [here](https://github.com/cloudfoundry/bosh-dns-release/blob/e8f5ba4233a5fb4b16b5c4ebb203c644fa82db4d/jobs/bosh-dns/spec#L52-L54). |

