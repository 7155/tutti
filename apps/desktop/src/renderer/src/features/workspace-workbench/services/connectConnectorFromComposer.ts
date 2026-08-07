import type { ConnectorMarketRootService } from "@tutti-os/connector-market/services";

export type ComposerConnectorConnectResult =
  | "authorization-started"
  | "installation-started"
  | "no-action";

export type ComposerConnectorOpenResult = "dialog-opened" | "no-action";

export async function openConnectorDialogFromComposer(
  root: ConnectorMarketRootService,
  connectorKey: string
): Promise<ComposerConnectorOpenResult> {
  const normalizedConnectorKey = connectorKey.trim();
  if (!normalizedConnectorKey) {
    return "no-action";
  }

  await root.market.ensureLoaded();
  if (!root.view.dataStore.cardsByKey[normalizedConnectorKey]) {
    return "no-action";
  }
  root.uiState.openConnector(normalizedConnectorKey);
  return "dialog-opened";
}

export async function connectConnectorFromComposer(
  root: ConnectorMarketRootService,
  connectorKey: string
): Promise<ComposerConnectorConnectResult> {
  const normalizedConnectorKey = connectorKey.trim();
  if (!normalizedConnectorKey) {
    return "no-action";
  }

  await root.market.ensureLoaded();
  const action = root.view.dataStore.cardsByKey[normalizedConnectorKey]?.action;
  if (action === "authorize") {
    await root.market.beginAuthorization(normalizedConnectorKey);
    return "authorization-started";
  }
  if (action === "install" || action === "update") {
    await root.market.install(normalizedConnectorKey);
    return "installation-started";
  }
  return "no-action";
}
