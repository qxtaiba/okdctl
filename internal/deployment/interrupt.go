package deployment

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// ErrInterrupted is returned when deployment is interrupted by a signal.
var ErrInterrupted = errors.New("deployment interrupted")

// InterruptHandler manages graceful shutdown on interrupt signals.
// It uses atomic operations for thread-safe access to the interrupted flag.
type InterruptHandler struct {
	sigChan     chan os.Signal
	cancel      context.CancelFunc
	interrupted int32 // accessed atomically

	// OnInterrupt is called when an interrupt signal is received.
	// This allows the caller to display messages or perform cleanup.
	// If nil, no callback is executed.
	OnInterrupt func(sig os.Signal)
}

// NewInterruptHandler creates a new interrupt handler and returns its context.
// The handler listens for SIGINT and SIGTERM signals.
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
		return // Channel closed, exit cleanly
	}

	atomic.StoreInt32(&h.interrupted, 1)
	h.cancel()

	if h.OnInterrupt != nil {
		h.OnInterrupt(sig)
	}
	// Don't os.Exit here - let context cancellation propagate and defers run
}

// WasInterrupted returns true if an interrupt signal was received.
// This method is safe to call from any goroutine.
func (h *InterruptHandler) WasInterrupted() bool {
	return atomic.LoadInt32(&h.interrupted) != 0
}

// Cleanup stops signal handling and cancels the context.
// This should be called in a defer after creating the handler.
func (h *InterruptHandler) Cleanup() {
	signal.Stop(h.sigChan)
	close(h.sigChan)
	h.cancel()
}
