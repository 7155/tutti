# Agent Runtime Preparation

Agent session setup is split into two reusable modules:

- `packages/agent/daemon` owns session control and provider protocols.
- `packages/agent/runtimeprep` owns canonical system prompts, skills,
  capability resolution, provider-local files, launch environment overlays,
  manifests, and cleanup.

Both local and VM-backed hosts execute runtime preparation on the machine where
the provider runs. A VM-backed host may use RPC to reach that machine, but the
RPC service is only a transport/path/security adapter and must call the same
`runtimeprep.DefaultPreparer`; it must not maintain separate Claude or Codex
preparers.

Deployment differences are expressed with `DeploymentProfile` and
`CapabilityPack`. A pack resolves policy, skills, and environment together.
Dynamic host skills use `SkillSource`; per-session skills use `ExtraSkills`.
The canonical template and shared skill bodies remain in runtimeprep so hosts
do not fork the actual prompt content.

Cursor preparation materializes one session-scoped skill plugin outside the
workspace and supplies it to `cursor-agent acp` through `--plugin-dir`. Cursor
Agent `2026.07.01-41b2de7` does not merge plugin-provided hooks into its ACP hook
executor: only user, project, and team hook sources are loaded. Runtimeprep
therefore must not advertise or materialize plugin hooks for ACP. A focused
background-Task guard implementation remains dormant with unit coverage so it
can be enabled if Cursor adds that capability; it is not a current runtime
guarantee. Never write an equivalent hook into user or project Cursor config to
work around the provider limitation.

Hermes Agent Extension currently has a narrow temporary runtime-preparation
adapter keyed by `acp:hermes`. Hermes anchors config, auth, and state under
`HERMES_HOME`, and discovers external skills through `skills.external_dirs` in
that home's `config.yaml`. The adapter keeps a per-session `HERMES_HOME`, copies
only the opaque global auth/env files needed for provider login, and reuses the
workspace-scoped `PrepareInput.ExtensionSkillRoots` declared by the signed
extension composer profile. Tutti-managed skills are materialized idempotently
into those extension roots and are referenced through a session config overlay;
the user's native `~/.hermes/skills` directory is appended as an external root
when it exists and is never recursively copied into session state.

Hermes config merge precedence is explicit: the user's original `config.yaml`
content is preserved, existing user `skills.external_dirs` remain first, Tutti
extension workspace roots are appended next, and the user's native Hermes skill
root is appended last. Exact duplicate paths are ignored after their first
occurrence. This provider-specific branch must be removed once Agent Extension
packages can declare constrained runtime-prep overlays, or once Hermes exposes a
native `--skills-dir` or environment contract that the extension can declare
without core runtimeprep knowing the provider identity.

Product-owned responsibilities remain outside the module:

- process or VM transport;
- physical/logical workspace path mapping;
- environment trust filtering;
- account login and token exchange;
- deployment capability availability and profile selection.

See the [runtimeprep package README](../../packages/agent/runtimeprep/README.md)
for the public integration contract.
