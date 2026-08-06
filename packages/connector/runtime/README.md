# Connector Runtime

`packages/connector/runtime` is the reusable same-machine Connector runtime
foundation. Tutti runs it on the desktop host; VM-backed products run it inside
the managed guest. It owns secure artifact preparation, managed runtime
identity, runtime ABI verification, and typed Node package installation.

Hosts supply the managed runtime resolver, implementation host, process
transport, HTTP client/proxy policy, state roots, and product-facing command
transport. Runtime code must not import `services/tuttid` or expose host
filesystem paths as a cross-machine protocol.
