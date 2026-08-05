package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	market "github.com/tutti-os/tutti/packages/connector/market/daemon"
	tuttigenerated "github.com/tutti-os/tutti/services/tuttid/api/generated"
)

type stubConnectorMarketService struct {
	market.Service
	snapshotFn   func(context.Context) (market.Snapshot, error)
	categoriesFn func(context.Context) ([]market.CatalogCategory, error)
	pageFn       func(context.Context, market.CatalogPageQuery) (market.CatalogPage, error)
	installFn    func(context.Context, market.ConnectorMutation) (market.MutationResult, error)
	agentsFn     func(context.Context) ([]market.AgentPrincipal, error)
	grantsFn     func(context.Context, market.SetAgentGrantsCommand) (market.AgentGrantSet, error)
}

func (service stubConnectorMarketService) ListAgentPrincipals(ctx context.Context) ([]market.AgentPrincipal, error) {
	return service.agentsFn(ctx)
}

func (service stubConnectorMarketService) SetAgentGrants(ctx context.Context, command market.SetAgentGrantsCommand) (market.AgentGrantSet, error) {
	return service.grantsFn(ctx, command)
}

func (service stubConnectorMarketService) Snapshot(ctx context.Context) (market.Snapshot, error) {
	return service.snapshotFn(ctx)
}

func (service stubConnectorMarketService) Install(ctx context.Context, mutation market.ConnectorMutation) (market.MutationResult, error) {
	return service.installFn(ctx, mutation)
}

func (service stubConnectorMarketService) ListCatalogCategories(ctx context.Context) ([]market.CatalogCategory, error) {
	return service.categoriesFn(ctx)
}

func (service stubConnectorMarketService) ListCatalogPage(ctx context.Context, query market.CatalogPageQuery) (market.CatalogPage, error) {
	return service.pageFn(ctx, query)
}

func TestDaemonAPIConnectorMarketSnapshotHidesImplementationConfig(t *testing.T) {
	service := stubConnectorMarketService{
		snapshotFn: func(_ context.Context) (market.Snapshot, error) {
			return market.Snapshot{
				CatalogState:   market.CatalogStateReady,
				Connectors:     []market.Connector{connectorMarketTestConnector()},
				Operations:     []market.Operation{},
				Revision:       7,
				SourceRevision: "sha256:catalog",
			}, nil
		},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewRoutes(DaemonAPI{ConnectorMarketService: service}))

	recorder := performGeneratedRouteRequest(t, mux, http.MethodGet, "/v1/connector-market", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var raw map[string]any
	decodeGeneratedRouteResponse(t, recorder, &raw)
	connectors := raw["connectors"].([]any)
	connector := connectors[0].(map[string]any)
	release := connector["release"].(map[string]any)
	manifest := release["manifest"].(map[string]any)
	implementation := manifest["implementation"].(map[string]any)
	if _, exists := implementation["config"]; exists {
		t.Fatalf("public implementation leaked config: %#v", implementation)
	}
	if implementation["kind"] != market.ImplementationKindManagedStdio {
		t.Fatalf("implementation.kind = %#v, want managed_stdio", implementation["kind"])
	}
}

func TestDaemonAPIConnectorMarketInstallMapsUnsupportedImplementation(t *testing.T) {
	service := stubConnectorMarketService{
		installFn: func(_ context.Context, mutation market.ConnectorMutation) (market.MutationResult, error) {
			if mutation.ConnectorKey != "notion" || mutation.ClientRequestID != "request-1" || mutation.ExpectedRevision != 7 || len(mutation.PrincipalIDs) != 1 || mutation.PrincipalIDs[0] != "principal-1" {
				t.Fatalf("mutation = %#v", mutation)
			}
			return market.MutationResult{}, market.NewDomainError(
				market.ErrorCodeUnsupportedImplementation,
				"connector implementation is not registered",
				false,
				nil,
			)
		},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewRoutes(DaemonAPI{ConnectorMarketService: service}))

	recorder := performGeneratedRouteRequest(t, mux, http.MethodPost, "/v1/connector-market/connectors/notion:install", map[string]any{
		"clientRequestId":  "request-1",
		"expectedRevision": 7,
		"principalIds":     []string{"principal-1"},
	})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
	}
	var response tuttigenerated.ConnectorMarketError
	decodeGeneratedRouteResponse(t, recorder, &response)
	if response.Code != tuttigenerated.ConnectorImplementationUnsupported || response.Retryable {
		t.Fatalf("response = %#v", response)
	}
}

func TestDaemonAPIConnectorMarketRefreshRejectsNegativeRevision(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewRoutes(DaemonAPI{ConnectorMarketService: stubConnectorMarketService{}}))

	recorder := performGeneratedRouteRequest(t, mux, http.MethodPost, "/v1/connector-market:refresh", map[string]any{
		"clientRequestId":  "request-1",
		"expectedRevision": -1,
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestDaemonAPIConnectorMarketListsAgentsAndReplacesGrants(t *testing.T) {
	service := stubConnectorMarketService{
		agentsFn: func(context.Context) ([]market.AgentPrincipal, error) {
			return []market.AgentPrincipal{{PrincipalID: "principal-1", Kind: "workspace_agent", Name: "Research Agent"}}, nil
		},
		grantsFn: func(_ context.Context, command market.SetAgentGrantsCommand) (market.AgentGrantSet, error) {
			if command.ConnectorKey != "notion" || command.ExpectedRevision != 2 || len(command.PrincipalIDs) != 1 {
				t.Fatalf("command = %#v", command)
			}
			return market.AgentGrantSet{ConnectorKey: "notion", PrincipalIDs: command.PrincipalIDs, Revision: 3}, nil
		},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewRoutes(DaemonAPI{ConnectorMarketService: service}))
	agents := performGeneratedRouteRequest(t, mux, http.MethodGet, "/v1/connector-market/agents", nil)
	if agents.Code != http.StatusOK {
		t.Fatalf("agents status = %d; body: %s", agents.Code, agents.Body.String())
	}
	grants := performGeneratedRouteRequest(t, mux, http.MethodPut, "/v1/connector-market/connectors/notion/agent-grants", map[string]any{
		"clientRequestId": "request-1", "expectedRevision": 2, "principalIds": []string{"principal-1"},
	})
	if grants.Code != http.StatusOK {
		t.Fatalf("grants status = %d; body: %s", grants.Code, grants.Body.String())
	}
}

func TestDaemonAPIConnectorMarketServesCategoriesAndCursorPage(t *testing.T) {
	service := stubConnectorMarketService{
		categoriesFn: func(context.Context) ([]market.CatalogCategory, error) {
			return []market.CatalogCategory{{CategoryID: "development", Kind: "category", SortOrder: 20, ItemCount: 1}}, nil
		},
		pageFn: func(_ context.Context, query market.CatalogPageQuery) (market.CatalogPage, error) {
			if query.SectionID != "development" || query.PageSize != 20 || query.PageToken != "cursor-1" {
				t.Fatalf("query = %#v", query)
			}
			return market.CatalogPage{
				SectionID:     "development",
				Items:         []market.CatalogListing{{CategoryID: "development", Connector: connectorMarketTestConnector()}},
				NextPageToken: "cursor-2",
				Revision:      8,
			}, nil
		},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewRoutes(DaemonAPI{ConnectorMarketService: service}))

	categories := performGeneratedRouteRequest(t, mux, http.MethodGet, "/v1/connector-market/categories", nil)
	if categories.Code != http.StatusOK {
		t.Fatalf("categories status = %d; body: %s", categories.Code, categories.Body.String())
	}
	page := performGeneratedRouteRequest(t, mux, http.MethodGet, "/v1/connector-market/catalog?sectionId=development&pageSize=20&pageToken=cursor-1", nil)
	if page.Code != http.StatusOK {
		t.Fatalf("page status = %d; body: %s", page.Code, page.Body.String())
	}
	var response tuttigenerated.ConnectorMarketCatalogPage
	decodeGeneratedRouteResponse(t, page, &response)
	if response.SectionId != "development" || response.Revision != 8 || len(response.Items) != 1 || response.Items[0].Connector.Key != "notion" {
		t.Fatalf("response = %#v", response)
	}
}

func connectorMarketTestConnector() market.Connector {
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return market.Connector{
		Key: "notion",
		Release: market.Release{
			SchemaVersion:  "1",
			ReleaseID:      "notion@1.0.0",
			ConnectorKey:   "notion",
			Version:        "1.0.0",
			ReleaseDigest:  digest,
			ManifestDigest: digest,
			Manifest: market.Manifest{
				SchemaVersion: "1",
				DisplayName:   "Notion",
				Permissions:   []string{"pages.read"},
				Implementation: market.Implementation{
					Kind: market.ImplementationKindManagedStdio,
					ManagedStdio: &market.ManagedStdioImplementation{
						Runtime: market.RuntimeRequirement{Language: "node", Profile: "connector-node-static", ABI: "node20-darwin-arm64"},
						MCP:     &market.ManagedMCPInterface{Entrypoint: "bin/notion.js"}, CredentialBrokerProtocol: market.CredentialBrokerProtocolV1,
					},
				},
				AuthorizationKind: "oauth2",
			},
			Artifact: market.Artifact{
				Key:       "connectors/notion/1.0.0.tar.gz",
				SHA256:    digest,
				SizeBytes: 128,
				MediaType: "application/gzip",
			},
			PublishedAt: time.Date(2026, time.August, 3, 0, 0, 0, 0, time.UTC), Status: market.ReleaseStatusAvailable,
		},
		Installation:  market.Installation{State: market.InstallationStateNotInstalled},
		Authorization: market.Authorization{State: market.AuthorizationStateDisconnected},
		Compatibility: market.Compatibility{State: market.CompatibilityStateSupported},
		Revision:      7,
	}
}
