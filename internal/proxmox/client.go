package proxmox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/luthermonson/go-proxmox"

	"github.com/qxtaiba/okdctl/internal/httputil"
)

// get decodes the data envelope of an authenticated GET on an
// /api2/json-relative path into v, translating go-proxmox's auth failure
// into this package's sentinel.
func (d *driver) get(ctx context.Context, path string, v any) error {
	if err := d.client.Get(ctx, path, v); err != nil {
		return fmt.Errorf("get %s: %w", path, mapAPIError(err))
	}
	return nil
}

// write sends form as a flat JSON object (PVE accepts JSON bodies for every
// form parameter) and decodes the data envelope into v; a nil v discards it.
func (d *driver) write(ctx context.Context, method, path string, form url.Values, v any) error {
	var body []byte
	if form != nil {
		fields := make(map[string]string, len(form))
		for k := range form {
			fields[k] = form.Get(k)
		}
		var err error
		if body, err = json.Marshal(fields); err != nil {
			return fmt.Errorf("%s %s: encode form: %w", method, path, err)
		}
	}
	if err := d.client.Req(ctx, method, path, body, v); err != nil {
		return fmt.Errorf("%s %s: %w", method, path, mapAPIError(err))
	}
	return nil
}

// task issues a write that returns a UPID and wraps it with node.
func (d *driver) task(ctx context.Context, method, node, path string, form url.Values) (Task, error) {
	var upid string
	if err := d.write(ctx, method, path, form, &upid); err != nil {
		return Task{}, err
	}
	return Task{Node: node, UPID: upid}, nil
}

func mapAPIError(err error) error {
	if errors.Is(err, proxmox.ErrNotAuthorized) {
		return ErrPermission
	}
	return err
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

// warnInsecureTLS logs once per process via slog.Default rather than
// threading a logger through every constructor.
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
