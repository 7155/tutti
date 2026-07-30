import { ipcRenderer } from "electron";
import {
  desktopIpcChannels,
  type AppUpdateState,
  type ConfigureAppUpdatesInput
} from "../../shared/contracts/ipc";
import type { DesktopUpdateApi } from "../types";
import { invokeDesktopApi } from "./invoke";

export function createUpdateDesktopApi(): DesktopUpdateApi {
  return {
    checkForUpdates(): Promise<AppUpdateState> {
      return invokeDesktopApi(desktopIpcChannels.update.check);
    },
    configure(payload: ConfigureAppUpdatesInput): Promise<AppUpdateState> {
      return invokeDesktopApi(desktopIpcChannels.update.configure, payload);
    },
    downloadUpdate(): Promise<AppUpdateState> {
      return invokeDesktopApi(desktopIpcChannels.update.download);
    },
    getState(): Promise<AppUpdateState> {
      return invokeDesktopApi(desktopIpcChannels.update.getState);
    },
    installUpdate(): Promise<void> {
      return invokeDesktopApi(desktopIpcChannels.update.install);
    },
    onState(listener: (state: AppUpdateState) => void): () => void {
      const handler = (
        _event: Electron.IpcRendererEvent,
        payload: AppUpdateState
      ) => {
        listener(payload);
      };

      ipcRenderer.on(desktopIpcChannels.update.state, handler);

      return () => {
        ipcRenderer.removeListener(desktopIpcChannels.update.state, handler);
      };
    }
  };
}
