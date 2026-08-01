# Desktop Update Admission

`@tutti-os/desktop-update-admission` is the shared minimum-version admission
boundary for Tutti Desktop and TSH Desktop. It keeps policy enforcement and the
forced-upgrade lifecycle identical while leaving product integrations in each
host.

## Ownership

The package owns:

- request and response contracts for `tutti-desktop` and `tsh-desktop`
- exact response identity and policy-shape validation
- startup and foreground check timing
- the one-prompt-per-process foreground rule
- exclusive mandatory-updater ownership and minimum-target validation
- immutable unpackaged development scenarios, policy/updater mocks, and the
  loopback policy server
- Electron admission-window lifecycle and restricted IPC handlers
- feature-key envelope validation, exact-identity persistent cache, immutable
  snapshots, membership queries, subscriptions, and trusted renderer IPC
- the capability-minimal preload API
- shared React presentation and English and Simplified Chinese defaults

Each host owns:

- the policy transport and trusted endpoint
- the production updater, feed resolution, and normal update preferences
- product download URLs, icon and renderer paths, logging sinks, and the list
  of business windows to isolate

The policy service remains authoritative for deciding whether the installed
version is allowed. The package validates the updater target independently so
that a forced flow never installs a release below the active minimum.

## Dependency Direction

```text
product bootstrap
  -> product transport adapter
  -> @tutti-os/desktop-update-admission controller
  -> product updater adapter

admission renderer
  -> @tutti-os/desktop-update-admission React UI
  -> @tutti-os/desktop-update-admission preload API
  -> shared controller IPC

business renderer
  -> product preload API
  -> shared feature-availability IPC
  -> shared feature-availability runtime
```

The package must not import product services, product globals, feed URLs,
backend clients, or product translation dictionaries.

## Lifecycle

At startup, a packaged desktop checks policy before business services and
windows start. A failed or timed-out check is fail-open. An upgrade-required
response opens the isolated admission window and holds the startup gate.

Before that check, the feature runtime loads only a cache matching the exact
product, platform, architecture, and current version. A successful v4 response
updates the snapshot before the business window starts and persists it with an
atomic file replacement. Missing, malformed, failed, or timed-out feature
responses retain the snapshot. A valid empty key list explicitly clears it.
The cache has no time expiry and stores no minimum version or admission
decision, so it cannot block startup or affect the forced-upgrade state.

After startup, resume and foreground restoration may check again after the
shared 30-minute interval. Only one foreground prompt is shown per process. The
user may defer before starting the forced flow; after it starts, business
windows remain isolated until install or process exit.

The forced flow acquires the updater lease, captures normal configuration,
stops normal scheduling, prepares a channel-matched update, validates its
target, downloads it, validates the downloaded target again, and requests the
mandatory install. Releasing a cleared policy restores the captured normal
configuration.

## Development boundary

The package resolves client-owned `DESKTOP_UPDATE_ADMISSION_*` variables once
in the Electron main process. The resulting runtime injects `checksEnabled`,
`currentVersion`, and `foregroundCheckIntervalMs` into the controller, and the
same `currentVersion` configures both admission requests and the updater
driver. Product adapters do not read scenario variables or implement their own
mock state machines.

The in-process transport also resolves a local immutable policy scenario. The
loopback transport instead resolves no client-side policy: the standalone mock
server exclusively parses policy, minimum-version, feature-key, sequence, and
named-policy variables and evaluates them against the `currentVersion` in each
HTTP request.
The client parser rejects those server-owned variables in loopback mode. This
keeps Tutti's Electron-to-`outboundFetch` path and TSH's
Electron-to-desktopd-to-HTTP path as real transport tests with one policy
authority.

Packaged applications ignore the environment family before parsing it. Enabled
invalid scenarios fail startup. The optional HTTP server binds only to
`127.0.0.1`; TSH routes it through desktopd's dedicated desktop-version client
while Tutti Desktop uses its normal outbound policy transport. Simulated
installation terminates in an explicit development-only state and never invokes
the production installer or restart path.
