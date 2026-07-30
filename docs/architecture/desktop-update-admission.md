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
- Electron admission-window lifecycle and restricted IPC handlers
- the capability-minimal preload API
- shared React presentation and English and Simplified Chinese defaults

Each host owns:

- the policy transport and trusted endpoint
- the concrete updater, feed resolution, and normal update preferences
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
```

The package must not import product services, product globals, feed URLs,
backend clients, or product translation dictionaries.

## Lifecycle

At startup, a packaged desktop checks policy before business services and
windows start. A failed or timed-out check is fail-open. An upgrade-required
response opens the isolated admission window and holds the startup gate.

After startup, resume and foreground restoration may check again after the
shared 30-minute interval. Only one foreground prompt is shown per process. The
user may defer before starting the forced flow; after it starts, business
windows remain isolated until install or process exit.

The forced flow acquires the updater lease, captures normal configuration,
stops normal scheduling, prepares a channel-matched update, validates its
target, downloads it, validates the downloaded target again, and requests the
mandatory install. Releasing a cleared policy restores the captured normal
configuration.
