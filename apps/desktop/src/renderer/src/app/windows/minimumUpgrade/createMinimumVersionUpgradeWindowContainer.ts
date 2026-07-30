import type { DesktopMinimumVersionApi } from "@preload/types";

export interface MinimumVersionUpgradeWindowContainer {
  port: DesktopMinimumVersionApi;
}

export function createMinimumVersionUpgradeWindowContainer(): MinimumVersionUpgradeWindowContainer {
  if (!window.tuttiMinimumVersion) {
    throw new Error("minimum-version preload bridge is unavailable");
  }
  return {
    port: window.tuttiMinimumVersion
  };
}
