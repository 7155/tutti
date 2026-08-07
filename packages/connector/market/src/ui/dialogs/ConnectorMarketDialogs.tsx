import { Dialog, Toast } from "@tutti-os/ui-system/components";
import { useEffect, useState } from "react";
import { useSnapshot } from "valtio";

import { useConnectorMarketServices } from "../ConnectorMarketServicesContext.tsx";
import { ConnectorAuthorizationDialog } from "./ConnectorAuthorizationDialog.tsx";
import { ConnectorBlockedDialog } from "./ConnectorBlockedDialog.tsx";
import { ConnectorManagementDialog } from "./ConnectorManagementDialog.tsx";
import { ConnectorInstallationDialog } from "./ConnectorInstallationDialog.tsx";

export function ConnectorMarketDialogs() {
  const { i18n, market, onError, onTryConnector, uiState, view } =
    useConnectorMarketServices();
  const dialog = useSnapshot(view.dataStore).dialog;

  const [showInstallSuccess, setShowInstallSuccess] = useState(false);
  const [lastInstalling, setLastInstalling] = useState(false);

  // Detect when installation completes
  useEffect(() => {
    if (!dialog && lastInstalling) {
      setShowInstallSuccess(true);
      setLastInstalling(false);
    } else if (dialog?.kind === "installation" && dialog.installing) {
      setLastInstalling(true);
      setShowInstallSuccess(false);
    }
  }, [dialog, lastInstalling]);

  // Hide management dialog when installation completes
  const shouldHideDialog = dialog?.kind === "management" && showInstallSuccess;

  if (!dialog && !showInstallSuccess) {
    return null;
  }

  return (
    <>
      {dialog && !shouldHideDialog && (
        <Dialog
          open
          onOpenChange={(open) =>
            !open &&
            !(dialog.kind === "installation" && dialog.installing) &&
            uiState.closeDialog()
          }
        >
          {dialog.kind === "installation" ? (
            <ConnectorInstallationDialog
              description={dialog.description}
              displayName={dialog.displayName}
              iconUrl={dialog.iconUrl}
              i18n={i18n}
              installing={dialog.installing}
              updating={dialog.updating}
              onClose={() => uiState.closeDialog()}
              onInstall={() =>
                void market.install(dialog.connectorKey).catch(() => {
                  onError?.(
                    i18n.t(
                      dialog.updating
                        ? "connectorUpdateFailed"
                        : "connectorInstallFailed"
                    )
                  );
                })
              }
            />
          ) : dialog.kind === "authorization" ? (
            <ConnectorAuthorizationDialog
              authorizing={dialog.authorizing}
              displayName={dialog.displayName}
              iconUrl={dialog.iconUrl}
              i18n={i18n}
              pending={dialog.pending}
              onAuthorize={() =>
                market.beginAuthorization(dialog.connectorKey).catch(() => {
                  onError?.(i18n.t("connectorAuthorizationFailed"));
                })
              }
              onClose={() => uiState.closeDialog()}
            />
          ) : dialog.kind === "management" ? (
            <ConnectorManagementDialog
              description={dialog.description}
              displayName={dialog.displayName}
              iconUrl={dialog.iconUrl}
              i18n={i18n}
              onDisconnect={() => {
                const disconnect = dialog.canAuthorize
                  ? market.disconnectAuthorization(dialog.connectorKey)
                  : market.uninstall(dialog.connectorKey);
                void disconnect
                  .then(() => uiState.closeDialog())
                  .catch(() => undefined);
              }}
              onTry={() => {
                uiState.closeDialog();
                onTryConnector?.(dialog.connectorKey);
              }}
            />
          ) : (
            <ConnectorBlockedDialog
              displayName={dialog.displayName}
              iconUrl={dialog.iconUrl}
              i18n={i18n}
              reason={dialog.reason}
              onClose={() => uiState.closeDialog()}
            />
          )}
        </Dialog>
      )}
      {showInstallSuccess && (
        <Toast
          variant="success"
          onOpenChange={(open) => !open && setShowInstallSuccess(false)}
        >
          {i18n.t("actionInstallSuccess")}
        </Toast>
      )}
    </>
  );
}
