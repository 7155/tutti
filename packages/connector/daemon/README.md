# Connector Daemon

`packages/connector/daemon` composes the Connector Host application inside a
long-running desktop daemon. It owns bootstrap fencing, recovery ordering,
operation scheduling, catalog refresh/reconcile scheduling, and durable outbox
delivery.

The module provides the market catalog projection, while hosts inject their
HTTP client/proxy policy, request authorization, event publication,
persistence, and execution ports. Product account policy and generated HTTP
handlers remain in the consuming daemon.
