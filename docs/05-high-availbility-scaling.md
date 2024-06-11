---
title: High Availability & Scaling
expires_at: never
tags: [routing-release]
---

# High Availability & Scaling

The TCP Router and Routing API are stateless and horizontally scalable. The TCP
Routers must be fronted by a load balancer for high-availability. The Routing
API depends on a database, that can be clustered for high-availability. For high
availability, deploy multiple instances of each job, distributed across regions
of your infrastructure.
