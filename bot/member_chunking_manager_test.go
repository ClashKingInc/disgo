package bot

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
)

func TestMemberChunkCancellationDoesNotCloseChannelDuringSend(t *testing.T) {
	const nonce = "member-request"
	members := make(chan discord.Member)
	sending := make(chan struct{})
	request := &chunkingRequest{
		nonce:      nonce,
		memberChan: members,
		done:       make(chan struct{}),
		memberFilterFunc: func(discord.Member) bool {
			close(sending)
			return true
		},
	}
	manager := &memberChunkingManagerImpl{
		client:           &Client{Caches: cache.New()},
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		chunkingRequests: map[string]*chunkingRequest{nonce: request},
	}
	panicValue := make(chan any, 1)
	go func() {
		var recovered any
		defer func() { panicValue <- recovered }()
		defer func() { recovered = recover() }()
		manager.HandleChunk(gateway.EventGuildMembersChunk{
			Nonce:      nonce,
			Members:    []discord.Member{{}},
			ChunkCount: 2,
		})
	}()

	select {
	case <-sending:
	case <-time.After(time.Second):
		t.Fatal("member chunk handler did not reach the channel send")
	}
	cleanupRequest(manager, request)

	select {
	case recovered := <-panicValue:
		if recovered != nil {
			t.Fatalf("member chunk handler panicked during cancellation: %v", recovered)
		}
	case <-time.After(time.Second):
		t.Fatal("member chunk handler did not exit after cancellation")
	}
	cleanupRequest(manager, request)
	if _, ok := <-members; ok {
		t.Fatal("member channel remained open after cancellation")
	}
}
