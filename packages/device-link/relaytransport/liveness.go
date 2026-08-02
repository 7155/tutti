package relaytransport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type livenessConfig struct {
	pingInterval time.Duration
	pongTimeout  time.Duration
	pingPayload  []byte
}

func startLiveness(ctx context.Context, ws *websocket.Conn, cfg livenessConfig) (func(), error) {
	if err := ws.SetReadDeadline(time.Now().Add(cfg.pongTimeout)); err != nil {
		return nil, fmt.Errorf("set relay owner read deadline: %w", err)
	}
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(cfg.pongTimeout))
	})
	payload := append([]byte(nil), cfg.pingPayload...)
	if len(payload) == 0 {
		payload = []byte("owner")
	}

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(cfg.pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				if err := ws.WriteControl(websocket.PingMessage, payload, time.Now().Add(time.Second)); err != nil {
					_ = ws.Close()
					return
				}
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-stopped
		})
	}, nil
}
