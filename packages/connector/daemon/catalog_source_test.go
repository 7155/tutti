package daemon

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	market "github.com/tutti-os/tutti/packages/connector/host"
)

type memoryCatalogTrustStore struct {
	mu     sync.Mutex
	states map[string]market.CatalogTrustState
}

func (store *memoryCatalogTrustStore) LoadCatalogTrustState(_ context.Context, marketType string) (market.CatalogTrustState, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	state, exists := store.states[marketType]
	return state, exists, nil
}

func (store *memoryCatalogTrustStore) SaveCatalogTrustState(_ context.Context, marketType string, state market.CatalogTrustState) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.states == nil {
		store.states = make(map[string]market.CatalogTrustState)
	}
	store.states[marketType] = state
	return nil
}

func TestCatalogSourceMapsPublishedConnectorItems(t *testing.T) {
	authoritativeCatalog, releaseDigest, publicKey := testAuthoritativeCatalog(t)
	releaseCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer catalog-token" {
			t.Fatalf("request path=%q query=%q authorization=%q", request.URL.Path, request.URL.RawQuery, request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == connectorCatalogPath {
			releaseCalls++
			_, _ = writer.Write(authoritativeCatalog)
			return
		}
		if request.URL.Query().Get("itemType") != "connector" {
			t.Fatalf("request path=%q query=%q", request.URL.Path, request.URL.RawQuery)
		}
		if request.URL.Path == connectorCategoriesPath {
			_, _ = writer.Write([]byte(`{
  "marketType": "overseas",
  "categories": [
    {"categoryId": "featured", "kind": "featured", "sortOrder": 10, "itemCount": "1"},
    {"categoryId": "development", "kind": "category", "sortOrder": 20, "itemCount": "1"}
  ]
}`))
			return
		}
		if request.URL.Path != connectorPlacementPath || request.URL.Query().Get("sectionId") != "development" || request.URL.Query().Get("pageSize") != "100" {
			t.Fatalf("request path=%q query=%q", request.URL.Path, request.URL.RawQuery)
		}
		_, _ = writer.Write([]byte(`{
  "marketType": "overseas",
  "items": [{
    "itemType": "connector",
    "itemKey": "github",
    "version": "1.0.0",
    "commitSha": "0123456789abcdef",
    "artifact": {
      "key": "connectors/github/1.0.0.zip",
      "sha256": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "sizeBytes": "123"
    },
    "manifest": {
      "schemaVersion": "1",
      "itemType": "connector",
      "itemKey": "github",
      "version": "1.0.0",
      "display": {"name": "GitHub", "description": "GitHub connector", "iconUrl": "data:image/png;base64,iVBORw0KGgo="},
      "supportedMarkets": ["overseas"],
      "payload": {
        "permissions": ["repository.read", "network:*"],
        "packageManifestSha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
        "authorization": {"kind": "none"},
        "compatibility": {},
        "implementations": {
          "overseas": {
            "kind": "managed_stdio",
            "managedStdio": {
              "runtime": {"language": "node", "profile": "connector-node-static", "abi": "node20-darwin-arm64"},
              "mcp": {"entrypoint": "bin/github.js"}
            }
          }
        }
      }
    },
    "publishedAtMs": "1785801600000",
    "categoryId": "development",
    "featured": true
  }],
  "nextPageToken": ""
}`))
	}))
	defer server.Close()

	source, err := NewCatalogSource(CatalogSourceConfig{
		BaseURL:            server.URL,
		ExpectedMarketType: "overseas",
		HTTPClient:         server.Client(),
		AuthorizeRequest: func(request *http.Request) error {
			request.Header.Set("Authorization", "Bearer catalog-token")
			return nil
		},
		TrustedSigningKeys:           map[string]ed25519.PublicKey{"key-1": publicKey},
		TrustedSigningKeyringVersion: 1,
		TrustStateStore:              &memoryCatalogTrustStore{},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := source.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Releases) != 1 || result.SourceRevision == "" {
		t.Fatalf("snapshot = %#v", result)
	}
	got := result.Releases[0]
	if got.ConnectorKey != "github" || got.ReleaseID != "release-42" || got.ReleaseDigest != releaseDigest || got.Manifest.SchemaVersion != "1" || got.ManifestDigest != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" || got.Artifact.SizeBytes != 123 || got.Artifact.MediaType != "application/vnd.tutti.connector+zip" || got.Manifest.Implementation.ManagedStdio == nil || len(got.Manifest.Permissions) != 2 || got.Manifest.Permissions[1] != "network:*" {
		t.Fatalf("release = %#v", got)
	}
	page, err := source.ListPage(context.Background(), market.CatalogSourcePageQuery{SectionID: "development", PageSize: 100})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].ConnectorKey != "github" ||
		page.Entries[0].Version != "1.0.0" || page.Entries[0].ArtifactSHA256 != strings.Repeat("c", 64) {
		t.Fatalf("page=%#v error=%v", page, err)
	}
	if _, err := source.ListPage(context.Background(), market.CatalogSourcePageQuery{SectionID: "development", PageSize: 100}); err != nil {
		t.Fatal(err)
	}
	if releaseCalls != 1 {
		t.Fatalf("authoritative release requests = %d, want 1", releaseCalls)
	}
}

func testAuthoritativeCatalog(t *testing.T) ([]byte, string, ed25519.PublicKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := json.Marshal(wireConnectorMarketManifest{
		SchemaVersion: "1", ItemType: "connector", ItemKey: "github", Version: "1.0.0",
		Display:          wireConnectorDisplay{Name: "GitHub", Description: "GitHub connector", IconURL: "data:image/png;base64,iVBORw0KGgo="},
		SupportedMarkets: []string{"overseas"},
		Payload: wireConnectorManifestPayload{Permissions: []string{"repository.read", "network:*"},
			PackageManifestSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Authorization:         wireConnectorAuthorization{Kind: "none"}, Compatibility: market.CompatibilityRequirements{},
			Implementations: map[string]market.Implementation{"overseas": {Kind: market.ImplementationKindManagedStdio,
				ManagedStdio: &market.ManagedStdioImplementation{Runtime: market.RuntimeRequirement{Language: "node", Profile: "connector-node-static", ABI: "node20-darwin-arm64"}, MCP: &market.ManagedMCPInterface{Entrypoint: "bin/github.js"}}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestHash := sha256.Sum256(manifestBytes)
	manifestDigest := hex.EncodeToString(manifestHash[:])
	envelopeBytes, err := json.Marshal(wireReleaseEnvelopePayload{
		SchemaVersion: "1", ItemType: "connector", ItemKey: "github", Version: "1.0.0",
		PublisherSubject: "ci", SourceRepository: "tutti/connectors", CommitSHA: "0123456789abcdef", WorkflowRef: "publish.yml",
		ProvenanceDigest: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		ArtifactKey:      "connectors/github/1.0.0.zip", ArtifactStorageRealm: connectorArtifactRealm, ArtifactObjectVersion: "version-1",
		ArtifactSHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", ArtifactSizeBytes: 123,
		ArtifactMediaType: "application/vnd.tutti.connector+zip", ManifestSHA256: manifestDigest, TrustTier: "verified",
		Permissions: []string{"repository.read", "network:*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	releaseHash := sha256.Sum256(envelopeBytes)
	releaseDigest := hex.EncodeToString(releaseHash[:])
	releaseDocument := signedTestDocument(t, releaseSignatureContext, envelopeBytes, privateKey)
	now := time.Now().UTC()
	snapshotBytes, err := json.Marshal(signedCatalogPayload{
		Sequence: 1, IssuedAt: now.Add(-time.Minute), NextUpdateAt: now.Add(5 * time.Minute), ExpiresAt: now.Add(10 * time.Minute),
		Catalog: signedCatalogIndex{SchemaVersion: "1", Releases: []signedCatalogReleaseStatus{{
			ConnectorKey: "github", ReleaseDigest: releaseDigest, Version: "1.0.0", Status: "available",
			PublishedMarkets: []string{"overseas"}, ManifestSHA256: manifestDigest,
			ArtifactSHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", ArtifactObjectVersion: "version-1",
			EnvelopeSHA256: releaseDigest, SignatureKeyID: releaseDocument.KeyID, Signature: releaseDocument.Signature,
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	wire := wireConnectorCatalogResponse{MarketType: "overseas", Releases: []wireConnectorRelease{{
		ConnectorKey: "github", ReleaseDigest: releaseDigest,
		SignedEnvelope: releaseDocument,
		Version:        "1.0.0", Manifest: wireCanonicalDocument{CanonicalBytes: string(manifestBytes), SHA256: manifestDigest},
		Artifact:      &wireConnectorArtifactProjection{ObjectVersion: "version-1", SHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", SizeBytes: 123, MediaType: "application/vnd.tutti.connector+zip"},
		PublishedAtMS: 1785801600000, ReleaseID: "release-42",
	}}}
	wire.Snapshot.SignedSnapshot = signedTestDocument(t, catalogSignatureContext, snapshotBytes, privateKey)
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return encoded, releaseDigest, publicKey
}

func signedTestDocument(t *testing.T, domain string, payload []byte, privateKey ed25519.PrivateKey) wireSignedDocument {
	t.Helper()
	payloadDigest := sha256.Sum256(payload)
	message := append([]byte(domain), payload...)
	messageDigest := sha256.Sum256(message)
	return wireSignedDocument{CanonicalBytes: string(payload), SHA256: hex.EncodeToString(payloadDigest[:]), KeyID: "key-1",
		Algorithm: connectorSignatureAlgorithm, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, messageDigest[:]))}
}

func TestCatalogSourceRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewCatalogSource(CatalogSourceConfig{BaseURL: "/market", ExpectedMarketType: "overseas"}); err == nil {
		t.Fatal("expected invalid URL")
	}
	if _, err := NewCatalogSource(CatalogSourceConfig{BaseURL: "https://example.test", ExpectedMarketType: "invalid"}); err == nil {
		t.Fatal("expected invalid market type")
	}
	if _, err := NewCatalogSource(CatalogSourceConfig{BaseURL: "https://example.test", ExpectedMarketType: "overseas"}); err == nil || !strings.Contains(err.Error(), "HTTP client") {
		t.Fatalf("expected missing HTTP client error, got %v", err)
	}
}

func TestCatalogSourceRejectsArtifactProjectionThatDiffersFromSignedEnvelope(t *testing.T) {
	payload, _, publicKey := testAuthoritativeCatalog(t)
	var catalog wireConnectorCatalogResponse
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatal(err)
	}
	catalog.Releases[0].Artifact.MediaType = "application/octet-stream"
	tampered, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(tampered)
	}))
	defer server.Close()
	source, err := NewCatalogSource(CatalogSourceConfig{BaseURL: server.URL, ExpectedMarketType: "overseas", HTTPClient: server.Client(),
		TrustedSigningKeys: map[string]ed25519.PublicKey{"key-1": publicKey}, TrustedSigningKeyringVersion: 1, TrustStateStore: &memoryCatalogTrustStore{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "signed catalog and release") {
		t.Fatalf("error = %v, want authoritative projection mismatch", err)
	}
}

func TestCatalogSourceRejectsForgedSignature(t *testing.T) {
	payload, _, publicKey := testAuthoritativeCatalog(t)
	var catalog wireConnectorCatalogResponse
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatal(err)
	}
	catalog.Releases[0].SignedEnvelope.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	forged, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { _, _ = writer.Write(forged) }))
	defer server.Close()
	source, err := NewCatalogSource(CatalogSourceConfig{BaseURL: server.URL, ExpectedMarketType: "overseas", HTTPClient: server.Client(),
		TrustedSigningKeys: map[string]ed25519.PublicKey{"key-1": publicKey}, TrustedSigningKeyringVersion: 1, TrustStateStore: &memoryCatalogTrustStore{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("error = %v, want forged signature rejection", err)
	}
}

func TestCatalogSourceRejectsUnknownSigningKey(t *testing.T) {
	payload, _, publicKey := testAuthoritativeCatalog(t)
	var catalog wireConnectorCatalogResponse
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatal(err)
	}
	catalog.Snapshot.SignedSnapshot.KeyID = "unknown-key"
	untrusted, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { _, _ = writer.Write(untrusted) }))
	defer server.Close()
	source, err := NewCatalogSource(CatalogSourceConfig{BaseURL: server.URL, ExpectedMarketType: "overseas", HTTPClient: server.Client(),
		TrustedSigningKeys: map[string]ed25519.PublicKey{"key-1": publicKey}, TrustedSigningKeyringVersion: 1, TrustStateStore: &memoryCatalogTrustStore{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Fatalf("error = %v, want unknown key rejection", err)
	}
}

func TestCatalogSourceRejectsReleaseWithheldFromSignedSnapshot(t *testing.T) {
	payload, _, publicKey := testAuthoritativeCatalog(t)
	var catalog wireConnectorCatalogResponse
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatal(err)
	}
	catalog.Releases = nil
	withheld, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { _, _ = writer.Write(withheld) }))
	defer server.Close()
	source, err := NewCatalogSource(CatalogSourceConfig{BaseURL: server.URL, ExpectedMarketType: "overseas", HTTPClient: server.Client(),
		TrustedSigningKeys: map[string]ed25519.PublicKey{"key-1": publicKey}, TrustedSigningKeyringVersion: 1, TrustStateStore: &memoryCatalogTrustStore{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "withheld") {
		t.Fatalf("error = %v, want signed snapshot binding rejection", err)
	}
}

func TestCatalogSourceRejectsRollbackFromDurableTrustState(t *testing.T) {
	payload, _, publicKey := testAuthoritativeCatalog(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { _, _ = writer.Write(payload) }))
	defer server.Close()
	trustStore := &memoryCatalogTrustStore{states: map[string]market.CatalogTrustState{
		"overseas": {KeyringVersion: 1, Sequence: 2, EnvelopeDigest: strings.Repeat("f", 64), WallHighWater: time.Now().UTC()},
	}}
	source, err := NewCatalogSource(CatalogSourceConfig{BaseURL: server.URL, ExpectedMarketType: "overseas", HTTPClient: server.Client(),
		TrustedSigningKeys: map[string]ed25519.PublicKey{"key-1": publicKey}, TrustedSigningKeyringVersion: 1, TrustStateStore: trustStore})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("error = %v, want durable rollback rejection", err)
	}
}

func TestCatalogSourcePreservesGatewayBasePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/desktop/v1/market/categories" {
			t.Fatalf("request path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"marketType":"overseas","categories":[]}`))
	}))
	defer server.Close()

	source, err := NewCatalogSource(CatalogSourceConfig{
		BaseURL:            server.URL + "/api/desktop",
		ExpectedMarketType: "overseas",
		HTTPClient:         server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ListCategories(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogSourceRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(strings.Repeat(" ", maxCatalogResponseBytes+1)))
	}))
	defer server.Close()
	source, err := NewCatalogSource(CatalogSourceConfig{
		BaseURL:            server.URL,
		ExpectedMarketType: "overseas",
		HTTPClient:         server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Refresh(context.Background()); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("error = %v", err)
	}
}
