import { contextBridge, ipcRenderer } from "electron";
import {
  desktopIpcChannels,
  type MinimumVersionUpgradeState
} from "../../shared/contracts/ipc.ts";

const invoke = <T>(channel: string): Promise<T> => ipcRenderer.invoke(channel);

const minimumVersion = {
  getState: () =>
    invoke<MinimumVersionUpgradeState | null>(
      desktopIpcChannels.update.minimumGetState
    ),
  start: () =>
    invoke<MinimumVersionUpgradeState | null>(
      desktopIpcChannels.update.minimumStart
    ),
  retry: () =>
    invoke<MinimumVersionUpgradeState | null>(
      desktopIpcChannels.update.minimumRetry
    ),
  later: () => invoke<void>(desktopIpcChannels.update.minimumLater),
  openManualDownload: () =>
    invoke<void>(desktopIpcChannels.update.minimumManualDownload),
  exit: () => invoke<void>(desktopIpcChannels.update.minimumExit),
  onState(listener: (state: MinimumVersionUpgradeState) => void): () => void {
    const handler = (
      _event: Electron.IpcRendererEvent,
      payload: MinimumVersionUpgradeState
    ) => listener(payload);
    ipcRenderer.on(desktopIpcChannels.update.minimumState, handler);
    return () => {
      ipcRenderer.removeListener(
        desktopIpcChannels.update.minimumState,
        handler
      );
    };
  }
};

contextBridge.exposeInMainWorld("tuttiMinimumVersion", minimumVersion);
