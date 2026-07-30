import { app, BrowserWindow, ipcMain, shell } from "electron";
import type {
  MinimumVersionCheckResponse,
  MinimumVersionUpgradeState
} from "../../shared/contracts/ipc.ts";
import { desktopIpcChannels } from "../../shared/contracts/ipc.ts";
import type { DesktopLogger } from "../logging.ts";
import { outboundFetch } from "../net/outboundFetch.ts";
import type {
  AppUpdateService,
  MandatoryAppUpdateSession
} from "./appUpdateService.ts";
import {
  releaseMeetsMinimum,
  resolveMinimumVersionRuntimeTarget,
  shouldCheckMinimumVersionAfterForeground
} from "./minimumVersionPolicy.ts";
import {
  type MinimumVersionCheckRequest,
  validateMinimumVersionResponse
} from "./minimumVersionContract.ts";

const startupTimeoutMs = 3_000;
const foregroundTimeoutMs = 10_000;
const productionControlPlaneBaseUrl = "https://tutti.sh/api/desktop/v1";
const officialDesktopDownloadUrl = "https://tutti.sh/desktop/download";

interface ControllerOptions {
  logger: DesktopLogger;
  preloadPath: string;
  rendererFilePath: string;
  rendererUrl?: string;
  updateService: AppUpdateService;
  onPolicyReleased(): void;
  normalUpdatePreferences(): {
    channel: "stable" | "rc";
    policy: "off" | "prompt" | "auto";
  };
  now?: () => number;
}

export interface MinimumVersionUpgradeController {
  runStartupCheck(): Promise<boolean>;
  configureNormalUpdates(): void;
  checkAfterForegroundRestore(): Promise<void>;
  dispose(): void;
}

function logCheck(
  logger: DesktopLogger,
  level: "info" | "error",
  details: Record<string, unknown>
): void {
  logger[level](`[minimum-version-check] ${JSON.stringify(details)}`);
}

function requestPayload(): MinimumVersionCheckRequest | null {
  const target = resolveMinimumVersionRuntimeTarget(
    process.platform,
    process.arch
  );
  if (!target) {
    return null;
  }
  return {
    product: "tutti-desktop",
    ...target,
    currentVersion: app.getVersion()
  };
}

async function checkMinimumVersion(
  payload: NonNullable<ReturnType<typeof requestPayload>>,
  signal?: AbortSignal
): Promise<MinimumVersionCheckResponse> {
  const configuredDevelopmentBaseUrl =
    !app.isPackaged && process.env.TUTTI_DESKTOP_CONTROL_PLANE_BASE_URL
      ? process.env.TUTTI_DESKTOP_CONTROL_PLANE_BASE_URL
      : productionControlPlaneBaseUrl;
  const baseUrl = new URL(configuredDevelopmentBaseUrl);
  if (
    app.isPackaged &&
    (baseUrl.protocol !== "https:" ||
      baseUrl.origin !== new URL(productionControlPlaneBaseUrl).origin)
  ) {
    throw new Error("packaged minimum-version control plane origin is invalid");
  }
  const endpoint = `${baseUrl.toString().replace(/\/+$/u, "")}/public/desktop-version/check`;
  const response = await outboundFetch(endpoint, {
    body: JSON.stringify(payload),
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      "User-Agent": "Tutti Desktop"
    },
    method: "POST",
    signal
  });
  if (!response.ok) {
    throw new Error(`minimum version check returned HTTP ${response.status}`);
  }
  return validateMinimumVersionResponse(await response.json(), payload);
}

export function createMinimumVersionUpgradeController(
  options: ControllerOptions
): MinimumVersionUpgradeController {
  const now = options.now ?? Date.now;
  let lastCheckAt = 0;
  let foregroundPrompted = false;
  let state: MinimumVersionUpgradeState | null = null;
  let window: BrowserWindow | null = null;
  let mode: "startup" | "foreground" = "startup";
  let forcedFlowStarted = false;
  let installRequested = false;
  let disposed = false;
  let activeCheck: Promise<MinimumVersionCheckResponse | null> | null = null;
  let appQuitStarted = false;
  let mandatoryUpdateSession: MandatoryAppUpdateSession | null = null;
  let isolatedBusinessWindows: Array<{
    window: BrowserWindow;
    wasFocused: boolean;
    wasMinimized: boolean;
    wasVisible: boolean;
  }> | null = null;
  const lifecycleAbort = new AbortController();

  const handleBeforeQuit = () => {
    appQuitStarted = true;
  };
  app.on("before-quit", handleBeforeQuit);

  const emitState = () => {
    if (state && window && !window.isDestroyed()) {
      window.webContents.send(desktopIpcChannels.update.minimumState, state);
    }
  };
  const applyState = (
    phase: MinimumVersionUpgradeState["phase"],
    update = options.updateService.getState(),
    message: string | null = null
  ) => {
    if (state) {
      state = { ...state, phase, update, message };
      emitState();
    }
  };
  const closeWindow = () => {
    if (window && !window.isDestroyed()) {
      window.destroy();
    }
    window = null;
  };
  const isolateBusinessWindows = () => {
    if (isolatedBusinessWindows) {
      return;
    }
    isolatedBusinessWindows = BrowserWindow.getAllWindows()
      .filter((candidate) => candidate !== window && !candidate.isDestroyed())
      .map((candidate) => ({
        window: candidate,
        wasFocused: candidate.isFocused(),
        wasMinimized: candidate.isMinimized(),
        wasVisible: candidate.isVisible()
      }));
    for (const snapshot of isolatedBusinessWindows) {
      snapshot.window.hide();
    }
  };
  const restoreBusinessWindows = () => {
    const snapshots = isolatedBusinessWindows;
    isolatedBusinessWindows = null;
    if (!snapshots) {
      return;
    }
    let focusedWindow: BrowserWindow | null = null;
    for (const snapshot of snapshots) {
      if (snapshot.window.isDestroyed()) {
        continue;
      }
      if (snapshot.wasMinimized) {
        snapshot.window.show();
        snapshot.window.minimize();
      } else if (snapshot.wasVisible) {
        snapshot.window.show();
      }
      if (snapshot.wasFocused) {
        focusedWindow = snapshot.window;
      }
    }
    if (
      focusedWindow &&
      !focusedWindow.isDestroyed() &&
      !focusedWindow.isMinimized()
    ) {
      focusedWindow.focus();
    }
  };
  const openWindow = (nextMode: "startup" | "foreground") => {
    if (window && !window.isDestroyed()) {
      window.show();
      window.focus();
      return;
    }
    mode = nextMode;
    window = new BrowserWindow({
      width: 520,
      height: 420,
      minWidth: 480,
      minHeight: 380,
      resizable: false,
      maximizable: false,
      fullscreenable: false,
      autoHideMenuBar: true,
      show: false,
      webPreferences: {
        preload: options.preloadPath,
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: true
      }
    });
    window.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
    window.webContents.on("will-navigate", (event) => event.preventDefault());
    window.on("close", (event) => {
      if (!appQuitStarted && (mode === "startup" || forcedFlowStarted)) {
        event.preventDefault();
        app.quit();
      }
    });
    window.once("ready-to-show", () => window?.show());
    const search = `mode=${nextMode}`;
    if (options.rendererUrl) {
      void window.loadURL(
        `${options.rendererUrl}/minimum-version.html?${search}`
      );
    } else {
      void window.loadFile(options.rendererFilePath, { search });
    }
  };

  const runPolicyCheck = async (
    bounded: boolean
  ): Promise<MinimumVersionCheckResponse | null> => {
    const payload = requestPayload();
    if (!payload) {
      lastCheckAt = now();
      logCheck(options.logger, "info", {
        stage: bounded ? "startup" : "foreground",
        result: "success",
        decision: "notApplicable",
        reason: "unsupportedRuntime",
        platform: process.platform,
        architecture: process.arch
      });
      return null;
    }
    const startedAt = now();
    const controller = new AbortController();
    const abortForLifecycle = () => controller.abort();
    lifecycleAbort.signal.addEventListener("abort", abortForLifecycle, {
      once: true
    });
    let timer: ReturnType<typeof setTimeout> | null = null;
    try {
      const timeoutMs = bounded ? startupTimeoutMs : foregroundTimeoutMs;
      const result = await Promise.race([
        checkMinimumVersion(payload, controller.signal),
        new Promise<never>((_resolve, reject) => {
          timer = setTimeout(() => {
            controller.abort();
            reject(new Error("minimum version check timed out"));
          }, timeoutMs);
        })
      ]);
      lastCheckAt = now();
      logCheck(options.logger, "info", {
        stage: bounded ? "startup" : "foreground",
        result: "success",
        decision: result.decision,
        reason: result.reason,
        currentVersion: result.currentVersion,
        minimumVersion: result.minimumVersion,
        policyRevision: result.policyRevision,
        elapsedMs: now() - startedAt
      });
      return result;
    } catch (error) {
      lastCheckAt = now();
      logCheck(options.logger, "error", {
        stage: bounded ? "startup" : "foreground",
        result: "failure",
        error: error instanceof Error ? error.message : String(error),
        elapsedMs: now() - startedAt
      });
      return null;
    } finally {
      if (timer) {
        clearTimeout(timer);
      }
      lifecycleAbort.signal.removeEventListener("abort", abortForLifecycle);
    }
  };
  const checkPolicy = async (
    bounded: boolean
  ): Promise<MinimumVersionCheckResponse | null> => {
    if (activeCheck) {
      return activeCheck;
    }
    const pendingCheck = runPolicyCheck(bounded);
    activeCheck = pendingCheck;
    try {
      return await pendingCheck;
    } finally {
      if (activeCheck === pendingCheck) {
        activeCheck = null;
      }
    }
  };
  const configureNormalUpdates = async (): Promise<void> => {
    try {
      await options.updateService.configure(options.normalUpdatePreferences());
    } catch (error) {
      logCheck(options.logger, "error", {
        stage: "normal-update-configure",
        result: "failure",
        error: error instanceof Error ? error.message : String(error)
      });
    }
  };

  const releaseMandatoryUpdater = async (
    restoreNormal = true
  ): Promise<void> => {
    const session = mandatoryUpdateSession;
    mandatoryUpdateSession = null;
    try {
      await session?.release({ restoreNormal });
    } catch (error) {
      logCheck(options.logger, "error", {
        stage: "normal-update-restore",
        result: "failure",
        error: error instanceof Error ? error.message : String(error)
      });
    }
  };

  const releaseBlock = async (): Promise<void> => {
    forcedFlowStarted = false;
    installRequested = false;
    await releaseMandatoryUpdater();
    closeWindow();
    restoreBusinessWindows();
    options.onPolicyReleased();
  };

  const installForcedUpdate = async (): Promise<void> => {
    if (installRequested) {
      return;
    }
    installRequested = true;
    try {
      if (!mandatoryUpdateSession) {
        throw new Error("mandatory update session is unavailable");
      }
      await mandatoryUpdateSession.installUpdate();
    } catch (error) {
      installRequested = false;
      throw error;
    }
  };

  const prepareUpdate = async () => {
    if (!state) {
      return;
    }
    try {
      forcedFlowStarted = true;
      applyState("checking");
      mandatoryUpdateSession ??=
        await options.updateService.acquireMandatorySession({
          channel: state.check.channel === "rc" ? "rc" : "stable",
          minimumVersion: state.check.minimumVersion,
          policyRevision: state.check.policyRevision,
          releaseMeetsMinimum
        });
      await mandatoryUpdateSession.configure();
      const update = await mandatoryUpdateSession.checkForUpdates();
      if (
        (update.status !== "available" && update.status !== "downloaded") ||
        !releaseMeetsMinimum(update.latestVersion, state.check.minimumVersion)
      ) {
        applyState("error", update, "releaseBelowMinimum");
        return;
      }
      if (update.status === "downloaded") {
        applyState("downloaded", update);
        await installForcedUpdate();
        return;
      }
      applyState("ready", update);
      const downloaded = await mandatoryUpdateSession.downloadUpdate();
      if (
        downloaded.status !== "downloaded" ||
        !releaseMeetsMinimum(
          downloaded.latestVersion,
          state.check.minimumVersion
        )
      ) {
        applyState(
          "error",
          downloaded,
          downloaded.status === "downloaded"
            ? "releaseBelowMinimum"
            : "updateFailed"
        );
        return;
      }
      applyState("downloaded", downloaded);
      await installForcedUpdate();
    } catch (error) {
      logCheck(options.logger, "error", {
        stage: "forced-update",
        result: "failure",
        error: error instanceof Error ? error.message : String(error)
      });
      applyState("error", options.updateService.getState(), "updateFailed");
    }
  };

  const unsubscribeUpdate = options.updateService.onStateChanged((update) => {
    if (!state || !forcedFlowStarted) {
      return;
    }
    if (update.status === "downloading") {
      applyState("downloading", update);
    } else if (update.status === "downloaded") {
      applyState("downloaded", update);
    } else if (update.status === "error") {
      installRequested = false;
      applyState("error", update, "updateFailed");
    }
  });

  const assertUpgradeWindowSender = (senderId: number): void => {
    if (!window || window.isDestroyed() || window.webContents.id !== senderId) {
      throw new Error(
        "minimum-version IPC is restricted to the upgrade window"
      );
    }
  };
  ipcMain.handle(desktopIpcChannels.update.minimumGetState, (event) => {
    assertUpgradeWindowSender(event.sender.id);
    return state;
  });
  ipcMain.handle(desktopIpcChannels.update.minimumStart, async (event) => {
    assertUpgradeWindowSender(event.sender.id);
    if (mode === "foreground") {
      isolateBusinessWindows();
    }
    await prepareUpdate();
    return state;
  });
  ipcMain.handle(desktopIpcChannels.update.minimumRetry, async (event) => {
    assertUpgradeWindowSender(event.sender.id);
    const response = await checkPolicy(false);
    if (!response) {
      applyState(
        "error",
        options.updateService.getState(),
        "policyCheckFailed"
      );
    } else if (response.decision !== "upgradeRequired") {
      await releaseBlock();
    } else if (state) {
      await releaseMandatoryUpdater(false);
      state = { ...state, check: response };
      await prepareUpdate();
    }
    return state;
  });
  ipcMain.handle(desktopIpcChannels.update.minimumLater, (event) => {
    assertUpgradeWindowSender(event.sender.id);
    if (mode === "foreground" && !forcedFlowStarted) {
      closeWindow();
    }
  });
  ipcMain.handle(desktopIpcChannels.update.minimumManualDownload, (event) => {
    assertUpgradeWindowSender(event.sender.id);
    return shell.openExternal(
      `${officialDesktopDownloadUrl}?channel=${state?.check.channel === "rc" ? "preview" : "stable"}&platform=macos&arch=universal&format=dmg`
    );
  });
  ipcMain.handle(desktopIpcChannels.update.minimumExit, (event) => {
    assertUpgradeWindowSender(event.sender.id);
    app.quit();
  });

  return {
    async runStartupCheck() {
      if (!app.isPackaged) {
        return false;
      }
      const response = await checkPolicy(true);
      if (!response || response.decision !== "upgradeRequired") {
        return false;
      }
      state = {
        phase: "blocked",
        check: response,
        update: options.updateService.getState(),
        message: null
      };
      openWindow("startup");
      return true;
    },
    configureNormalUpdates() {
      void configureNormalUpdates();
    },
    async checkAfterForegroundRestore() {
      if (
        !shouldCheckMinimumVersionAfterForeground({
          disposed,
          packaged: app.isPackaged,
          foregroundPrompted,
          startupBlocked:
            mode === "startup" && window !== null && !window.isDestroyed(),
          lastCheckAt,
          now: now()
        })
      ) {
        return;
      }
      const response = await checkPolicy(false);
      if (disposed || !response || response.decision !== "upgradeRequired") {
        return;
      }
      foregroundPrompted = true;
      state = {
        phase: "blocked",
        check: response,
        update: options.updateService.getState(),
        message: null
      };
      openWindow("foreground");
    },
    dispose() {
      disposed = true;
      lifecycleAbort.abort();
      void releaseMandatoryUpdater(false);
      app.removeListener("before-quit", handleBeforeQuit);
      unsubscribeUpdate();
      closeWindow();
      for (const channel of [
        desktopIpcChannels.update.minimumGetState,
        desktopIpcChannels.update.minimumStart,
        desktopIpcChannels.update.minimumRetry,
        desktopIpcChannels.update.minimumLater,
        desktopIpcChannels.update.minimumManualDownload,
        desktopIpcChannels.update.minimumExit
      ]) {
        ipcMain.removeHandler(channel);
      }
    }
  };
}
