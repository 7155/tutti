# @tutti-os/desktop-update-admission

Product-neutral minimum-version admission and forced desktop update mechanics
shared by Tutti Desktop and TSH Desktop.

The package owns the client contract, response validation, startup and
foreground admission lifecycle, mandatory updater lease, Electron upgrade
window binding, feature-availability validation and persistent cache, trusted
main IPC registration, preload API factories, shared React presentation, and
default i18n resources.

Consumers still own:

- policy transport (`outboundFetch` for Tutti, desktopd for TSH)
- release-feed resolution and the concrete updater driver
- normal update preferences and scheduling
- product download URLs, logging sinks, window assets, and business-window
  enumeration

The server remains authoritative for deciding whether the installed version is
allowed. The client compares the updater target with the returned minimum only
to prevent installing a release that cannot satisfy the active policy.
Responses contain only the server decision, resolved channel, policy revision,
optional minimum version, and optional feature availability. Request identity
is composed locally and is never echoed by the server; policy-resolution source
details are not part of the public contract.

## Development scenarios

Both desktop hosts resolve one immutable client scenario from the
`DESKTOP_UPDATE_ADMISSION_*` environment variables. Packaged applications
ignore these variables before parsing them. Invalid enabled scenarios fail
startup instead of falling back to production values.

Client-owned variables are:

| Variable                                          | Meaning                                                                             |
| ------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `DESKTOP_UPDATE_ADMISSION_DEV`                    | Enables the unpackaged-only scenario.                                               |
| `DESKTOP_UPDATE_ADMISSION_CURRENT_VERSION`        | Supplies the one current version used by admission requests and the updater driver. |
| `DESKTOP_UPDATE_ADMISSION_LATEST_VERSION`         | Supplies the updater target for available/downloaded outcomes.                      |
| `DESKTOP_UPDATE_ADMISSION_UPDATER`                | Selects `available`, `downloaded`, `unavailable`, `error`, or `targetBelowMinimum`. |
| `DESKTOP_UPDATE_ADMISSION_DOWNLOAD`               | Selects `success` or `error`.                                                       |
| `DESKTOP_UPDATE_ADMISSION_INSTALL`                | Selects `simulated` or `error`; neither performs a real installation.               |
| `DESKTOP_UPDATE_ADMISSION_FOREGROUND_INTERVAL_MS` | Overrides the foreground admission interval with an integer of at least 100 ms.     |
| `DESKTOP_UPDATE_ADMISSION_TRANSPORT`              | Selects `in-process` (default) or `loopback`.                                       |
| `DESKTOP_UPDATE_ADMISSION_MOCK_SERVER_URL`        | Supplies the exact `http://127.0.0.1` origin required by loopback clients.          |

Policy variables are parsed by the in-process client checker or by the
standalone loopback server, never by both:

| Variable                                    | Meaning                                                                                                     |
| ------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `DESKTOP_UPDATE_ADMISSION_MINIMUM_VERSION`  | Supplies the default minimum for policy steps that require one.                                             |
| `DESKTOP_UPDATE_ADMISSION_POLICY`           | Selects one policy outcome.                                                                                 |
| `DESKTOP_UPDATE_ADMISSION_POLICY_SEQUENCE`  | Selects a comma-separated per-client outcome sequence such as `upgradeRequired@1.1.0,minimumNotConfigured`. |
| `DESKTOP_UPDATE_ADMISSION_SCENARIO`         | Selects one named policy scenario instead of individual policy fields.                                      |
| `DESKTOP_UPDATE_ADMISSION_FEATURE_KEYS`     | Supplies a comma-separated feature key list returned by the policy mock.                                    |
| `DESKTOP_UPDATE_ADMISSION_MOCK_SERVER_PORT` | Selects the loopback CLI port; omit it for an ephemeral port.                                               |

The shortest startup-blocking scenario is:

```bash
DESKTOP_UPDATE_ADMISSION_DEV=1 \
DESKTOP_UPDATE_ADMISSION_POLICY=upgradeRequired \
DESKTOP_UPDATE_ADMISSION_CURRENT_VERSION=1.0.0 \
DESKTOP_UPDATE_ADMISSION_MINIMUM_VERSION=1.1.0 \
DESKTOP_UPDATE_ADMISSION_LATEST_VERSION=1.2.0 \
DESKTOP_UPDATE_ADMISSION_UPDATER=available
```

Policy sequences make cross-request behavior deterministic. For example,
`upgradeRequired@1.1.0,minimumNotConfigured` releases the block on retry, while
`allowed@1.0.0,upgradeRequired@1.1.0` allows startup and prompts after the
foreground interval.

Named scenarios are also available:

- `startup-force-success`
- `startup-policy-timeout`
- `startup-updater-unavailable`
- `startup-target-below-minimum`
- `startup-download-error`
- `retry-policy-released`
- `foreground-upgrade-required`

The default `in-process` transport parses client and policy variables in one
process for fast state-machine tests.

To exercise the real HTTP path, give policy variables only to the loopback
server:

```bash
DESKTOP_UPDATE_ADMISSION_DEV=1 \
DESKTOP_UPDATE_ADMISSION_POLICY=upgradeRequired \
DESKTOP_UPDATE_ADMISSION_MINIMUM_VERSION=1.4.0 \
DESKTOP_UPDATE_ADMISSION_MOCK_SERVER_PORT=43210 \
pnpm exec desktop-update-admission-mock-server
```

Then start a client with only client-owned variables:

```bash
DESKTOP_UPDATE_ADMISSION_DEV=1 \
DESKTOP_UPDATE_ADMISSION_CURRENT_VERSION=1.0.0 \
DESKTOP_UPDATE_ADMISSION_TRANSPORT=loopback \
DESKTOP_UPDATE_ADMISSION_MOCK_SERVER_URL=http://127.0.0.1:43210
```

The loopback client defaults its updater to `unavailable`. To exercise a forced
updater flow, add `DESKTOP_UPDATE_ADMISSION_UPDATER=available` and
`DESKTOP_UPDATE_ADMISSION_LATEST_VERSION=1.5.0` to the client command. Policy
variables supplied to a loopback client are rejected so an HTTP test cannot
accidentally gain a second policy source.

The server binds only to `127.0.0.1`. Simulated installation never invokes a
real installer or application restart and is rendered as a distinct
development-only completion state.

## Feature availability

The optional `featureAvailability.keys` response envelope is independent from
the minimum-version decision. A valid envelope replaces the current in-memory
snapshot and the exact-identity cache. A missing or invalid envelope retains
the previous snapshot and cannot change admission behavior.

Desktop hosts create one runtime with the same product, platform,
architecture, and current version used by the admission request. The cache is
stored at `<userData>/desktop-feature-availability-v1.json`, uses atomic
replacement, has no time expiry, and is accepted only when all identity fields
match. It stores no minimum version or admission decision. With no matching
cache every feature query returns `false`.

Main and renderer consumers use the shared runtime and preload API:

```ts
const snapshot = await desktopApi.featureAvailability.getSnapshot();
const enabled = await desktopApi.featureAvailability.isSupported(
  "workspace.exampleFeature"
);
const unsubscribe = desktopApi.featureAvailability.onChanged((next) => {
  console.log(next.keys);
});
```

Hosts must register the shared IPC handlers with a sender check that admits
only their trusted business-window preload. Product code should consume keys
explicitly; the runtime does not merge remote keys into user preferences or
existing feature-flag systems.
