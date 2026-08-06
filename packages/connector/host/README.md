# Connector Host

`packages/connector/host` is the host-neutral Connector application core. It
owns catalog acceptance, installation and authorization state, durable
operation transitions, compatibility evaluation, recovery, reconcile intent,
manifest validation, and the ports implemented by daemon and runtime hosts.

The package contains no HTTP client, SQLite driver, product account state,
Electron API, absolute state root, or operating-system process policy.
Lifecycle behavior belongs here when Tutti and another daemon host must observe
the same result.
