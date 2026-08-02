package relaytransport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// Dial opens one Relay WebSocket and exposes binary messages as a byte stream.
func Dial(ctx context.Context, request DialRequest) (net.Conn, error) {
	ws, err := dialWebSocket(ctx, request)
	if err != nil {
		return nil, err
	}
	return newWebSocketByteConn(ws), nil
}

func dialWebSocket(ctx context.Context, request DialRequest) (*websocket.Conn, error) {
	request, err := normalizeDialRequest(request)
	if err != nil {
		return nil, err
	}

	header := request.Header.Clone()
	dialer := *websocket.DefaultDialer
	dialer.Subprotocols = []string{request.Subprotocol}

	endpoint, _ := url.Parse(request.Endpoint)
	query := endpoint.Query()
	for key, values := range request.Query {
		query[key] = append([]string(nil), values...)
	}
	endpoint.RawQuery = query.Encode()

	ws, response, err := dialer.DialContext(ctx, endpoint.String(), header)
	if err != nil {
		if ws != nil {
			_ = ws.Close()
		}
		return nil, newDialError(response, err)
	}
	if ws.Subprotocol() != request.Subprotocol {
		_ = ws.Close()
		return nil, fmt.Errorf("relay websocket requires subprotocol %q, got %q", request.Subprotocol, ws.Subprotocol())
	}
	return ws, nil
}

func normalizeDialRequest(request DialRequest) (DialRequest, error) {
	endpoint, err := url.Parse(strings.TrimSpace(request.Endpoint))
	if err != nil {
		return DialRequest{}, fmt.Errorf("parse relay endpoint: %w", err)
	}
	if endpoint.Scheme != "ws" && endpoint.Scheme != "wss" {
		return DialRequest{}, fmt.Errorf("relay endpoint scheme %q is not ws or wss", endpoint.Scheme)
	}
	if strings.TrimSpace(endpoint.Host) == "" {
		return DialRequest{}, errors.New("relay endpoint host is empty")
	}
	if endpoint.User != nil {
		return DialRequest{}, errors.New("relay endpoint userinfo is not allowed")
	}
	request.Endpoint = endpoint.String()
	request.Subprotocol = strings.TrimSpace(request.Subprotocol)
	if request.Subprotocol == "" {
		return DialRequest{}, errors.New("relay websocket subprotocol is required")
	}
	query := make(url.Values, len(request.Query))
	for key, values := range request.Query {
		query[key] = append([]string(nil), values...)
	}
	request.Query = query
	request.Header = request.Header.Clone()
	return request, nil
}

// DialError exposes bounded HTTP retry metadata without exposing response
// bodies or authorization headers.
type DialError struct {
	statusCode int
	retryAfter string
	cause      error
}

func newDialError(response *http.Response, cause error) error {
	if response == nil || response.StatusCode <= 0 {
		return fmt.Errorf("dial relay websocket: %w", cause)
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	return &DialError{
		statusCode: response.StatusCode,
		retryAfter: strings.TrimSpace(response.Header.Get("Retry-After")),
		cause:      cause,
	}
}

func (e *DialError) Error() string {
	return fmt.Sprintf("dial relay websocket: http %d: %v", e.statusCode, e.cause)
}

func (e *DialError) Unwrap() error          { return e.cause }
func (e *DialError) HTTPStatusCode() int    { return e.statusCode }
func (e *DialError) HTTPRetryAfter() string { return e.retryAfter }
