package proxmox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/luthermonson/go-proxmox"

	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// notFoundPhrases are how PVE's die() paths report a missing VM, config or
// HA resource (deleting one says "not HA managed"); pveproxy sends them as
// the reason phrase and in the body.
var notFoundPhrases = []string{"does not exist", "no such resource", "not HA managed"}

// get decodes the data envelope of an authenticated GET on an
// /api2/json-relative path into v, translating go-proxmox's auth failure
// into this package's sentinel.
func (d *driver) get(ctx context.Context, path string, v any) error {
	if err := d.client.Get(ctx, path, v); err != nil {
		return fmt.Errorf("get %s: %w", path, mapAPIError(err))
	}
	return nil
}

// write sends the first value of every form key as a flat JSON object (PVE
// accepts JSON bodies for every form parameter) and decodes the data
// envelope into v; a nil v discards it.
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

// mapAPIError is the one place API failures become this package's sentinels;
// the PVE message is kept behind ErrNotFound for diagnostics.
func mapAPIError(err error) error {
	if errors.Is(err, proxmox.ErrNotAuthorized) {
		return ErrPermission
	}
	for _, phrase := range notFoundPhrases {
		if strings.Contains(err.Error(), phrase) {
			return fmt.Errorf("%w: %w", ErrNotFound, err)
		}
	}
	return err
}

// newProxmoxClient builds the go-proxmox client and returns the *http.Client
// under it for raw requests; the token becomes an immutable Go string Zeroize
// can't reach, bounded to this call. The whole-request timeout stays zero
// because ISO uploads and WaitTask polls outlive any fixed budget; the
// transport bounds dial, TLS and response headers, bodies answer to ctx.
func newProxmoxClient(endpoint string, apiToken []byte, insecure bool) (client *proxmox.Client, httpClient *http.Client, err error) {
	if endpoint == "" {
		return nil, nil, errors.New("proxmox client: endpoint is required")
	}
	if len(apiToken) == 0 {
		return nil, nil, errors.New("proxmox client: no credentials (need an api token)")
	}
	id, secret, ok := strings.Cut(string(apiToken), "=")
	if !ok {
		return nil, nil, errors.New("proxmox client: api token must be in id=secret form")
	}
	if insecure {
		warnInsecureTLS(endpoint)
	}
	httpClient = httputil.NewOptionalInsecure(insecure, 0)
	client = proxmox.NewClient(APIBaseURL(endpoint), proxmox.WithHTTPClient(httpClient), proxmox.WithAPIToken(id, secret))
	return client, httpClient, nil
}

var insecureTLSWarnOnce sync.Once

// warnInsecureTLS logs once per process through the logutil facade rather
// than threading a logger through every constructor.
func warnInsecureTLS(endpoint string) {
	insecureTLSWarnOnce.Do(func() {
		logutil.Warn("proxmox tls verification disabled (insecure: true)", logutil.LF("endpoint", endpoint))
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
