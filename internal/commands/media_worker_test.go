package commands

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaWorker_DoubleStartPanics(t *testing.T) {
	app := NewAppWithDeps(&MockWAClient{}, &MockMessageStore{}, t.TempDir(), "test")
	w := newMediaDownloadWorker(app, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.Start(ctx)
	defer w.Stop()

	require.Panics(t, func() {
		w.Start(ctx)
	}, "calling Start twice should panic")
}

func TestMediaWorker_EnqueueDropsWhenFull(t *testing.T) {
	app := NewAppWithDeps(&MockWAClient{}, &MockMessageStore{}, t.TempDir(), "test")
	// 1 worker, buffer = 1*4 = 4
	w := newMediaDownloadWorker(app, 1)

	// Set ctx without starting goroutines, so nothing drains the channel.
	w.ctx, w.cancel = context.WithCancel(context.Background())
	defer w.cancel()

	// Fill the buffer (capacity 4).
	for i := 0; i < 4; i++ {
		w.Enqueue(mediaJob{messageID: "fill", chatJID: "chat"})
	}

	// Next enqueue should be dropped, not spawn a goroutine.
	w.Enqueue(mediaJob{messageID: "overflow", chatJID: "chat"})

	w.mu.Lock()
	dropped := w.droppedCount
	w.mu.Unlock()

	assert.Equal(t, 1, dropped, "one job should have been dropped")
}

func TestMediaWorker_NilReceiverSafe(t *testing.T) {
	var w *mediaDownloadWorker

	// These should all be no-ops, not panics.
	require.NotPanics(t, func() {
		w.Start(context.Background())
		w.Enqueue(mediaJob{messageID: "x", chatJID: "y"})
		w.Stop()
		w.PrintSummary()
	})
}
