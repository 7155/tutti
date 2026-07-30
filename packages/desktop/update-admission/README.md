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
