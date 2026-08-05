import type {
  Connector,
  ConnectorAgentGrantSet,
  ConnectorAgentPrincipal,
  ConnectorAuthorizationResult,
  ConnectorMarketCatalogPage,
  ConnectorMarketCategory,
  ConnectorMarketMutationInput,
  ConnectorMarketSnapshot,
  ConnectorMutationInput,
  InstallConnectorInput,
  ConnectorMutationResult,
  ConnectorOperation
} from "./domain.ts";

export interface ConnectorMarketBackend {
  getSnapshot(): Promise<ConnectorMarketSnapshot>;
  listAgents(): Promise<ConnectorAgentPrincipal[]>;
  listCategories(): Promise<ConnectorMarketCategory[]>;
  listCatalogPage(input: {
    sectionId: string;
    pageSize: number;
    pageToken?: string;
  }): Promise<ConnectorMarketCatalogPage>;
  getConnector(input: { connectorKey: string }): Promise<Connector>;
  getOperation(input: { operationId: string }): Promise<ConnectorOperation>;
  refreshCatalog(
    input: ConnectorMarketMutationInput
  ): Promise<ConnectorMutationResult>;
  installConnector(
    input: InstallConnectorInput
  ): Promise<ConnectorMutationResult>;
  uninstallConnector(
    input: ConnectorMutationInput
  ): Promise<ConnectorMutationResult>;
  beginAuthorization(
    input: ConnectorMutationInput
  ): Promise<ConnectorAuthorizationResult>;
  disconnectAuthorization(
    input: ConnectorMutationInput
  ): Promise<ConnectorMutationResult>;
  getAgentGrants(input: {
    connectorKey: string;
  }): Promise<ConnectorAgentGrantSet>;
  setAgentGrants(
    input: ConnectorMutationInput & { principalIds: string[] }
  ): Promise<ConnectorAgentGrantSet>;
}
