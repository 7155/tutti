# @tutti-os/desktop-update-admission

Product-neutral minimum-version admission and forced desktop update mechanics
shared by Tutti Desktop and TSH Desktop.

The package owns the client contract, response validation, startup and
foreground admission lifecycle, mandatory updater lease, Electron upgrade
window binding, preload API factory, shared React presentation, and default
i18n resources.

Consumers still own:

- policy transport (`outboundFetch` for Tutti, desktopd for TSH)
- release-feed resolution and the concrete updater driver
- normal update preferences and scheduling
- product download URLs, logging sinks, window assets, and business-window
  enumeration

The server remains authoritative for deciding whether the installed version is
allowed. The client compares the updater target with the returned minimum only
to prevent installing a release that cannot satisfy the active policy.

## Development scenarios

Both desktop hosts resolve one immutable development scenario from the
`DESKTOP_UPDATE_ADMISSION_*` environment variables. Packaged applications
ignore these variables before parsing them. Invalid enabled scenarios fail
startup instead of falling back to production values.

| Variable                                          | Meaning                                                                                         |
| ------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `DESKTOP_UPDATE_ADMISSION_DEV`                    | Enables the unpackaged-only scenario.                                                           |
| `DESKTOP_UPDATE_ADMISSION_CURRENT_VERSION`        | Supplies the one current version used by policy and updater adapters.                           |
| `DESKTOP_UPDATE_ADMISSION_MINIMUM_VERSION`        | Supplies the default minimum for policy steps that require one.                                 |
| `DESKTOP_UPDATE_ADMISSION_LATEST_VERSION`         | Supplies the updater target for available/downloaded outcomes.                                  |
| `DESKTOP_UPDATE_ADMISSION_POLICY`                 | Selects one policy outcome.                                                                     |
| `DESKTOP_UPDATE_ADMISSION_POLICY_SEQUENCE`        | Selects a comma-separated per-client outcome sequence such as `upgradeRequired@1.1.0,disabled`. |
| `DESKTOP_UPDATE_ADMISSION_UPDATER`                | Selects `available`, `downloaded`, `unavailable`, `error`, or `targetBelowMinimum`.             |
| `DESKTOP_UPDATE_ADMISSION_DOWNLOAD`               | Selects `success` or `error`.                                                                   |
| `DESKTOP_UPDATE_ADMISSION_INSTALL`                | Selects `simulated` or `error`; neither performs a real installation.                           |
| `DESKTOP_UPDATE_ADMISSION_FOREGROUND_INTERVAL_MS` | Overrides the foreground admission interval with an integer of at least 100 ms.                 |
| `DESKTOP_UPDATE_ADMISSION_SCENARIO`               | Selects one named scenario instead of individual policy fields.                                 |
| `DESKTOP_UPDATE_ADMISSION_TRANSPORT`              | Selects `in-process` (default) or `loopback`.                                                   |
| `DESKTOP_UPDATE_ADMISSION_MOCK_SERVER_URL`        | Supplies the exact `http://127.0.0.1` origin required by loopback clients.                      |
| `DESKTOP_UPDATE_ADMISSION_MOCK_SERVER_PORT`       | Selects the loopback CLI port; omit it for an ephemeral port.                                   |

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
`upgradeRequired@1.1.0,disabled` releases the block on retry, while
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

Set `DESKTOP_UPDATE_ADMISSION_TRANSPORT=in-process` for the fast main-process
mock. To exercise the real HTTP path, start the loopback server with the same
scenario variables and a known port:

```bash
DESKTOP_UPDATE_ADMISSION_MOCK_SERVER_PORT=43210 \
pnpm exec desktop-update-admission-mock-server
```

Then set:

```bash
DESKTOP_UPDATE_ADMISSION_TRANSPORT=loopback
DESKTOP_UPDATE_ADMISSION_MOCK_SERVER_URL=http://127.0.0.1:43210
```

The server binds only to `127.0.0.1`. Simulated installation never invokes a
real installer or application restart and is rendered as a distinct
development-only completion state.
