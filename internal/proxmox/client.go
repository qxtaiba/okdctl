package proxmox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/luthermonson/go-proxmox"

	"github.com/qxtaiba/okdctl/internal/httputil"
)

// get decodes the data envelope of an authenticated GET on an
// /api2/json-relative path into v, translating go-proxmox's auth and
// lookup failures into this package's sentinels.
func (d *driver) get(ctx context.Context, path string, v any) error {
	if err := d.client.Get(ctx, path, v); err != nil {
		return fmt.Errorf("get %s: %w", path, mapAPIError(err))
	}
	return nil
}

func mapAPIError(err error) error {
	switch {
	case errors.Is(err, proxmox.ErrNotAuthorized):
		return ErrPermission
	case errors.Is(err, proxmox.ErrNotFound):
		return ErrNotFound
	default:
		return err
	}
}

// newProxmoxClient builds the shared go-proxmox client; the credential
// becomes an immutable Go string Zeroize can't reach, bounded to this call.
func newProxmoxClient(endpoint, username string, password, apiToken []byte, insecure bool, timeout time.Duration) (*proxmox.Client, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("proxmox client: endpoint is required")
	}
	if insecure {
		warnInsecureTLS(endpoint)
	}
	httpClient := httputil.NewOptionalInsecure(insecure, timeout)
	base := APIBaseURL(endpoint)

	switch {
	case len(password) > 0:
		return proxmox.NewClient(
			base,
			proxmox.WithHTTPClient(httpClient),
			proxmox.WithCredentials(&proxmox.Credentials{Username: username, Password: string(password)}),
		), nil
	case len(apiToken) > 0:
		id, secret, ok := strings.Cut(string(apiToken), "=")
		if !ok {
			return nil, fmt.Errorf("proxmox client: PROXMOX_VE_API_TOKEN must be in id=secret form")
		}
		return proxmox.NewClient(
			base,
			proxmox.WithHTTPClient(httpClient),
			proxmox.WithAPIToken(id, secret),
		), nil
	default:
		return nil, fmt.Errorf("proxmox client: no credentials (need password or api token)")
	}
}

var insecureTLSWarnOnce sync.Once

// warnInsecureTLS logs insecure:true once per process (PowerCycler builds a
// client per call) via slog.Default rather than threading a logger through.
func warnInsecureTLS(endpoint string) {
	insecureTLSWarnOnce.Do(func() {
		slog.Warn("proxmox tls verification disabled (insecure: true)", "endpoint", endpoint)
	})
}

// APIBaseURL normalizes endpoint (https default, trailing slash trimmed) and
// appends /api2/json; every go-proxmox client builds its base URL here.
func APIBaseURL(endpoint string) string {
	return normalizeEndpoint(endpoint) + "/api2/json"
}

func normalizeEndpoint(endpoint string) string {
	e := strings.TrimRight(endpoint, "/")
	if strings.Contains(e, "://") {
		return e
	}
	return "https://" + e
}

func bytesToMiB(b uint64) int { return int(b / (1024 * 1024)) } //nolint:gosec // G115: MiB-scale value fits int
