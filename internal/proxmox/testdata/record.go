//go:build pveproof

// Command record refreshes the fake-PVE fixtures in this directory from a
// live Proxmox VE host, keeping every identifier the existing fixtures pin.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/httputil"
	"github.com/qxtaiba/okdctl/internal/proxmox"
)

// The recorded VM mirrors nodes_pve_qemu_101_config.json, so the refreshed
// fixture differs from the old one only where PVE's encoding does.
const (
	fixtureVMID  = 101
	fixtureMAC   = "BC:24:11:AA:BB:CC"
	configFile   = "nodes_pve_qemu_101_config.json"
	proofTag     = "okdctl-proof"
	proofMAC     = "02:4F:4B:44:26:AC"
	resourceWait = 90 * time.Second
	httpTimeout  = 60 * time.Second
)

var keptFixtures = map[string]string{
	"access_permissions.json":          "the live body is the production token's whole ACL; encoding verified",
	"cluster_ha_resources_vm_101.json": "recording it would register an HA resource on the production host",
	"nodes_pve_qemu_101_pending.json":  "queued changes need a running VM; the rows exercise pending and delete",
}

type settings struct {
	endpoint   string
	token      []byte
	insecure   bool
	node       string
	storage    string
	isoStorage string
	bridge     string
	pool       string
	vmid       int
}

func loadSettings() (*settings, error) {
	s := &settings{
		endpoint:   os.Getenv("PVEPROOF_ENDPOINT"),
		token:      []byte(os.Getenv("PVEPROOF_TOKEN")),
		insecure:   true,
		node:       envOr("PVEPROOF_NODE", "pve"),
		storage:    envOr("PVEPROOF_STORAGE", "local-lvm"),
		isoStorage: envOr("PVEPROOF_ISO_STORAGE", "local"),
		bridge:     envOr("PVEPROOF_BRIDGE", "vmbr0"),
		pool:       os.Getenv("PVEPROOF_POOL"),
		vmid:       9900,
	}
	if s.endpoint == "" || len(s.token) == 0 {
		return nil, errors.New("PVEPROOF_ENDPOINT and PVEPROOF_TOKEN are required")
	}
	if v := os.Getenv("PVEPROOF_INSECURE"); v != "" {
		insecure, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("parse PVEPROOF_INSECURE: %w", err)
		}
		s.insecure = insecure
	}
	if v := os.Getenv("PVEPROOF_VMID"); v != "" {
		vmid, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("parse PVEPROOF_VMID: %w", err)
		}
		s.vmid = vmid
	}
	return s, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// merge builds a fixture from its old payload and the sanitised live one; a
// nil result keeps the file on disk.
type merge func(old, live any) (any, error)

type fixture struct {
	file  string
	path  string
	merge merge
}

type recorder struct {
	s         *settings
	d         proxmox.Driver
	http      *http.Client
	base      string
	auth      string
	dir       string
	old       map[string]any
	repl      *strings.Replacer
	forbidden []string
}

func main() {
	os.Exit(realMain())
}

func realMain() int {
	// The recorder is a process entry point, so the signal-watched root context is created here.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "record:", err)
		return 1
	}
	return 0
}

func run(ctx context.Context) (err error) {
	dir := flag.String("dir", "internal/proxmox/testdata", "fixture directory to refresh")
	flag.Parse()
	s, err := loadSettings()
	if err != nil {
		return err
	}
	d, err := proxmox.New(s.endpoint, s.token, s.insecure)
	if err != nil {
		return err
	}
	r := &recorder{
		s:    s,
		d:    d,
		http: httputil.NewOptionalInsecure(s.insecure, httpTimeout),
		base: proxmox.APIBaseURL(s.endpoint),
		auth: "PVEAPIToken=" + string(s.token),
		dir:  *dir,
	}
	if err := r.loadOld(); err != nil {
		return err
	}
	task, err := r.create(ctx)
	if err != nil {
		return err
	}
	// Armed as soon as PVE accepted the POST, so a failed wait or Ctrl-C still destroys the VM.
	defer func() { err = errors.Join(err, r.destroy(ctx, task)) }()
	if err := r.d.WaitTask(ctx, task); err != nil {
		return fmt.Errorf("create vm %d: %w", r.s.vmid, err)
	}
	fmt.Printf("created vm %d on %s\n", r.s.vmid, r.s.node)
	if err := r.awaitResource(ctx); err != nil {
		return err
	}
	return r.record(ctx, task.UPID)
}

func (r *recorder) fixtures(upid string) []fixture {
	node, vmid := url.PathEscape(r.s.node), strconv.Itoa(r.s.vmid)
	return []fixture{
		{"version.json", "/version", mergeObject},
		{"nodes.json", "/nodes", mergeRows("node", fakeFingerprint)},
		{"nodes_pve_status.json", "/nodes/" + node + "/status", mergeObject},
		{"nodes_pve_storage.json", "/nodes/" + node + "/storage", mergeRows("storage", nil)},
		{"nodes_pve_network_any_bridge.json", "/nodes/" + node + "/network?type=any_bridge", mergeRows("iface", fakeAddresses)},
		{"cluster_resources_vm.json", "/cluster/resources?type=vm", mergeResources},
		{"nodes_pve_storage_local_content_iso.json", "/nodes/" + node + "/storage/" + r.s.isoStorage + "/content?content=iso", mergeRows("volid", nil)},
		{"access_permissions.json", "/access/permissions", checkPermissions},
		{configFile, "/nodes/" + node + "/qemu/" + vmid + "/config", pinKeys("digest", "smbios1", "vmgenid", "ide2", "boot")},
		{"nodes_pve_qemu_101_status_current.json", "/nodes/" + node + "/qemu/" + vmid + "/status/current", pinKeys("status", "qmpstatus", "ha", "uptime", "pid")},
		{"nodes_pve_tasks_qmstart_status.json", "/nodes/" + node + "/tasks/" + url.PathEscape(upid) + "/status", pinKeys("upid", "user", "type", "id")},
	}
}

// loadOld reads every fixture the recorder refreshes and derives the
// replacements that map the live VM onto the fixture's identifiers.
func (r *recorder) loadOld() error {
	r.old = map[string]any{}
	for _, f := range r.fixtures("") {
		body, err := os.ReadFile(filepath.Join(r.dir, f.file))
		if err != nil {
			return fmt.Errorf("read fixture: %w", err)
		}
		var envelope struct {
			Data any `json:"data"`
		}
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		if err := dec.Decode(&envelope); err != nil {
			return fmt.Errorf("decode %s: %w", f.file, err)
		}
		r.old[f.file] = envelope.Data
	}
	cfg, err := object(r.old[configFile])
	if err != nil {
		return fmt.Errorf("%s: %w", configFile, err)
	}
	oldName, oldTag := fmt.Sprint(cfg["name"]), fmt.Sprint(cfg["tags"])
	vmid, proofName := strconv.Itoa(r.s.vmid), r.proofName()
	pairs := []string{
		"vm-" + vmid + "-", "vm-" + strconv.Itoa(fixtureVMID) + "-",
		"qemu/" + vmid, "qemu/" + strconv.Itoa(fixtureVMID),
		":" + vmid + ":", ":" + strconv.Itoa(fixtureVMID) + ":",
		proofName, oldName,
		proofTag, oldTag,
		proofMAC, fixtureMAC,
		strings.ToLower(proofMAC), strings.ToLower(fixtureMAC),
	}
	if r.s.node != "pve" {
		pairs = append(pairs, "node/"+r.s.node, "node/pve", "UPID:"+r.s.node+":", "UPID:pve:")
	}
	if r.s.storage != "local-lvm" {
		pairs = append(pairs, r.s.storage+":", "local-lvm:")
	}
	if r.s.isoStorage != "local" {
		pairs = append(pairs, r.s.isoStorage+":iso/", "local:iso/")
	}
	r.repl = strings.NewReplacer(pairs...)
	id, secret, _ := strings.Cut(string(r.s.token), "=")
	_, tokenID, _ := strings.Cut(id, "!")
	host, err := url.Parse(r.base)
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}
	r.forbidden = []string{id, tokenID, secret, "PVEAPIToken", host.Hostname(), proofName, proofTag, proofMAC}
	return nil
}

func (r *recorder) proofName() string {
	return "okdctl-proof-" + strconv.Itoa(r.s.vmid)
}

// create posts the VM and returns its task unawaited; the listing lags, so
// the per-VM status endpoint is the authoritative absence check.
func (r *recorder) create(ctx context.Context) (proxmox.Task, error) {
	vms, err := r.d.ListVMs(ctx)
	if err != nil {
		return proxmox.Task{}, fmt.Errorf("list vms: %w", err)
	}
	if i := slices.IndexFunc(vms, func(vm proxmox.VMSummary) bool { return vm.VMID == r.s.vmid }); i >= 0 {
		return proxmox.Task{}, fmt.Errorf("vmid %d is taken by %q on %s; refusing to touch it", r.s.vmid, vms[i].Name, vms[i].Node)
	}
	ref := proxmox.VMRef{Node: r.s.node, VMID: r.s.vmid}
	st, err := r.d.Status(ctx, ref)
	switch {
	case err == nil:
		return proxmox.Task{}, fmt.Errorf("vmid %d exists (%s); refusing to touch it", r.s.vmid, st.Status)
	case !errors.Is(err, proxmox.ErrNotFound):
		return proxmox.Task{}, fmt.Errorf("status vm %d: %w", r.s.vmid, err)
	}
	spec := proxmox.VMSpec{
		VMID:       r.s.vmid,
		Name:       r.proofName(),
		CPUType:    "host",
		Cores:      4,
		MemoryMiB:  16384,
		Disks:      []proxmox.DiskSpec{{Slot: "scsi0", Storage: r.s.storage, SizeGiB: 120, Serial: "master-0-root"}},
		EFIStorage: r.s.storage,
		MAC:        proofMAC,
		Bridge:     r.s.bridge,
		Tag:        proofTag,
		Pool:       r.s.pool,
	}
	task, err := r.d.CreateVM(ctx, r.s.node, spec)
	if err != nil {
		return proxmox.Task{}, fmt.Errorf("create vm %d: %w", r.s.vmid, err)
	}
	if task.UPID == "" {
		return proxmox.Task{}, fmt.Errorf("create vm %d: already carries tag %s", r.s.vmid, proofTag)
	}
	return task, nil
}

// destroy runs even after ctx is cancelled, so it carries its own deadline;
// it awaits the create task first because PVE holds a create lock until then.
func (r *recorder) destroy(ctx context.Context, created proxmox.Task) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	_ = r.d.WaitTask(ctx, created)
	ref := proxmox.VMRef{Node: r.s.node, VMID: r.s.vmid}
	if _, err := r.d.Status(ctx, ref); errors.Is(err, proxmox.ErrNotFound) {
		return nil
	}
	task, err := r.d.DeleteVM(ctx, ref)
	if err == nil {
		err = r.d.WaitTask(ctx, task)
	}
	if err != nil {
		return fmt.Errorf("destroy vm %d (left on host): %w", r.s.vmid, err)
	}
	fmt.Printf("destroyed vm %d\n", r.s.vmid)
	return nil
}

// pvestatd publishes /cluster/resources on its own tick, so a fresh VM shows up seconds later.
func (r *recorder) awaitResource(ctx context.Context) error {
	deadline := time.Now().Add(resourceWait)
	for {
		vms, err := r.d.ListVMs(ctx)
		if err != nil {
			return fmt.Errorf("list vms: %w", err)
		}
		if slices.ContainsFunc(vms, func(vm proxmox.VMSummary) bool { return vm.VMID == r.s.vmid }) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("vm %d absent from /cluster/resources after %s", r.s.vmid, resourceWait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (r *recorder) record(ctx context.Context, upid string) error {
	for _, f := range r.fixtures(upid) {
		live, err := r.get(ctx, f.path)
		if err != nil {
			return err
		}
		data, err := f.merge(r.old[f.file], r.sanitize(live))
		if err != nil {
			return fmt.Errorf("%s: %w", f.file, err)
		}
		if data == nil {
			fmt.Printf("kept      %s: %s\n", f.file, keptFixtures[f.file])
			continue
		}
		if err := r.write(f.file, data); err != nil {
			return err
		}
		fmt.Printf("refreshed %s\n", f.file)
	}
	for _, file := range slices.Sorted(maps.Keys(keptFixtures)) {
		if _, fetched := r.old[file]; !fetched {
			fmt.Printf("kept      %s: %s\n", file, keptFixtures[file])
		}
	}
	return nil
}

func (r *recorder) get(ctx context.Context, path string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}
	req.Header.Set("Authorization", r.auth)
	res, err := r.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get %s: %s", path, res.Status)
	}
	var envelope struct {
		Data any `json:"data"`
	}
	dec := json.NewDecoder(res.Body)
	dec.UseNumber()
	if err := dec.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("get %s: decode: %w", path, err)
	}
	return envelope.Data, nil
}

func (r *recorder) sanitize(v any) any {
	switch v := v.(type) {
	case string:
		return r.repl.Replace(v)
	case []any:
		for i := range v {
			v[i] = r.sanitize(v[i])
		}
	case map[string]any:
		for k, val := range v {
			switch {
			case k == "vmid" && fmt.Sprint(val) == strconv.Itoa(r.s.vmid):
				v[k] = json.Number(strconv.Itoa(fixtureVMID))
			case k == "node" && val == r.s.node:
				v[k] = "pve"
			case k == "tokenid": // PVE 8+ reports the bare token id beside user
				v[k] = "tok"
			default:
				v[k] = r.sanitize(val)
			}
		}
	}
	return v
}

func (r *recorder) write(file string, data any) error {
	body, err := encode(data)
	if err != nil {
		return fmt.Errorf("encode %s: %w", file, err)
	}
	for _, s := range r.forbidden {
		if s != "" && bytes.Contains(body, []byte(s)) {
			return fmt.Errorf("%s: refusing to write a fixture carrying a live identifier", file)
		}
	}
	if err := os.WriteFile(filepath.Join(r.dir, file), body, 0o644); err != nil { //nolint:gosec // G306: fixtures are world-readable test data
		return fmt.Errorf("write fixture: %w", err)
	}
	return nil
}

func encode(data any) ([]byte, error) {
	rows, ok := data.([]any)
	if !ok {
		body, err := json.Marshal(map[string]any{"data": data})
		return append(body, '\n'), err
	}
	var buf bytes.Buffer
	buf.WriteString("{\"data\":[\n")
	for i, row := range rows {
		body, err := json.Marshal(row)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			buf.WriteString(",\n")
		}
		buf.Write(body)
	}
	buf.WriteString("\n]}\n")
	return buf.Bytes(), nil
}

// mergeOld returns live's field set with every value old carries taken from old.
func mergeOld(old, live map[string]any) map[string]any {
	out := maps.Clone(live)
	for k, v := range old {
		if o, ok := v.(map[string]any); ok {
			if l, ok := out[k].(map[string]any); ok {
				out[k] = mergeOld(o, l)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func mergeObject(old, live any) (any, error) {
	o, err := object(old)
	if err != nil {
		return nil, err
	}
	l, err := object(live)
	if err != nil {
		return nil, err
	}
	return mergeOld(o, l), nil
}

// mergeRows rebuilds every old row on the live row sharing its key (the
// first live row when none does); fix then scrubs host-specific values.
func mergeRows(key string, fix func(map[string]any)) merge {
	return func(old, live any) (any, error) {
		oldRows, liveRows, err := rowPair(old, live)
		if err != nil || len(liveRows) == 0 {
			return nil, err
		}
		out := make([]any, 0, len(oldRows))
		for _, o := range oldRows {
			tmpl := liveRows[0]
			if i := slices.IndexFunc(liveRows, func(l map[string]any) bool { return l[key] == o[key] }); i >= 0 {
				tmpl = liveRows[i]
			}
			row := mergeOld(o, tmpl)
			if fix != nil {
				fix(row)
			}
			out = append(out, row)
		}
		return out, nil
	}
}

// mergeResources uses the recorded VM's row as the field template for every
// old guest; its tags and pool are dropped so old rows without them stay bare.
func mergeResources(old, live any) (any, error) {
	oldRows, liveRows, err := rowPair(old, live)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(liveRows, func(l map[string]any) bool { return fmt.Sprint(l["vmid"]) == strconv.Itoa(fixtureVMID) })
	if i < 0 {
		return nil, errors.New("recorded vm missing from the listing")
	}
	tmpl := maps.Clone(liveRows[i])
	delete(tmpl, "tags")
	delete(tmpl, "pool")
	out := make([]any, 0, len(oldRows))
	for _, o := range oldRows {
		out = append(out, mergeOld(o, tmpl))
	}
	return out, nil
}

// pinKeys takes the live object with the named keys copied from the old one.
func pinKeys(keys ...string) merge {
	return func(old, live any) (any, error) {
		o, err := object(old)
		if err != nil {
			return nil, err
		}
		l, err := object(live)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if v, ok := o[k]; ok {
				l[k] = v
			}
		}
		return l, nil
	}
}

// checkPermissions verifies the {path: {privilege: 1}} encoding and keeps the file.
func checkPermissions(_, live any) (any, error) {
	perms, err := object(live)
	if err != nil {
		return nil, err
	}
	for path, privs := range perms {
		m, err := object(privs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for priv, v := range m {
			if _, ok := v.(json.Number); !ok {
				return nil, fmt.Errorf("%s %s: got %T, want number", path, priv, v)
			}
		}
	}
	return nil, nil
}

func fakeFingerprint(row map[string]any) {
	if _, ok := row["ssl_fingerprint"]; ok {
		row["ssl_fingerprint"] = strings.Repeat("AB:", 31) + "AB"
	}
}

func fakeAddresses(row map[string]any) {
	docs := map[string]string{
		"cidr": "192.0.2.10/24", "address": "192.0.2.10", "gateway": "192.0.2.1",
		"cidr6": "2001:db8::10/64", "address6": "2001:db8::10", "gateway6": "2001:db8::1",
	}
	for k, v := range docs {
		if _, ok := row[k]; ok {
			row[k] = v
		}
	}
	delete(row, "comments")
}

func rowPair(old, live any) (oldRows, liveRows []map[string]any, err error) {
	if oldRows, err = rows(old); err != nil {
		return nil, nil, fmt.Errorf("old: %w", err)
	}
	if liveRows, err = rows(live); err != nil {
		return nil, nil, fmt.Errorf("live: %w", err)
	}
	return oldRows, liveRows, nil
}

func rows(v any) ([]map[string]any, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("want an array, got %T", v)
	}
	out := make([]map[string]any, 0, len(list))
	for _, e := range list {
		m, err := object(e)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func object(v any) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("want an object, got %T", v)
	}
	return m, nil
}
