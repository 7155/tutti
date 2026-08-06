package daemon

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	market "github.com/tutti-os/tutti/packages/connector/host"
)

const connectorCatalogPath = "/v1/connector-market/releases"
const connectorPlacementPath = "/v1/market/items"
const connectorCategoriesPath = "/v1/market/categories"
const maxCatalogResponseBytes = 8 << 20

type RequestAuthorizer func(*http.Request) error

type CatalogSourceConfig struct {
	BaseURL            string
	ExpectedMarketType string
	HTTPClient         *http.Client
	AuthorizeRequest   RequestAuthorizer
	// TrustedSigningKeys are pinned Ed25519 connector-market roots keyed by
	// keyId. An empty keyring keeps the local daemon usable but fails every
	// remote catalog acceptance closed.
	TrustedSigningKeys           map[string]ed25519.PublicKey
	TrustedSigningKeyringVersion uint64
	TrustStateStore              market.CatalogTrustStateReader
}

type CatalogSource struct {
	baseURL            *url.URL
	expectedMarketType string
	httpClient         *http.Client
	authorizeRequest   RequestAuthorizer
	trustVerifier      *catalogTrustVerifier
	trustStateStore    market.CatalogTrustStateReader
	trustMu            sync.Mutex
}

var _ market.CatalogSource = (*CatalogSource)(nil)

func NewCatalogSource(config CatalogSourceConfig) (*CatalogSource, error) {
	baseURL, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, errors.New("connector market base URL must be an absolute URL")
	}
	if baseURL.Scheme != "https" && (baseURL.Scheme != "http" || !isLoopbackHost(baseURL.Hostname())) {
		return nil, errors.New("connector market base URL must use https (http is allowed only for loopback tests)")
	}
	expectedMarketType := strings.ToLower(strings.TrimSpace(config.ExpectedMarketType))
	if expectedMarketType != "domestic" && expectedMarketType != "overseas" {
		return nil, errors.New("connector market type must be domestic or overseas")
	}
	client := config.HTTPClient
	if client == nil {
		return nil, errors.New("connector market HTTP client is required")
	}
	verifier, err := newCatalogTrustVerifier(config.TrustedSigningKeyringVersion, config.TrustedSigningKeys)
	if err != nil {
		return nil, err
	}
	if verifier != nil && config.TrustStateStore == nil {
		return nil, errors.New("connector market durable trust state store is required")
	}
	return &CatalogSource{baseURL: baseURL, expectedMarketType: expectedMarketType,
		httpClient: client, authorizeRequest: config.AuthorizeRequest, trustVerifier: verifier,
		trustStateStore: config.TrustStateStore}, nil
}

func (source *CatalogSource) Refresh(ctx context.Context) (market.CatalogSnapshot, error) {
	payload, trustState, err := source.listReleases(ctx)
	if err != nil {
		return market.CatalogSnapshot{}, err
	}
	releases := make([]market.Release, 0, len(payload.Releases))
	seen := make(map[string]struct{}, len(payload.Releases))
	for _, item := range payload.Releases {
		release, mapErr := source.mapRelease(item)
		if mapErr != nil {
			return market.CatalogSnapshot{}, mapErr
		}
		if _, duplicate := seen[release.ConnectorKey]; duplicate {
			return market.CatalogSnapshot{}, errors.New("connector market catalog contains duplicate active connector releases")
		}
		seen[release.ConnectorKey] = struct{}{}
		releases = append(releases, release)
	}
	return market.CatalogSnapshot{SourceRevision: payload.Snapshot.SignedSnapshot.SHA256, Releases: releases,
		MarketType: payload.MarketType, TrustState: &trustState}, nil
}

func (source *CatalogSource) ListCategories(ctx context.Context) ([]market.CatalogCategory, error) {
	var payload wireMarketCategoriesResponse
	if _, err := source.getJSON(ctx, connectorCategoriesPath, url.Values{"itemType": {"connector"}}, &payload); err != nil {
		return nil, err
	}
	if payload.MarketType != source.expectedMarketType {
		return nil, errors.New("connector market type does not match configured market")
	}
	categories := make([]market.CatalogCategory, 0, len(payload.Categories))
	seen := make(map[string]struct{}, len(payload.Categories))
	for _, category := range payload.Categories {
		if strings.TrimSpace(category.CategoryID) == "" || (category.Kind != "category" && category.Kind != "featured") || category.ItemCount < 0 {
			return nil, errors.New("connector market category is invalid")
		}
		if _, exists := seen[category.CategoryID]; exists {
			return nil, errors.New("connector market category is duplicated")
		}
		seen[category.CategoryID] = struct{}{}
		categories = append(categories, market.CatalogCategory{CategoryID: category.CategoryID, Kind: category.Kind, SortOrder: category.SortOrder, ItemCount: int64(category.ItemCount)})
	}
	return categories, nil
}

func (source *CatalogSource) ListPage(ctx context.Context, input market.CatalogSourcePageQuery) (market.CatalogSourcePage, error) {
	query := url.Values{
		"itemType":  {"connector"},
		"sectionId": {strings.TrimSpace(input.SectionID)},
		"pageSize":  {strconv.Itoa(input.PageSize)},
	}
	if token := strings.TrimSpace(input.PageToken); token != "" {
		query.Set("pageToken", token)
	}
	var payload wireMarketResponse
	if _, err := source.getJSON(ctx, connectorPlacementPath, query, &payload); err != nil {
		return market.CatalogSourcePage{}, err
	}
	if payload.MarketType != source.expectedMarketType {
		return market.CatalogSourcePage{}, errors.New("connector market type does not match configured market")
	}
	entries := make([]market.CatalogEntry, 0, len(payload.Items))
	for _, item := range payload.Items {
		if strings.TrimSpace(item.CategoryID) == "" {
			return market.CatalogSourcePage{}, errors.New("connector market item category is missing")
		}
		if item.ItemType != "connector" || item.ItemKey == "" || item.Version == "" || item.Artifact == nil ||
			!isSHA256Hex(item.Artifact.SHA256) || item.Artifact.SizeBytes <= 0 {
			return market.CatalogSourcePage{}, errors.New("connector market placement identity is incomplete")
		}
		entries = append(entries, market.CatalogEntry{CategoryID: item.CategoryID, Featured: item.Featured, ConnectorKey: item.ItemKey,
			Version: item.Version, ArtifactSHA256: item.Artifact.SHA256, ArtifactSizeBytes: int64(item.Artifact.SizeBytes)})
	}
	return market.CatalogSourcePage{SectionID: strings.TrimSpace(input.SectionID), Entries: entries, NextPageToken: payload.NextPageToken}, nil
}

func (source *CatalogSource) listReleases(ctx context.Context) (wireConnectorCatalogResponse, market.CatalogTrustState, error) {
	var payload wireConnectorCatalogResponse
	if _, err := source.getJSON(ctx, connectorCatalogPath, nil, &payload); err != nil {
		return wireConnectorCatalogResponse{}, market.CatalogTrustState{}, err
	}
	if payload.MarketType != source.expectedMarketType {
		return wireConnectorCatalogResponse{}, market.CatalogTrustState{}, errors.New("connector market type does not match configured market")
	}
	trustState, err := source.verifyProjection(ctx, payload)
	if err != nil {
		return wireConnectorCatalogResponse{}, market.CatalogTrustState{}, fmt.Errorf("verify connector catalog trust: %w", err)
	}
	return payload, trustState, nil
}

func (source *CatalogSource) getJSON(ctx context.Context, requestPath string, query url.Values, target any) ([]byte, error) {
	joined, err := url.JoinPath(source.baseURL.String(), requestPath)
	if err != nil {
		return nil, fmt.Errorf("build connector market catalog URL: %w", err)
	}
	endpoint, err := url.Parse(joined)
	if err != nil {
		return nil, fmt.Errorf("parse connector market catalog URL: %w", err)
	}
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if source.authorizeRequest != nil {
		if err := source.authorizeRequest(request); err != nil {
			return nil, err
		}
	}
	response, err := source.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request connector market catalog: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return nil, fmt.Errorf("request connector market catalog: status %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	payloadBytes, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payloadBytes) > maxCatalogResponseBytes {
		return nil, errors.New("decode connector market catalog: response exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(payloadBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return nil, fmt.Errorf("decode connector market catalog: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode connector market catalog: trailing JSON value")
	}
	return payloadBytes, nil
}

func (source *CatalogSource) mapRelease(item wireConnectorRelease) (market.Release, error) {
	if item.ConnectorKey == "" || item.Version == "" || item.ReleaseID == "" || item.Artifact == nil {
		return market.Release{}, errors.New("connector release identity is incomplete")
	}
	if err := validateSignedDocumentDigest(item.SignedEnvelope); err != nil || item.SignedEnvelope.SHA256 != item.ReleaseDigest {
		return market.Release{}, errors.New("connector release signed envelope digest is invalid")
	}
	manifestHash := sha256.Sum256([]byte(item.Manifest.CanonicalBytes))
	manifestEnvelopeDigest := hex.EncodeToString(manifestHash[:])
	if item.Manifest.SHA256 != manifestEnvelopeDigest {
		return market.Release{}, errors.New("connector release canonical manifest digest is invalid")
	}
	var signedPayload wireReleaseEnvelopePayload
	if err := decodeStrictJSON([]byte(item.SignedEnvelope.CanonicalBytes), &signedPayload); err != nil {
		return market.Release{}, fmt.Errorf("decode connector release envelope: %w", err)
	}
	if signedPayload.SchemaVersion != "1" || signedPayload.ItemType != "connector" ||
		signedPayload.ItemKey != item.ConnectorKey || signedPayload.Version != item.Version ||
		signedPayload.ManifestSHA256 != item.Manifest.SHA256 || !safeArtifactKey(signedPayload.ArtifactKey) ||
		item.Artifact.ObjectVersion == "" || item.Artifact.ObjectVersion != signedPayload.ArtifactObjectVersion ||
		item.Artifact.SHA256 != signedPayload.ArtifactSHA256 || int64(item.Artifact.SizeBytes) != signedPayload.ArtifactSizeBytes ||
		item.Artifact.MediaType != signedPayload.ArtifactMediaType {
		return market.Release{}, errors.New("connector release projection does not match the signed envelope")
	}
	var connectorManifest wireConnectorMarketManifest
	if err := decodeStrictJSON([]byte(item.Manifest.CanonicalBytes), &connectorManifest); err != nil {
		return market.Release{}, fmt.Errorf("decode connector market manifest: %w", err)
	}
	if connectorManifest.SchemaVersion != "1" || connectorManifest.ItemType != "connector" ||
		connectorManifest.ItemKey != item.ConnectorKey || connectorManifest.Version != item.Version ||
		!containsString(connectorManifest.SupportedMarkets, source.expectedMarketType) {
		return market.Release{}, errors.New("connector manifest identity or market does not match release")
	}
	if !isSHA256Hex(connectorManifest.Payload.PackageManifestSHA256) {
		return market.Release{}, errors.New("connector manifest package digest is invalid")
	}
	implementation, ok := connectorManifest.Payload.Implementations[source.expectedMarketType]
	if !ok {
		return market.Release{}, errors.New("connector manifest does not provide the configured market implementation")
	}
	if !reflect.DeepEqual(connectorManifest.Payload.Permissions, signedPayload.Permissions) {
		return market.Release{}, errors.New("connector manifest permissions do not match the signed envelope")
	}
	iconURL := connectorManifest.Display.IconURL
	if connectorManifest.SchemaVersion == "1" && strings.TrimSpace(iconURL) == "" {
		iconURL = legacyConnectorIconURL
	}
	manifest := market.Manifest{SchemaVersion: connectorManifest.SchemaVersion, DisplayName: connectorManifest.Display.Name, IconURL: iconURL,
		Description: connectorManifest.Display.Description, Permissions: connectorManifest.Payload.Permissions,
		Implementation: implementation, AuthorizationKind: connectorManifest.Payload.Authorization.Kind,
		Compatibility: connectorManifest.Payload.Compatibility}
	release := market.Release{SchemaVersion: "1", ReleaseID: item.ReleaseID,
		ConnectorKey: item.ConnectorKey, Version: item.Version,
		ReleaseDigest: item.ReleaseDigest, ManifestDigest: connectorManifest.Payload.PackageManifestSHA256,
		Manifest: manifest, Artifact: market.Artifact{StorageRealm: signedPayload.ArtifactStorageRealm, Key: signedPayload.ArtifactKey,
			ObjectVersion: signedPayload.ArtifactObjectVersion, SHA256: signedPayload.ArtifactSHA256,
			SizeBytes: signedPayload.ArtifactSizeBytes, MediaType: signedPayload.ArtifactMediaType},
		PublishedAt: time.UnixMilli(int64(item.PublishedAtMS)).UTC(), Status: market.ReleaseStatusAvailable}
	if err := market.ValidateReleaseShape(release); err != nil {
		return market.Release{}, err
	}
	return release, nil
}

func validateSignedDocumentDigest(document wireSignedDocument) error {
	digest := sha256.Sum256([]byte(document.CanonicalBytes))
	if !isSHA256Hex(document.SHA256) || hex.EncodeToString(digest[:]) != document.SHA256 ||
		strings.TrimSpace(document.KeyID) == "" || strings.TrimSpace(document.Algorithm) == "" || strings.TrimSpace(document.Signature) == "" {
		return errors.New("signed document evidence is incomplete")
	}
	return nil
}

func decodeStrictJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON value")
	}
	return nil
}

type wireMarketResponse struct {
	MarketType    string           `json:"marketType"`
	Items         []wireMarketItem `json:"items"`
	NextPageToken string           `json:"nextPageToken"`
}

type wireConnectorCatalogResponse struct {
	MarketType string `json:"marketType"`
	Snapshot   struct {
		SignedSnapshot wireSignedDocument `json:"signedSnapshot"`
	} `json:"snapshot"`
	Releases []wireConnectorRelease `json:"releases"`
}

type wireSignedDocument struct {
	CanonicalBytes string `json:"canonicalBytes"`
	SHA256         string `json:"sha256"`
	KeyID          string `json:"keyId"`
	Algorithm      string `json:"algorithm"`
	Signature      string `json:"signature"`
}

type wireCanonicalDocument struct {
	CanonicalBytes string `json:"canonicalBytes"`
	SHA256         string `json:"sha256"`
}

type wireConnectorArtifactProjection struct {
	ObjectVersion string    `json:"objectVersion"`
	SHA256        string    `json:"sha256"`
	SizeBytes     wireInt64 `json:"sizeBytes"`
	MediaType     string    `json:"mediaType"`
}

type wireConnectorRelease struct {
	ConnectorKey   string                           `json:"connectorKey"`
	ReleaseDigest  string                           `json:"releaseDigest"`
	SignedEnvelope wireSignedDocument               `json:"signedEnvelope"`
	Version        string                           `json:"version"`
	Manifest       wireCanonicalDocument            `json:"manifest"`
	Artifact       *wireConnectorArtifactProjection `json:"artifact"`
	PublishedAtMS  wireInt64                        `json:"publishedAtMs"`
	ReleaseID      string                           `json:"releaseId"`
}

type wireReleaseEnvelopePayload struct {
	SchemaVersion         string   `json:"schemaVersion"`
	ItemType              string   `json:"itemType"`
	ItemKey               string   `json:"itemKey"`
	Version               string   `json:"version"`
	PublisherSubject      string   `json:"publisherSubject"`
	SourceRepository      string   `json:"sourceRepository"`
	CommitSHA             string   `json:"commitSha"`
	WorkflowRef           string   `json:"workflowRef"`
	ProvenanceDigest      string   `json:"provenanceDigest"`
	ArtifactKey           string   `json:"artifactKey"`
	ArtifactStorageRealm  string   `json:"artifactStorageRealm"`
	ArtifactObjectVersion string   `json:"artifactObjectVersion"`
	ArtifactSHA256        string   `json:"artifactSha256"`
	ArtifactSizeBytes     int64    `json:"artifactSizeBytes"`
	ArtifactMediaType     string   `json:"artifactMediaType"`
	ManifestSHA256        string   `json:"manifestSha256"`
	TrustTier             string   `json:"trustTier"`
	Permissions           []string `json:"permissions"`
}

type wireMarketCategoriesResponse struct {
	MarketType string               `json:"marketType"`
	Categories []wireMarketCategory `json:"categories"`
}

type wireMarketCategory struct {
	CategoryID string    `json:"categoryId"`
	Kind       string    `json:"kind"`
	SortOrder  int32     `json:"sortOrder"`
	ItemCount  wireInt64 `json:"itemCount"`
}

type wireMarketItem struct {
	ItemType      string         `json:"itemType"`
	ItemKey       string         `json:"itemKey"`
	Version       string         `json:"version"`
	CommitSHA     string         `json:"commitSha"`
	Artifact      *wireArtifact  `json:"artifact"`
	Manifest      map[string]any `json:"manifest"`
	PublishedAtMS wireInt64      `json:"publishedAtMs"`
	CategoryID    string         `json:"categoryId"`
	Featured      bool           `json:"featured"`
}

type wireArtifact struct {
	Key       string    `json:"key"`
	SHA256    string    `json:"sha256"`
	SizeBytes wireInt64 `json:"sizeBytes"`
}

// Kratos/protojson encodes int64 fields as JSON strings. Accepting numeric
// literals too keeps local tests and non-protobuf adapters straightforward.
type wireInt64 int64

func (value *wireInt64) UnmarshalJSON(payload []byte) error {
	text := strings.TrimSpace(string(payload))
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		text = text[1 : len(text)-1]
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("decode market int64: %w", err)
	}
	*value = wireInt64(parsed)
	return nil
}

type wireConnectorMarketManifest struct {
	SchemaVersion    string                       `json:"schemaVersion"`
	ItemType         string                       `json:"itemType"`
	ItemKey          string                       `json:"itemKey"`
	Version          string                       `json:"version"`
	Display          wireConnectorDisplay         `json:"display"`
	SupportedMarkets []string                     `json:"supportedMarkets"`
	Payload          wireConnectorManifestPayload `json:"payload"`
}

type wireConnectorDisplay struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IconURL     string `json:"iconUrl"`
}

type wireConnectorManifestPayload struct {
	Permissions           []string                         `json:"permissions"`
	PackageManifestSHA256 string                           `json:"packageManifestSha256"`
	Authorization         wireConnectorAuthorization       `json:"authorization"`
	Compatibility         market.CompatibilityRequirements `json:"compatibility"`
	Implementations       map[string]market.Implementation `json:"implementations"`
}

type wireConnectorAuthorization struct {
	Kind string `json:"kind"`
}

const legacyConnectorIconURL = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCA2NCA2NCI+PHJlY3Qgd2lkdGg9IjY0IiBoZWlnaHQ9IjY0IiByeD0iMTQiIGZpbGw9IiM2YjcyODAiLz48cGF0aCBkPSJNMTggMjBoMjh2MjRIMTh6IiBmaWxsPSJub25lIiBzdHJva2U9IndoaXRlIiBzdHJva2Utd2lkdGg9IjQiLz48L3N2Zz4="

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func safeArtifactKey(key string) bool {
	cleaned := path.Clean(strings.TrimSpace(key))
	return cleaned != "." && cleaned != ".." && cleaned == key && !path.IsAbs(cleaned) && !strings.HasPrefix(cleaned, "../") && !strings.Contains(cleaned, "\\")
}

func isSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
