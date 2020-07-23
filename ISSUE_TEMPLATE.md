<!-- Thanks for submitting an issue to `routing-release`. We are always trying to improve! To help us, please fill out the following template. →

## Is this a security vulnerability?
<!-- If this bug can cause gorouter to crash or become unreachable then this bug is a security vulnerability. Please report security vulnerabilities here: https://www.cloudfoundry.org/security/.  -->

## Issue
<!-- provide quick introduction so we can triage this issue -->

## Affected Versions
<!-- include version numbers of all relevant components including routing release and cf-deployment-->

## Context

<!-- Provide more detailed description of the bug.--> 
<!-- Please describe what have you done so far to debug the issue? --> 

## Traffic Diagram 

<!-- Often bugs are related to the interaction between load balancers. Please draw a little diagram of the traffic that is failing. Make sure to include all load balances and route services.--> 
```
EXAMPLE DIAGRAM - please draw your own at asciiflow.com
           +----+---+    +----------+     +-------+
  \o/      |        |    |          |     |       |
   +  +--->+ AWS LB +--->+ Gorouter +---->+  App  |
  / \      |        |    |          |     |       |
 client    +--------+    +----------+     +-------+
```

## Steps to Reproduce

<!-- ordered list describing the process to find and recreate the issue -->

## Expected result

<!-- describe what you would expect to have resulted from this process -->

## Current result

<!-- describe what you actually receive from this process -->

## Possible Fix

<!-- not obligatory, but suggest fixes or reasons for the bug -->

## Additional Context

<!-- dumping ground for all additional context we might find interesting, including -->
<!-- logs, configuration yaml, and more screenshots. Use of github's -->
<!-- [details](https://gist.github.com/ericclemmons/b146fe5da72ca1f706b2ef72a20ac39d) -->
<!-- expandable blocks is appreciated. -->
