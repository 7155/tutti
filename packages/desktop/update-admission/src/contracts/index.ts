export const desktopUpdatePolicies = ["off", "prompt", "auto"] as const;
export type DesktopUpdatePolicy = (typeof desktopUpdatePolicies)[number];

export const desktopUpdateChannels = ["stable", "rc"] as const;
export type DesktopUpdateChannel = (typeof desktopUpdateChannels)[number];

export const desktopUpdateStatuses = [
  "disabled",
  "unsupported",
  "idle",
  "checking",
  "available",
  "downloading",
  "downloaded",
  "up_to_date",
  "error"
] as const;
export type DesktopUpdateStatus = (typeof desktopUpdateStatuses)[number];

export interface ConfigureDesktopUpdatesInput {
  channel: DesktopUpdateChannel;
  policy: DesktopUpdatePolicy;
}

export interface DesktopUpdateState {
  channel: DesktopUpdateChannel;
  checkedAt: string | null;
  currentVersion: string;
  downloadedBytes: number | null;
  downloadPercent: number | null;
  latestVersion: string | null;
  message: string | null;
  policy: DesktopUpdatePolicy;
  releaseDate: string | null;
  releaseName: string | null;
  releaseNotesUrl: string | null;
  status: DesktopUpdateStatus;
  totalBytes: number | null;
}

export const desktopProducts = ["tsh-desktop", "tutti-desktop"] as const;
export type DesktopProduct = (typeof desktopProducts)[number];
export type DesktopPlatform = "macos" | "windows" | "linux";
export type DesktopArchitecture = "arm64" | "x64";

export interface MinimumVersionCheckRequest<
  TProduct extends DesktopProduct = DesktopProduct
> {
  product: TProduct;
  platform: DesktopPlatform;
  architecture: DesktopArchitecture;
  currentVersion: string;
}

export type MinimumVersionDecision =
  | "allowed"
  | "upgradeRequired"
  | "notApplicable";

export interface MinimumVersionCheckResponse<
  TProduct extends DesktopProduct = DesktopProduct
> extends MinimumVersionCheckRequest<TProduct> {
  channel: DesktopUpdateChannel | "unmanaged";
  minimumVersion: string;
  decision: MinimumVersionDecision;
  reason:
    | "unmanagedPrerelease"
    | "productDisabled"
    | "unsupportedRelease"
    | "belowMinimum"
    | "meetsMinimum";
  policySource: "" | "defaultMinimum" | "platformOverride";
  policyRevision: string;
}

export type MinimumVersionUpgradePhase =
  | "blocked"
  | "checking"
  | "ready"
  | "downloading"
  | "downloaded"
  | "error"
  | "released";

export type MinimumVersionUpgradeError =
  | "releaseBelowMinimum"
  | "updateUnavailable"
  | "policyCheckFailed"
  | "installFailed"
  | "updateFailed";

export interface MinimumVersionUpgradeState<
  TProduct extends DesktopProduct = DesktopProduct
> {
  phase: MinimumVersionUpgradePhase;
  check: MinimumVersionCheckResponse<TProduct>;
  update: DesktopUpdateState;
  message: MinimumVersionUpgradeError | null;
}

export interface MandatoryDesktopUpdateTarget {
  channel: DesktopUpdateChannel;
  minimumVersion: string;
  policyRevision: string;
}

export interface MandatoryDesktopUpdateSession {
  retarget(input: MandatoryDesktopUpdateTarget): void;
  prepare(): Promise<DesktopUpdateState>;
  downloadUpdate(): Promise<DesktopUpdateState>;
  installUpdate(): Promise<void>;
  release(options?: { restoreNormal?: boolean }): Promise<void>;
}

export interface MinimumVersionAppUpdateService {
  getState(): DesktopUpdateState;
  acquireMandatorySession(
    input: MandatoryDesktopUpdateTarget
  ): Promise<MandatoryDesktopUpdateSession>;
  subscribe(listener: (state: DesktopUpdateState) => void): () => void;
}

export const desktopUpdateAdmissionIpcChannels = {
  exit: "desktop-update-admission:exit",
  getState: "desktop-update-admission:get-state",
  later: "desktop-update-admission:later",
  manualDownload: "desktop-update-admission:manual-download",
  retry: "desktop-update-admission:retry",
  start: "desktop-update-admission:start",
  state: "desktop-update-admission:state"
} as const;

export interface DesktopMinimumVersionApi {
  getState(): Promise<MinimumVersionUpgradeState | null>;
  start(): Promise<MinimumVersionUpgradeState | null>;
  retry(): Promise<MinimumVersionUpgradeState | null>;
  later(): Promise<void>;
  openManualDownload(): Promise<void>;
  exit(): Promise<void>;
  onState(listener: (state: MinimumVersionUpgradeState) => void): () => void;
}
