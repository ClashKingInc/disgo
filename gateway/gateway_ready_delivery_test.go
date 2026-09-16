package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestOpenWaitsForReadyEventDelivery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if err = conn.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 45_000}}); err != nil {
			return
		}
		var message struct {
			Op int `json:"op"`
		}
		if err = conn.ReadJSON(&message); err != nil || message.Op != int(OpcodeIdentify) {
			return
		}
		if err = conn.WriteJSON(map[string]any{
			"op": 0, "t": "READY", "s": 1,
			"d": map[string]any{
				"session_id": "test-session", "resume_gateway_url": "ws://" + r.Host,
				"user": map[string]any{"id": "123", "username": "test"}, "guilds": []any{},
				"application": map[string]any{"id": "456"},
			},
		}); err != nil {
			return
		}
		for {
			if err = conn.ReadJSON(&message); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	readyHandlerStarted := make(chan struct{})
	releaseReadyHandler := make(chan struct{})
	gateway := New("token", func(_ Gateway, eventType EventType, _ int, _ EventData) {
		if eventType == EventTypeReady {
			close(readyHandlerStarted)
			<-releaseReadyHandler
		}
	},
		WithURL("ws"+strings.TrimPrefix(server.URL, "http")),
		WithCompression(CompressionNone),
		WithAutoReconnect(false),
	)
	openDone := make(chan error, 1)
	go func() { openDone <- gateway.Open(context.Background()) }()
	select {
	case <-readyHandlerStarted:
	case <-time.After(time.Second):
		t.Fatal("READY event handler did not start")
	}
	select {
	case err := <-openDone:
		close(releaseReadyHandler)
		t.Fatalf("gateway opened before READY event delivery completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseReadyHandler)
	select {
	case err := <-openDone:
		if err != nil {
			t.Fatalf("open gateway after READY event delivery: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("gateway did not open after READY event delivery completed")
	}
	gateway.Close(t.Context())
}
