package gateway

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/klauspost/compress/zstd"
)

type blockingReader struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingReader) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return 0, io.EOF
}

func TestZstdStreamCloseWaitsForActiveReceive(t *testing.T) {
	serverConn, clientConn := websocketPair(t)

	reader := &blockingReader{started: make(chan struct{}), release: make(chan struct{})}
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	if err = decoder.Reset(reader); err != nil {
		t.Fatal(err)
	}

	transport := newZstdStreamTransport(clientConn, slog.Default())
	transport.inflator = decoder
	receiveDone := make(chan struct{})
	go func() {
		defer close(receiveDone)
		_, _ = transport.ReceiveMessage()
	}()
	if err = serverConn.WriteMessage(websocket.BinaryMessage, []byte{0}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reader.started:
	case <-time.After(time.Second):
		t.Fatal("receive did not enter the decoder")
	}

	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		_ = transport.Close()
	}()

	closedBeforeReceive := false
	select {
	case <-closeDone:
		closedBeforeReceive = true
	case <-time.After(50 * time.Millisecond):
	}
	close(reader.release)
	select {
	case <-receiveDone:
	case <-time.After(time.Second):
		t.Fatal("receive did not exit after decoder input was released")
	}
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("close did not finish after receive exited")
	}
	if closedBeforeReceive {
		t.Fatal("close returned while ReceiveMessage was still using the decoder")
	}
}

func websocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	serverConn := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		serverConn <- conn
	}))
	t.Cleanup(server.Close)

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	serverSide := <-serverConn
	t.Cleanup(func() { _ = serverSide.Close() })
	t.Cleanup(func() { _ = conn.Close() })
	return serverSide, conn
}
