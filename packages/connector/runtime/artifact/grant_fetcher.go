package artifact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const connectorArtifactGrantPath = "/v1/connector-market/artifact-grants"
const maxConnectorArtifactGrantResponseBytes = 1 << 20

type GrantRequestAuthorizer func(*http.Request) error
type WorkspaceIDProvider func(context.Context) (string, error)

// Protojson represents int64 fields as decimal strings. Numeric literals are
// also accepted for non-protobuf test servers and compatible adapters.
type grantInt64 int64

func (value *grantInt64) UnmarshalJSON(payload []byte) error {
	text := strings.TrimSpace(string(payload))
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		text = text[1 : len(text)-1]
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("decode connector artifact grant int64: %w", err)
	}
	*value = grantInt64(parsed)
	return nil
}

type GrantFetcherConfig struct {
	BaseURL             string
	HTTPClient          *http.Client
	AuthorizeRequest    GrantRequestAuthorizer
	WorkspaceIDProvider WorkspaceIDProvider
	Now                 func() time.Time
}

// GrantFetcher exchanges immutable signed release identity for a short-lived
// download URL. The artifact object key is never used as an authorization or
// addressing input.
type GrantFetcher struct {
	baseURL             *url.URL
	httpClient          *http.Client
	authorizeRequest    GrantRequestAuthorizer
	workspaceIDProvider WorkspaceIDProvider
	now                 func() time.Time
}

var _ Fetcher = (*GrantFetcher)(nil)

func NewGrantFetcher(config GrantFetcherConfig) (*GrantFetcher, error) {
	baseURL, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("connector artifact grant base URL is invalid")
	}
	if baseURL.Scheme != "https" && (baseURL.Scheme != "http" || !isLoopbackHost(baseURL.Hostname())) {
		return nil, errors.New("connector artifact grant base URL must use https (http is allowed only for loopback tests)")
	}
	if config.HTTPClient == nil || config.AuthorizeRequest == nil || config.WorkspaceIDProvider == nil {
		return nil, errors.New("connector artifact grant dependencies are incomplete")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &GrantFetcher{baseURL: baseURL, httpClient: config.HTTPClient, authorizeRequest: config.AuthorizeRequest,
		workspaceIDProvider: config.WorkspaceIDProvider, now: config.Now}, nil
}

func (fetcher *GrantFetcher) Fetch(ctx context.Context, request FetchRequest) (FetchResponse, error) {
	workspaceID, err := fetcher.workspaceIDProvider(ctx)
	if err != nil || strings.TrimSpace(workspaceID) == "" {
		return FetchResponse{}, errors.New("connector artifact workspace authority is unavailable")
	}
	release := request.Release
	body, err := json.Marshal(map[string]string{
		"workspaceId": workspaceID, "connectorKey": release.ConnectorKey, "releaseDigest": release.ReleaseDigest,
		"artifactSha256": release.Artifact.SHA256, "objectVersion": release.Artifact.ObjectVersion,
	})
	if err != nil {
		return FetchResponse{}, err
	}
	endpoint, err := url.JoinPath(fetcher.baseURL.String(), connectorArtifactGrantPath)
	if err != nil {
		return FetchResponse{}, err
	}
	grantRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return FetchResponse{}, err
	}
	grantRequest.Header.Set("Accept", "application/json")
	grantRequest.Header.Set("Content-Type", "application/json")
	if err := fetcher.authorizeRequest(grantRequest); err != nil {
		return FetchResponse{}, err
	}
	grantClient := *fetcher.httpClient
	previousGrantRedirect := grantClient.CheckRedirect
	grantClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !sameOrigin(next.URL, grantRequest.URL) {
			return errors.New("connector artifact grant redirect leaves the trusted origin")
		}
		if previousGrantRedirect != nil {
			return previousGrantRedirect(next, via)
		}
		return nil
	}
	grantResponse, err := grantClient.Do(grantRequest)
	if err != nil {
		return FetchResponse{}, fmt.Errorf("grant connector artifact download: %w", err)
	}
	defer func() { _ = grantResponse.Body.Close() }()
	if grantResponse.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(grantResponse.Body, 4<<10))
		return FetchResponse{}, fmt.Errorf("grant connector artifact download: status %d: %s", grantResponse.StatusCode, strings.TrimSpace(string(message)))
	}
	var grant struct {
		DownloadURL   string     `json:"downloadUrl"`
		Method        string     `json:"method"`
		ExpiresAtMS   grantInt64 `json:"expiresAtMs"`
		ConnectorKey  string     `json:"connectorKey"`
		ReleaseDigest string     `json:"releaseDigest"`
		ArtifactSHA   string     `json:"artifactSha256"`
		SizeBytes     grantInt64 `json:"sizeBytes"`
		MediaType     string     `json:"mediaType"`
		ObjectVersion string     `json:"objectVersion"`
	}
	grantPayload, err := io.ReadAll(io.LimitReader(grantResponse.Body, maxConnectorArtifactGrantResponseBytes+1))
	if err != nil || len(grantPayload) > maxConnectorArtifactGrantResponseBytes {
		return FetchResponse{}, errors.New("connector artifact grant response is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(grantPayload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&grant); err != nil {
		return FetchResponse{}, errors.New("connector artifact grant response is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return FetchResponse{}, errors.New("connector artifact grant response contains trailing data")
	}
	now := fetcher.now().UTC()
	expiresAt := time.UnixMilli(int64(grant.ExpiresAtMS)).UTC()
	if grant.Method != http.MethodGet || grant.ConnectorKey != release.ConnectorKey || grant.ReleaseDigest != release.ReleaseDigest ||
		grant.ArtifactSHA != release.Artifact.SHA256 || int64(grant.SizeBytes) != release.Artifact.SizeBytes || grant.MediaType != release.Artifact.MediaType ||
		grant.ObjectVersion != release.Artifact.ObjectVersion || !expiresAt.After(now) || expiresAt.After(now.Add(5*time.Minute+30*time.Second)) {
		return FetchResponse{}, errors.New("connector artifact grant does not match the signed release")
	}
	downloadURL, err := url.Parse(strings.TrimSpace(grant.DownloadURL))
	if err != nil || downloadURL.Host == "" || downloadURL.User != nil ||
		(downloadURL.Scheme != "https" && (downloadURL.Scheme != "http" || !isLoopbackHost(downloadURL.Hostname()))) {
		return FetchResponse{}, errors.New("connector artifact grant download URL is invalid")
	}
	downloadRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL.String(), nil)
	if err != nil {
		return FetchResponse{}, err
	}
	downloadClient := *fetcher.httpClient
	// Account/session cookies authorize the grant exchange, never the object
	// store. The presigned URL is the sole download authority.
	downloadClient.Jar = nil
	previousRedirect := downloadClient.CheckRedirect
	downloadClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !sameOrigin(next.URL, downloadURL) {
			return errors.New("connector artifact redirect leaves the granted origin")
		}
		if previousRedirect != nil {
			return previousRedirect(next, via)
		}
		return nil
	}
	downloadResponse, err := downloadClient.Do(downloadRequest)
	if err != nil {
		return FetchResponse{}, fmt.Errorf("download granted connector artifact: %w", err)
	}
	if downloadResponse.StatusCode != http.StatusOK {
		defer func() { _ = downloadResponse.Body.Close() }()
		message, _ := io.ReadAll(io.LimitReader(downloadResponse.Body, 4<<10))
		return FetchResponse{}, fmt.Errorf("download granted connector artifact: status %d: %s", downloadResponse.StatusCode, strings.TrimSpace(string(message)))
	}
	return FetchResponse{Body: downloadResponse.Body, ContentLength: downloadResponse.ContentLength,
		MediaType: downloadResponse.Header.Get("Content-Type")}, nil
}
