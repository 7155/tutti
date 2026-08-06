# Connector SQLite Store

`packages/connector/store-sqlite` is the canonical local SQLite implementation
of the Connector Host repository and changed-event outbox contracts. Hosts own
the selected database path and process lifecycle; the module owns schema,
migration, transactions, revisions, leases, and operation persistence.
