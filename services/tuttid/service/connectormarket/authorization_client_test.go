package connectormarket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	market "github.com/tutti-os/tutti/packages/connector/host"
)

func TestConnectorAuthorizationClientStartsAccountScopedSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Cookie") != "sid=user-session" {
			t.Fatalf("cookie = %q", request.Header.Get("Cookie"))
		}
		switch request.URL.Path {
		case "/api/desktop/v1/connectors/gmail/authorization-options":
			_, _ = response.Write([]byte(`{"options":[{"authorizationMethod":"oauth2"}]}`))
		case "/api/desktop/v1/connectors/gmail/authorization-sessions":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["authorizationMethod"] != "oauth2" || body["clientRequestId"] != "request-1" {
				t.Fatalf("body = %#v", body)
			}
			_, _ = response.Write([]byte(`{"session":{"sessionId":"auth-1","nextAction":{"url":"https://auth.example/connect"}}}`))
		case "/api/desktop/v1/connector-authorization-sessions/auth-1":
			_, _ = response.Write([]byte(`{"session":{"status":"CONNECTOR_AUTHORIZATION_SESSION_STATUS_SUCCEEDED"}}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	client, err := NewConnectorAuthorizationClient(ConnectorAuthorizationClientConfig{
		BaseURL: server.URL + "/api/desktop", HTTPClient: server.Client(),
		AuthorizeRequest: func(request *http.Request) error { request.Header.Set("Cookie", "sid=user-session"); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Begin(context.Background(), market.AuthorizationStartRequest{
		OperationID: "operation-1", ClientRequestID: "request-1",
		Connector: market.Connector{Key: "gmail"},
		Release:   market.Release{Manifest: market.Manifest{AuthorizationKind: "oauth2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "auth-1" || result.AuthorizationURL != "https://auth.example/connect" || result.OperationID != "operation-1" {
		t.Fatalf("result = %#v", result)
	}
	observation, err := client.Observe(context.Background(), market.AuthorizationObserveRequest{Session: result})
	if err != nil {
		t.Fatal(err)
	}
	if observation.State != market.AuthorizationObservationConnected {
		t.Fatalf("observation = %#v", observation)
	}
}
