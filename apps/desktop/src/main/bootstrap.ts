import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  app,
  BrowserWindow,
  ipcMain,
  powerMonitor,
  protocol,
  shell
} from "electron";
import {
  createDesktopUpdateAdmissionController,
  type DesktopUpdateAdmissionController
} from "@tutti-os/desktop-update-admission/electron-main";
import {
  createDevelopmentMinimumVersionChecker,
  resolveDesktopUpdateAdmissionDevelopment
} from "@tutti-os/desktop-update-admission/development";
import {
  initializeDesktopEnvironment,
  resolveDesktopDevelopmentAppName,
  resolveDesktopLoginCallbackUrl,
  resolveDesktopLoginProtocolClientRegistration,
  resolveDesktopUserDataPath
} from "./defaults";
import { registerDesktopAppLifecycle } from "./desktopAppLifecycle";
import { createDesktopAppServices } from "./desktopAppServices";
import { startDesktopAppUpdateAnalytics } from "./appUpdateAnalytics.ts";
import { configureApplicationMenu } from "./applicationMenu.ts";
import { connectAgentPowerSaveBlocker } from "./agentPowerSaveBlocker.ts";
import {
  connectDesktopHostPreferencesEventStream,
  createDesktopHostPreferencesEventStreamClient
} from "./desktopHostPreferencesEventStream";
import {
  createDesktopDeveloperLogsService,
  exportDesktopDeveloperLogsAndNotify
} from "./developerLogsDesktop.ts";
import {
  applyDesktopThemeSource,
  syncDesktopWindowBackgroundColors
} from "./desktopTheme";
import { registerIpcHandlers } from "./ipc/register";
import { flushDesktopLogger, setupDesktopLogger } from "./logging";
import { ensureMacosApplicationInstalled } from "./macosApplicationInstallGuard.ts";
import { ensureSingleInstance } from "./singleInstance";
import {
  completeDesktopLoginCallbackUrl,
  findDesktopLoginCallbackUrl
} from "./desktopLoginCallback";
import { getSystemDesktopLocale } from "./desktopLocale";
import { openDesktopWorkspaceAppFolder } from "./host/workspaceAppFolderAccess";
import { openPerfMonitorDevToolsWindow } from "./windows/perfMonitorDevToolsWindow.ts";
import { createTranslator } from "../shared/i18n/index.ts";
import { registerTuttiAssetProtocol } from "./host/tuttiAssetProtocol.ts";
import { desktopCustomProtocolSchemes } from "./host/desktopCustomProtocolSchemes.ts";
import { createWorkspaceFileIconCacheStore } from "./host/workspaceFileIconCacheStore.ts";
import { registerWorkspaceFileIconProtocol } from "./host/workspaceFileIconProtocol.ts";
import { applyDesktopElectronPlatformCompatibility } from "./electronPlatformCompatibility.ts";
import { createAppUpdateService } from "./update/appUpdateService.ts";
import { createTuttiMinimumVersionChecker } from "./update/minimumVersionPolicyClient.ts";

function envFlagEnabled(value: string | undefined): boolean {
  return /^(1|true|yes|on)$/iu.test(value?.trim() ?? "");
}

function applyElectronDiagnosticSwitches(): void {
  const remoteDebuggingPort =
    process.env.TUTTI_ELECTRON_REMOTE_DEBUGGING_PORT?.trim();
  if (remoteDebuggingPort) {
    app.commandLine.appendSwitch("remote-debugging-port", remoteDebuggingPort);
  }

  const jsFlags = process.env.TUTTI_ELECTRON_JS_FLAGS?.trim();
  if (jsFlags) {
    app.commandLine.appendSwitch("js-flags", jsFlags);
  }
}

function focusPrimaryDesktopWindow(): void {
  const target = BrowserWindow.getAllWindows().find(
    (window) => !window.isDestroyed()
  );
  if (!target) {
    return;
  }
  if (target.isMinimized()) {
    target.restore();
  }
  target.show();
  target.focus();
}

export async function bootstrapDesktopApp(): Promise<void> {
  applyDesktopElectronPlatformCompatibility(app.commandLine);
  applyElectronDiagnosticSwitches();
  initializeDesktopEnvironment({
    appVersion: app.getVersion(),
    isPackaged: app.isPackaged
  });
  protocol.registerSchemesAsPrivileged(desktopCustomProtocolSchemes);
  const loginCallbackUrl = resolveDesktopLoginCallbackUrl();
  const protocolClientRegistration =
    resolveDesktopLoginProtocolClientRegistration({
      isPackaged: app.isPackaged
    });
  if (app.isPackaged) {
    app.setAsDefaultProtocolClient(protocolClientRegistration.scheme);
  }
  const handleLoginCallbackUrl = (url: string): void => {
    void completeDesktopLoginCallbackUrl(url).catch(() => undefined);
  };
  app.on("open-url", (event, url) => {
    if (url.startsWith(loginCallbackUrl)) {
      event.preventDefault();
      handleLoginCallbackUrl(url);
      focusPrimaryDesktopWindow();
    }
  });
  const appName = app.getName();
  const userDataPath = resolveDesktopUserDataPath({
    appDataDir: app.getPath("appData"),
    appName
  });
  if (userDataPath) {
    app.setPath("userData", userDataPath);
  }
  const developmentAppName = resolveDesktopDevelopmentAppName(appName);
  if (developmentAppName) {
    app.setName(developmentAppName);
  }
  const logger = await setupDesktopLogger();

  // A single live desktop instance per environment. The managed tuttid daemon is
  // a global singleton (one pid/listener file per env root); a second instance
  // would otherwise reap the first instance's live daemon as a "stale" orphan,
  // breaking the first instance until it is restarted manually.
  const isPrimaryInstance = ensureSingleInstance({
    requestSingleInstanceLock: () => app.requestSingleInstanceLock(),
    quit: () => app.quit(),
    onSecondInstance: (handler) => {
      app.on("second-instance", (_event, commandLine) => handler(commandLine));
    },
    handleSecondInstanceArgv: (argv) => {
      const callbackUrl = findDesktopLoginCallbackUrl(argv, loginCallbackUrl);
      if (callbackUrl) {
        handleLoginCallbackUrl(callbackUrl);
      }
    },
    focusPrimaryWindow: focusPrimaryDesktopWindow
  });
  if (!isPrimaryInstance) {
    logger.info(
      "secondary tutti instance detected; focusing existing window and quitting"
    );
    return;
  }

  const currentDir = dirname(fileURLToPath(import.meta.url));
  const preloadPath = join(currentDir, "../preload/index.cjs");
  const minimumVersionPreloadPath = join(
    currentDir,
    "../preload/minimum-version.cjs"
  );
  const browserNodeGuestPreloadPath = join(
    currentDir,
    "../preload/browser-node-guest.cjs"
  );
  const workspaceAppPreloadPath = join(
    currentDir,
    "../preload/workspace-app.cjs"
  );
  const rendererUrl = process.env.ELECTRON_RENDERER_URL;

  await app.whenReady();
  const systemLocale = getSystemDesktopLocale();
  const canContinueStartup = await ensureMacosApplicationInstalled({
    appPath: process.execPath,
    isPackaged: app.isPackaged,
    locale: systemLocale,
    logger
  });
  if (!canContinueStartup) {
    return;
  }

  const desktopUpdateAdmission = resolveDesktopUpdateAdmissionDevelopment({
    applicationVersion: app.getVersion(),
    env: process.env,
    isPackaged: app.isPackaged
  });
  const updateService = createAppUpdateService(undefined, {
    currentVersion: desktopUpdateAdmission.runtime.currentVersion,
    developmentScenario: desktopUpdateAdmission.scenario
  });
  const minimumVersionChecker =
    desktopUpdateAdmission.scenario?.transport === "in-process"
      ? createDevelopmentMinimumVersionChecker(desktopUpdateAdmission.scenario)
      : createTuttiMinimumVersionChecker(
          desktopUpdateAdmission.scenario?.mockServerUrl
            ? `${desktopUpdateAdmission.scenario.mockServerUrl}/api/desktop/v1`
            : undefined
        );
  let desktopAppServices: Awaited<
    ReturnType<typeof createDesktopAppServices>
  > | null = null;
  let releaseStartupGate: (() => void) | null = null;
  let minimumVersionController: DesktopUpdateAdmissionController | null =
    createDesktopUpdateAdmissionController({
      checkMinimumVersion: minimumVersionChecker,
      electron: { app, BrowserWindow, ipcMain, shell },
      listBusinessWindows: () => BrowserWindow.getAllWindows(),
      logger,
      manualDownloadUrl: (response) => {
        const channel = response.channel === "rc" ? "preview" : "stable";
        return `https://tutti.sh/desktop/download?channel=${channel}&platform=macos&arch=universal&format=dmg`;
      },
      onPolicyReleased: () => {
        if (releaseStartupGate) {
          const release = releaseStartupGate;
          releaseStartupGate = null;
          release();
        }
      },
      preloadPath: minimumVersionPreloadPath,
      product: "tutti-desktop",
      runtime: desktopUpdateAdmission.runtime,
      rendererFilePath: join(currentDir, "../renderer/minimum-version.html"),
      rendererUrl: rendererUrl
        ? `${rendererUrl}/minimum-version.html`
        : undefined,
      updateService: {
        acquireMandatorySession: (input) =>
          updateService.acquireMandatorySession(input),
        getState: () => updateService.getState(),
        subscribe: (listener) =>
          updateService.onStateChanged((state) => listener(state))
      }
    });
  const startupBlocked = await minimumVersionController.runStartupCheck();
  if (startupBlocked) {
    await new Promise<void>((resolve) => {
      releaseStartupGate = resolve;
    });
  }

  const workspaceFileIconCache = createWorkspaceFileIconCacheStore({
    directory: join(app.getPath("userData"), "workspace-file-icons")
  });
  registerTuttiAssetProtocol();
  registerWorkspaceFileIconProtocol(workspaceFileIconCache);
  desktopAppServices = await createDesktopAppServices({
    appVersion: app.getVersion(),
    enableDevelopmentReloadShortcut: Boolean(rendererUrl) && !app.isPackaged,
    fallbackLocale: systemLocale,
    browserNodeGuestPreloadPath,
    isPackaged: app.isPackaged,
    logger,
    preloadPath,
    rendererUrl,
    updateService,
    workspaceAppPreloadPath
  });
  const theme = applyDesktopThemeSource(
    desktopAppServices.preferences.getThemeSource()
  );
  syncDesktopWindowBackgroundColors();

  void import("electron").then(({ nativeTheme }) => {
    nativeTheme.on("updated", () => {
      if (desktopAppServices.preferences.getThemeSource() !== "system") {
        return;
      }

      syncDesktopWindowBackgroundColors();
    });
  });

  logger.info("desktop app ready", {
    locale: desktopAppServices.preferences.getLocale(),
    rendererUrl: rendererUrl ?? null,
    themeAppearance: theme.appearance,
    themeSource: theme.source
  });
  await flushDesktopLogger();
  await configureApplicationMenu({
    checkForUpdates: () => desktopAppServices.updateService.checkForUpdates(),
    clearDeveloperLogs: () =>
      createDesktopDeveloperLogsService(
        desktopAppServices.preferences,
        desktopAppServices.tuttidClient
      ).clearLogs(),
    exportDeveloperLogs: (input) =>
      exportDesktopDeveloperLogsAndNotify(
        desktopAppServices.preferences,
        desktopAppServices.tuttidClient,
        input
      ),
    getLocale: () => desktopAppServices.preferences.getLocale(),
    logger,
    openPerfMonitorDevTools:
      rendererUrl && envFlagEnabled(process.env.TUTTI_ENABLE_PERF_MONITOR)
        ? (ownerWindow) => {
            const translator = createTranslator(
              desktopAppServices.preferences.getLocale()
            );
            openPerfMonitorDevToolsWindow({
              logger,
              ownerWindow:
                ownerWindow instanceof BrowserWindow ? ownerWindow : null,
              rendererUrl,
              title: translator.t("desktop.menu.openPerfMonitor")
            });
          }
        : undefined
  });

  const ipcDisposables = await registerIpcHandlers({
    daemonEndpoint: desktopAppServices.daemonEndpoint,
    fileDialogs: desktopAppServices.fileDialogs,
    logger,
    workspaceFileIconCache,
    tuttidClient: desktopAppServices.tuttidClient,
    openWorkspaceAppFolder: openDesktopWorkspaceAppFolder,
    preferences: desktopAppServices.preferences,
    updateService: desktopAppServices.updateService,
    workspaceLaunch: desktopAppServices.workspaceLaunch
  });
  const hostPreferencesEventStream = connectDesktopHostPreferencesEventStream({
    applyThemeSource: applyDesktopThemeSource,
    eventStreamClient: createDesktopHostPreferencesEventStreamClient(
      desktopAppServices.daemonEndpoint
    ),
    logger,
    preferences: desktopAppServices.preferences,
    updateService: desktopAppServices.updateService,
    syncWindowBackgroundColors: syncDesktopWindowBackgroundColors
  });
  const agentPowerSaveBlocker = connectAgentPowerSaveBlocker({
    eventStreamClient: createDesktopHostPreferencesEventStreamClient(
      desktopAppServices.daemonEndpoint
    ),
    logger,
    tuttidClient: desktopAppServices.tuttidClient,
    preferences: desktopAppServices.preferences
  });

  const appUpdateAnalytics = startDesktopAppUpdateAnalytics({
    tuttidClient: desktopAppServices.tuttidClient,
    onError(error) {
      logger.warn("failed to record app update analytics", {
        error: error instanceof Error ? error.message : String(error)
      });
    },
    updateService: desktopAppServices.updateService
  });

  let businessWindowAllowed = false;
  let businessWindowOpened = false;
  const openBusinessWindow = async () => {
    businessWindowAllowed = true;
    if (!businessWindowOpened) {
      businessWindowOpened = true;
      await desktopAppServices.workspaceLaunch.openStartupWindow();
    } else {
      focusPrimaryDesktopWindow();
    }
  };
  const checkMinimumVersionAfterRestore = () => {
    void minimumVersionController?.checkAfterForegroundRestore();
  };
  powerMonitor.on("resume", checkMinimumVersionAfterRestore);
  app.on("browser-window-focus", checkMinimumVersionAfterRestore);

  registerDesktopAppLifecycle({
    canOpenBusinessWindow: () => businessWindowAllowed,
    logger,
    tuttid: desktopAppServices.tuttid,
    disposables: [
      ...ipcDisposables,
      hostPreferencesEventStream,
      agentPowerSaveBlocker,
      {
        dispose() {
          appUpdateAnalytics.release();
        }
      },
      {
        dispose() {
          powerMonitor.removeListener(
            "resume",
            checkMinimumVersionAfterRestore
          );
          app.removeListener(
            "browser-window-focus",
            checkMinimumVersionAfterRestore
          );
          minimumVersionController?.dispose();
          minimumVersionController = null;
        }
      }
    ],
    updateService: desktopAppServices.updateService,
    workspaceLaunch: desktopAppServices.workspaceLaunch
  });

  await updateService.configure({
    channel: desktopAppServices.preferences.getUpdateChannel(),
    policy: desktopAppServices.preferences.getUpdatePolicy()
  });
  await openBusinessWindow();
}
