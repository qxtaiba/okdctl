package deploy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/deploymetrics"
	"github.com/qxtaiba/okdctl/internal/distribution/okd"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// metricsReadHeaderTimeout / metricsReadTimeout / metricsWriteTimeout set
// conservative HTTP server bounds for the unauthenticated /metrics endpoint.
// metricsIdleTimeout leaves slack for Prometheus scrapers that reconnect on
// their configured scrape_interval (typically 15–60 s); metricsShutdownTimeout
// gives in-flight scrapes time to drain on graceful stop.
const (
	metricsReadHeaderTimeout = 5 * time.Second
	metricsReadTimeout       = 10 * time.Second
	metricsWriteTimeout      = 10 * time.Second
	metricsIdleTimeout       = 60 * time.Second
	metricsShutdownTimeout   = 5 * time.Second
)

// startMetricsServer starts a Prometheus metrics HTTP server on addr (disabled
// when addr is empty). Returns a stop closure that shuts the server down with
// a 5-second deadline and surfaces any bind error, plus any provisioner
// options the caller must apply so orchestrated phases feed observations to
// the recorder.
//
// Bare ":port" is rewritten to "127.0.0.1:port" so the unauthenticated
// listener does not leak to the network by default. Wildcard addresses
// (0.0.0.0 or [::]) are rejected unless allowNetwork is true; pass
// --metrics-allow-network to opt in.
func startMetricsServer(ctx context.Context, addr string, allowNetwork bool) (func() error, []okd.ProvisionerOption, error) {
	if addr == "" {
		return func() error { return nil }, nil, nil
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, nil, &errtypes.ConfigError{Msg: fmt.Sprintf("invalid --metrics-addr %q", addr), Err: err}
	}
	if host != "" {
		parsed, parseErr := netip.ParseAddr(host)
		if parseErr == nil && parsed.IsUnspecified() && !allowNetwork {
			return nil, nil, &errtypes.ConfigError{Msg: "wildcard metrics bind requires --metrics-allow-network"}
		}
	}
	rec := deploymetrics.NewRecorder()
	mux := http.NewServeMux()
	mux.Handle("/metrics", rec.Handler())
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// BaseContext propagates the deploy ctx so in-flight scrape connections
		// are cancelled when the parent context is cancelled.
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: metricsReadHeaderTimeout,
		ReadTimeout:       metricsReadTimeout,
		WriteTimeout:      metricsWriteTimeout,
		IdleTimeout:       metricsIdleTimeout,
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, &errtypes.ConfigError{Msg: fmt.Sprintf("metrics bind failed on %q", addr), Err: err}
	}
	tui.Info("metrics endpoint listening", tui.LF("addr", addr))
	// errCh cap=1: the goroutine sends exactly once and never blocks, so it
	// exits cleanly even if stop is never called (early return on phase error).
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	stop := func() error {
		// Use Background, not the caller's ctx: by stop() time the parent ctx
		// is already cancelled by SIGINT, and we need the 5s drain to complete.
		shutCtx, cancel := context.WithTimeout(context.Background(), metricsShutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		// Shutdown's return guarantees ListenAndServe has exited; drain errCh.
		select {
		case err := <-errCh:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		default:
			return nil
		}
	}
	return stop, []okd.ProvisionerOption{okd.WithMetricsRecorder(rec)}, nil
}
