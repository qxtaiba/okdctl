package deployment

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

var ErrInterrupted = errors.New("deployment interrupted")

type InterruptHandler struct {
	sigChan     chan os.Signal
	cancel      context.CancelFunc
	interrupted int32 // accessed atomically

	// OnInterrupt is called when an interrupt signal is received.
	// This allows the caller to display messages or perform cleanup.
	// If nil, no callback is executed.
	OnInterrupt func(sig os.Signal)
}

// NewInterruptHandler listens for SIGINT and SIGTERM and returns a handler
// paired with a context that is cancelled on signal receipt.
func NewInterruptHandler() (*InterruptHandler, context.Context) {
	ctx, cancel := context.WithCancel(context.Background())
	h := &InterruptHandler{
		sigChan: make(chan os.Signal, 1),
		cancel:  cancel,
	}
	signal.Notify(h.sigChan, os.Interrupt, syscall.SIGTERM)
	go h.listen()
	return h, ctx
}

func (h *InterruptHandler) listen() {
	sig, ok := <-h.sigChan
	if !ok {
		return
	}

	atomic.StoreInt32(&h.interrupted, 1)
	h.cancel()

	if h.OnInterrupt != nil {
		h.OnInterrupt(sig)
	}
	// Don't os.Exit here - let context cancellation propagate and defers run
}

// WasInterrupted is safe to call from any goroutine.
func (h *InterruptHandler) WasInterrupted() bool {
	return atomic.LoadInt32(&h.interrupted) != 0
}

// Cleanup should be deferred immediately after creating the handler.
func (h *InterruptHandler) Cleanup() {
	signal.Stop(h.sigChan)
	close(h.sigChan)
	h.cancel()
}
