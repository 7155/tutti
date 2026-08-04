package mobile

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestStreamReadPreservesFinalBytesReturnedWithEOF(t *testing.T) {
	t.Parallel()
	stream := &Stream{conn: &finalBytesConn{remaining: []byte("final response frame")}}
	buffer := make([]byte, 1024)

	count := stream.ReadInto(buffer)
	if count != len("final response frame") {
		t.Fatalf("read final bytes count = %d, want %d", count, len("final response frame"))
	}
	if string(buffer[:count]) != "final response frame" {
		t.Fatalf("read = %q, want final response frame", buffer[:count])
	}

	if count := stream.ReadInto(buffer); count != -1 {
		t.Fatalf("read after final bytes count = %d, want -1", count)
	}
}

func TestLoopbackLinksExchangeDescriptionsAndStreams(t *testing.T) {
	t.Parallel()
	caller, err := NewLoopbackLink()
	if err != nil {
		t.Fatal(err)
	}
	defer caller.Close()
	owner, err := NewLoopbackLink()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()

	callerDescription, err := caller.LocalDescription(20_000)
	if err != nil {
		t.Fatal(err)
	}
	ownerDescription, err := owner.LocalDescription(20_000)
	if err != nil {
		t.Fatal(err)
	}
	ownerResult := make(chan error, 1)
	go func() {
		_, connectErr := owner.Connect(callerDescription, false, 20_000)
		ownerResult <- connectErr
	}()
	if _, err := caller.Connect(ownerDescription, true, 20_000); err != nil {
		t.Fatal(err)
	}
	if err := <-ownerResult; err != nil {
		t.Fatal(err)
	}

	serveDone := make(chan error, 1)
	go func() {
		stream, acceptErr := owner.AcceptStream(20_000)
		if acceptErr != nil {
			serveDone <- acceptErr
			return
		}
		defer stream.Close()
		buffer := make([]byte, 1024)
		for {
			count := stream.ReadInto(buffer)
			if count <= 0 {
				serveDone <- io.EOF
				return
			}
			if _, writeErr := stream.Write(buffer[:count]); writeErr != nil {
				serveDone <- writeErr
				return
			}
		}
	}()
	stream, err := caller.OpenStream(20_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SetDeadline(20_000); err != nil {
		t.Fatal(err)
	}
	payload := []byte("gomobile-authenticated-link")
	if _, err := stream.Write(payload); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, len(payload))
	count := stream.ReadInto(buffer)
	if count != len(payload) {
		t.Fatalf("read count = %d, want %d", count, len(payload))
	}
	got := buffer[:count]
	if string(got) != string(payload) {
		t.Fatalf("echo = %q, want %q", got, payload)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serveDone; err != nil && err != io.EOF {
		t.Fatal(err)
	}
}

func TestOpenStreamWithRelayStartsBeforeConnect(t *testing.T) {
	t.Parallel()
	server := newRelayEchoServer(t)
	defer server.Close()

	link, err := NewLoopbackLink()
	if err != nil {
		t.Fatal(err)
	}
	defer link.Close()

	stream, err := link.OpenStreamWithRelay(
		server.endpoint,
		`{"authority_id":["authority-1"]}`,
		`{"Authorization":["Bearer target-token"]}`,
		"relay.test.v1",
		5_000,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if got := stream.transport; got != "relay" {
		t.Fatalf("transport = %q, want relay", got)
	}
}

func TestOpenStreamCanStartBeforeConnect(t *testing.T) {
	t.Parallel()
	caller, err := NewLoopbackLink()
	if err != nil {
		t.Fatal(err)
	}
	defer caller.Close()
	owner, err := NewLoopbackLink()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()

	callerDescription, err := caller.LocalDescription(20_000)
	if err != nil {
		t.Fatal(err)
	}
	ownerDescription, err := owner.LocalDescription(20_000)
	if err != nil {
		t.Fatal(err)
	}

	streamResult := make(chan struct {
		stream *Stream
		err    error
	}, 1)
	go func() {
		stream, openErr := caller.OpenStream(5_000)
		streamResult <- struct {
			stream *Stream
			err    error
		}{stream: stream, err: openErr}
	}()
	ownerResult := make(chan error, 1)
	go func() {
		_, connectErr := owner.Connect(callerDescription, false, 20_000)
		ownerResult <- connectErr
	}()
	if _, err := caller.Connect(ownerDescription, true, 20_000); err != nil {
		t.Fatal(err)
	}
	if err := <-ownerResult; err != nil {
		t.Fatal(err)
	}
	acceptResult := make(chan struct {
		stream *Stream
		err    error
	}, 1)
	go func() {
		stream, acceptErr := owner.AcceptStream(5_000)
		acceptResult <- struct {
			stream *Stream
			err    error
		}{stream: stream, err: acceptErr}
	}()
	result := <-streamResult
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.stream == nil {
		t.Fatal("OpenStream returned a nil stream")
	}
	defer result.stream.Close()
	if result.stream.transport != "direct" {
		t.Fatalf("transport = %q, want direct", result.stream.transport)
	}
	if _, err := result.stream.Write([]byte("pending-stream")); err != nil {
		t.Fatal(err)
	}
	acceptedResult := <-acceptResult
	if acceptedResult.err != nil {
		t.Fatal(acceptedResult.err)
	}
	accepted := acceptedResult.stream
	if accepted == nil {
		t.Fatal("AcceptStream returned a nil stream")
	}
	defer accepted.Close()
}

func TestProtocolEpochMatchesApplicationPrelude(t *testing.T) {
	if ProtocolEpoch() != ApplicationProtocolEpoch {
		t.Fatalf("ProtocolEpoch() = %d, want %d", ProtocolEpoch(), ApplicationProtocolEpoch)
	}
}

func TestDialRelayOpensConfiguredByteStream(t *testing.T) {
	t.Parallel()
	server := newRelayEchoServer(t)
	defer server.Close()
	stream, err := DialRelay(
		server.endpoint,
		`{"authority_id":["authority-1"]}`,
		`{"Authorization":["Bearer target-token"]}`,
		"relay.test.v1",
		5_000,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := stream.SetDeadline(5_000); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Write([]byte("relay-payload")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, len("relay-payload"))
	count := stream.ReadInto(buffer)
	if count != len(buffer) || string(buffer[:count]) != "relay-payload" {
		t.Fatalf("relay echo = %q (%d bytes)", buffer[:max(count, 0)], count)
	}
}

func TestDialRelayRejectsMalformedValuesBeforeDial(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		query   string
		headers string
		want    string
	}{
		{name: "query", query: "[", headers: "{}", want: "decode relay query"},
		{name: "headers", query: "{}", headers: "[", want: "decode relay headers"},
		{name: "empty key", query: `{" ":["value"]}`, headers: "{}", want: "empty key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := DialRelay("ws://relay.invalid", test.query, test.headers, "relay.test.v1", 1)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DialRelay() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

type relayTestServer struct {
	endpoint string
	server   *httptest.Server
}

func newRelayEchoServer(t *testing.T) *relayTestServer {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin:  func(*http.Request) bool { return true },
		Subprotocols: []string{"relay.test.v1"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("authority_id") != "authority-1" {
			http.Error(w, "authority missing", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer target-token" {
			http.Error(w, "authorization missing", http.StatusUnauthorized)
			return
		}
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		messageType, payload, err := connection.ReadMessage()
		if err != nil {
			return
		}
		_ = connection.WriteMessage(messageType, payload)
	}))
	return &relayTestServer{
		endpoint: "ws" + strings.TrimPrefix(server.URL, "http"),
		server:   server,
	}
}

func (s *relayTestServer) Close() {
	s.server.Close()
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

type finalBytesConn struct {
	net.Conn
	remaining []byte
}

func (c *finalBytesConn) Read(buffer []byte) (int, error) {
	if len(c.remaining) == 0 {
		return 0, io.EOF
	}
	count := copy(buffer, c.remaining)
	c.remaining = c.remaining[count:]
	return count, io.EOF
}
