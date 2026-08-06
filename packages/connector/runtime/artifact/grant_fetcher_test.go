package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGrantFetcherBindsWorkspaceAndImmutableReleaseBeforeDownload(t *testing.T) {
	now := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)
	release := directFetcherTestRelease()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/desktop/v1/connector-market/artifact-grants":
			if request.Method != http.MethodPost || request.Header.Get("Cookie") != "session=trusted" {
				t.Fatalf("grant request = %s cookie=%q", request.Method, request.Header.Get("Cookie"))
			}
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["workspaceId"] != "workspace-1" || body["connectorKey"] != release.ConnectorKey ||
				body["releaseDigest"] != release.ReleaseDigest || body["artifactSha256"] != release.Artifact.SHA256 ||
				body["objectVersion"] != release.Artifact.ObjectVersion {
				t.Fatalf("grant body = %#v", body)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"downloadUrl": server.URL + "/signed-download", "method": "GET",
				"expiresAtMs": fmt.Sprint(now.Add(4 * time.Minute).UnixMilli()), "connectorKey": release.ConnectorKey,
				"releaseDigest": release.ReleaseDigest, "artifactSha256": release.Artifact.SHA256,
				"sizeBytes": fmt.Sprint(release.Artifact.SizeBytes), "mediaType": release.Artifact.MediaType,
				"objectVersion": release.Artifact.ObjectVersion})
		case "/signed-download":
			if request.Header.Get("Cookie") != "" {
				t.Fatal("account authorization leaked to presigned download")
			}
			writer.Header().Set("Content-Type", release.Artifact.MediaType)
			writer.Header().Set("Content-Length", "3")
			_, _ = writer.Write([]byte("zip"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	fetcher, err := NewGrantFetcher(GrantFetcherConfig{BaseURL: server.URL + "/api/desktop", HTTPClient: server.Client(),
		AuthorizeRequest:    func(request *http.Request) error { request.Header.Set("Cookie", "session=trusted"); return nil },
		WorkspaceIDProvider: func(context.Context) (string, error) { return "workspace-1", nil }, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	response, err := fetcher.Fetch(context.Background(), FetchRequest{Release: release})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	content, _ := io.ReadAll(response.Body)
	if string(content) != "zip" || response.ContentLength != 3 || response.MediaType != release.Artifact.MediaType {
		t.Fatalf("response=%#v content=%q", response, content)
	}
}

func TestGrantFetcherRejectsMismatchedGrantProjection(t *testing.T) {
	now := time.Now().UTC()
	release := directFetcherTestRelease()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{"downloadUrl": "https://artifacts.example.test/file", "method": "GET",
			"expiresAtMs": fmt.Sprint(now.Add(time.Minute).UnixMilli()), "connectorKey": release.ConnectorKey,
			"releaseDigest": release.ReleaseDigest, "artifactSha256": release.Artifact.SHA256,
			"sizeBytes": fmt.Sprint(release.Artifact.SizeBytes + 1), "mediaType": release.Artifact.MediaType,
			"objectVersion": release.Artifact.ObjectVersion})
	}))
	defer server.Close()
	fetcher, err := NewGrantFetcher(GrantFetcherConfig{BaseURL: server.URL, HTTPClient: server.Client(),
		AuthorizeRequest: func(*http.Request) error { return nil }, WorkspaceIDProvider: func(context.Context) (string, error) { return "workspace-1", nil },
		Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fetcher.Fetch(context.Background(), FetchRequest{Release: release}); err == nil {
		t.Fatal("expected mismatched grant to fail closed")
	}
}
