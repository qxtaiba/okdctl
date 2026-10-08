package proxmox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/luthermonson/go-proxmox"

	"github.com/qxtaiba/okdctl/internal/credentials"
	"github.com/qxtaiba/okdctl/internal/httputil"
)

const (
	isoContentType = "iso"
	isoTaskTimeout = 10 * time.Minute
)

// ISOStore manages one Proxmox storage's ISO volumes through the API, the
// sanctioned non-terraform path for files terraform only references. creds
// stays caller-owned and must outlive every call before its Zeroize.
type ISOStore struct {
	creds   *credentials.ProxmoxCredentials
	storage string
}

// NewISOStore returns an ISOStore for storage authenticated by creds.
func NewISOStore(creds *credentials.ProxmoxCredentials, storage string) *ISOStore {
	return &ISOStore{creds: creds, storage: storage}
}

// VolumeID returns the Proxmox volume id of ISO name on this storage.
func (s *ISOStore) VolumeID(name string) string {
	return s.storage + ":" + isoContentType + "/" + name
}

// ctxTransport binds uncancellable requests to ctx: go-proxmox's
// UploadReader sends with context.Background(), so without it an ISO upload
// ignores cancellation.
type ctxTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t ctxTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Context().Done() == nil {
		req = req.WithContext(t.ctx)
	}
	return t.base.RoundTrip(req)
}

// client builds a go-proxmox client whose requests all follow ctx; timeout 0
// leaves a long upload bounded by ctx alone.
func (s *ISOStore) client(ctx context.Context, timeout time.Duration) (*proxmox.Client, error) {
	if s.creds == nil {
		return nil, fmt.Errorf("proxmox client: no credentials (need password or api token)")
	}
	hc := httputil.NewOptionalInsecure(s.creds.Insecure, timeout)
	hc.Transport = ctxTransport{ctx: ctx, base: hc.Transport}
	return buildProxmoxClient(s.creds.Endpoint, s.creds.Username, s.creds.Password, s.creds.APIToken, s.creds.Insecure, hc)
}

// open reads the storage status on node; the read also opens the ticket
// session that UploadReader cannot open on its own for password auth.
func (s *ISOStore) open(ctx context.Context, client *proxmox.Client, node string) (*proxmox.Storage, error) {
	st, err := new(proxmox.Node).New(client, node).Storage(ctx, s.storage)
	if err != nil {
		return nil, fmt.Errorf("get storage %s on %s: %w", s.storage, node, err)
	}
	return st, nil
}

// StorageStatus is a storage's configuration and availability as one node sees it.
type StorageStatus struct {
	Type    string
	Content []string
	Active  bool
	Enabled bool
	Shared  bool
}

// Status reads the storage's status on node.
func (s *ISOStore) Status(ctx context.Context, node string) (*StorageStatus, error) {
	client, err := s.client(ctx, defaultProbeTimeout)
	if err != nil {
		return nil, err
	}
	st, err := s.open(ctx, client, node)
	if err != nil {
		return nil, err
	}
	return &StorageStatus{
		Type:    st.Type,
		Content: strings.Split(st.Content, ","),
		Active:  st.Active == 1,
		Enabled: st.Enabled == 1,
		Shared:  st.Shared == 1,
	}, nil
}

// Volumes lists the ISO volumes on node, keyed by file name with their size in bytes.
func (s *ISOStore) Volumes(ctx context.Context, node string) (map[string]uint64, error) {
	client, err := s.client(ctx, defaultProbeTimeout)
	if err != nil {
		return nil, err
	}
	st, err := s.open(ctx, client, node)
	if err != nil {
		return nil, err
	}
	content, err := st.GetContent(ctx)
	if err != nil {
		return nil, fmt.Errorf("list storage %s on %s: %w", s.storage, node, err)
	}
	prefix := s.VolumeID("")
	volumes := make(map[string]uint64)
	for _, item := range content {
		if item == nil {
			continue
		}
		if name, ok := strings.CutPrefix(item.Volid, prefix); ok && name != "" {
			volumes[name] = item.Size
		}
	}
	return volumes, nil
}

// Upload sends the local ISO at path to node and waits for Proxmox to verify
// sha256 server-side and copy it into place; a mismatch fails the task.
func (s *ISOStore) Upload(ctx context.Context, node, path, sha256 string) error {
	name := filepath.Base(path)
	client, err := s.client(ctx, 0)
	if err != nil {
		return err
	}
	st, err := s.open(ctx, client, node)
	if err != nil {
		return err
	}
	task, err := st.UploadWithHash(isoContentType, path, nil, sha256, "sha256")
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("upload %s: %w", name, ctxErr)
		}
		return fmt.Errorf("upload %s: %w", name, err)
	}
	if task == nil {
		return fmt.Errorf("upload %s: proxmox returned no task id", name)
	}
	if err := waitTask(ctx, task, isoTaskTimeout); err != nil {
		return fmt.Errorf("import %s: %w", name, err)
	}
	return nil
}

// Remove deletes ISO name from the storage via node and waits for the task.
func (s *ISOStore) Remove(ctx context.Context, node, name string) error {
	client, err := s.client(ctx, defaultProbeTimeout)
	if err != nil {
		return err
	}
	st, err := s.open(ctx, client, node)
	if err != nil {
		return err
	}
	task, err := st.DeleteContent(ctx, s.VolumeID(name))
	if err != nil {
		return fmt.Errorf("delete %s: %w", s.VolumeID(name), err)
	}
	// PVE may delete synchronously and return no task id.
	if task == nil {
		return nil
	}
	if err := waitTask(ctx, task, isoTaskTimeout); err != nil {
		return fmt.Errorf("delete %s: %w", s.VolumeID(name), err)
	}
	return nil
}

// ReferencedVolumes returns every volume id named by the config of a qemu
// VM in the cluster, running or stopped; any unreadable config is an error
// so callers never treat an unknown reference as absent.
func (s *ISOStore) ReferencedVolumes(ctx context.Context) (map[string]bool, error) {
	client, err := s.client(ctx, defaultProbeTimeout)
	if err != nil {
		return nil, err
	}
	var resources proxmox.ClusterResources
	if err := client.Get(ctx, "/cluster/resources?type=vm", &resources); err != nil {
		return nil, fmt.Errorf("list cluster vms: %w", err)
	}
	refs := make(map[string]bool)
	for _, r := range resources {
		if r == nil {
			return nil, fmt.Errorf("list cluster vms: null resource in response")
		}
		if r.Type != resourceTypeQEMU {
			continue
		}
		var vmConfig map[string]any
		if err := client.Get(ctx, fmt.Sprintf("/nodes/%s/qemu/%d/config", r.Node, r.VMID), &vmConfig); err != nil {
			return nil, fmt.Errorf("read vm %d config: %w", r.VMID, err)
		}
		for _, value := range vmConfig {
			str, ok := value.(string)
			if !ok {
				continue
			}
			for seg := range strings.SplitSeq(str, ",") {
				seg = strings.TrimPrefix(strings.TrimSpace(seg), "file=")
				if seg != "" && !strings.Contains(seg, "=") {
					refs[seg] = true
				}
			}
		}
	}
	return refs, nil
}

// RemoveUnreferenced deletes the ISOs on node whose names satisfy match and
// that no VM config references, returning the removed and in-use names. It
// removes nothing when the VM references cannot be read; per-volume delete
// failures are joined into err after the remaining volumes are tried.
func (s *ISOStore) RemoveUnreferenced(ctx context.Context, node string, match func(name string) bool) (removed, inUse []string, err error) {
	volumes, err := s.Volumes(ctx, node)
	if err != nil {
		return nil, nil, err
	}
	var candidates []string
	for name := range volumes {
		if match(name) {
			candidates = append(candidates, name)
		}
	}
	if len(candidates) == 0 {
		return nil, nil, nil
	}
	slices.Sort(candidates)
	refs, err := s.ReferencedVolumes(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("check vm references: %w", err)
	}
	var errs []error
	for _, name := range candidates {
		if refs[s.VolumeID(name)] {
			inUse = append(inUse, name)
			continue
		}
		if err := s.Remove(ctx, node, name); err != nil {
			errs = append(errs, err)
			continue
		}
		removed = append(removed, name)
	}
	return removed, inUse, errors.Join(errs...)
}
