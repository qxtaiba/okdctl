package proxmox

import (
	"cmp"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	diskSlot  = regexp.MustCompile(`^(scsi|virtio|sata|ide)\d+$`)
	netSlot   = regexp.MustCompile(`^net(\d+)$`)
	nicModels = map[string]bool{
		"virtio": true, "e1000": true, "e1000e": true, "e1000-82540em": true, "e1000-82544gc": true,
		"e1000-82545em": true, "rtl8139": true, "vmxnet3": true, "i82551": true, "i82557b": true,
		"i82559er": true, "ne2k_isa": true, "ne2k_pci": true, "pcnet": true,
	}
	sizeUnits = map[byte]int64{'K': 1 << 10, 'M': 1 << 20, 'G': 1 << 30, 'T': 1 << 40}
)

// ParseVMConfig normalises the data object of GET /nodes/{node}/qemu/{vmid}/config.
func ParseVMConfig(raw map[string]any) (VMConfig, error) {
	r := cfgReader{raw: raw}
	c := VMConfig{
		Name:      r.str("name"),
		Cores:     cmp.Or(r.int("cores"), 1),
		Sockets:   cmp.Or(r.int("sockets"), 1),
		CPUType:   parseCPU(r.str("cpu")),
		MemoryMiB: cmp.Or(r.memory(), 512),
		Balloon:   r.int("balloon"),
		BIOS:      r.str("bios"),
		Machine:   parseMachine(r.str("machine")),
		SCSIHW:    r.str("scsihw"),
		OSType:    r.str("ostype"),
		OnBoot:    r.bool("onboot"),
		BootOrder: parseBoot(r.str("boot")),
		Tags:      splitList(r.str("tags"), ';'),
		Digest:    r.str("digest"),
	}
	if _, ok := raw["balloon"]; !ok {
		c.Balloon = c.MemoryMiB // PVE balloons to the full memory unless told otherwise
	}
	c.AgentEnabled = parseAgent(r.str("agent"))
	if err := c.setNIC(r.str("net0")); err != nil {
		r.fail("net0", err)
	}
	c.MACs = collectMACs(r)
	if err := c.setEFI(r.str("efidisk0")); err != nil {
		r.fail("efidisk0", err)
	}
	for key := range raw {
		if !diskSlot.MatchString(key) {
			continue
		}
		if err := c.setDisk(key, r.str(key)); err != nil {
			r.fail(key, err)
		}
	}
	if r.err != nil {
		return VMConfig{}, r.err
	}
	return c, nil
}

// cfgReader reads typed keys from the raw config, keeping the first failure.
type cfgReader struct {
	raw map[string]any
	err error
}

func (r *cfgReader) fail(key string, err error) {
	if r.err == nil {
		r.err = fmt.Errorf("parse %s: %w", key, err)
	}
}

func (r *cfgReader) str(key string) string { return scalar(r.raw[key]) }

func (r *cfgReader) int(key string) int {
	s := r.str(key)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		r.fail(key, err)
	}
	return n
}

func (r *cfgReader) bool(key string) bool {
	s := r.str(key)
	if s == "" {
		return false
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		r.fail(key, err)
	}
	return b
}

// memory accepts the three shapes PVE emits: a number, a quoted number and
// the PVE 8.2+ property string current=<MiB>.
func (r *cfgReader) memory() int {
	s := r.str("memory")
	if s == "" {
		return 0
	}
	bare, kv := propString(s)
	if v, ok := kv["current"]; ok {
		bare = v
	}
	n, err := strconv.Atoi(bare)
	if err != nil {
		r.fail("memory", err)
	}
	return n
}

// scalar renders a decoded JSON value the way PVE's config file spells it.
func scalar(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	case bool:
		return boolFlag(v)
	default:
		return fmt.Sprint(v)
	}
}

// propString splits a PVE property string into its bare (default-key) value
// and its key=value pairs.
func propString(s string) (bare string, kv map[string]string) {
	kv = map[string]string{}
	for part := range strings.SplitSeq(s, ",") {
		if k, v, ok := strings.Cut(part, "="); ok {
			kv[k] = v
		} else if bare == "" {
			bare = part
		}
	}
	return bare, kv
}

func parseCPU(s string) string {
	bare, kv := propString(s)
	if t, ok := kv["cputype"]; ok {
		return t
	}
	return bare
}

// parseMachine reduces q35, pc-q35-9.0, type=q35 and pc-i440fx-9.0 to the
// bare machine type PVE accepts on create.
func parseMachine(s string) string {
	bare, kv := propString(s)
	if t, ok := kv["type"]; ok {
		bare = t
	}
	if rest, ok := strings.CutPrefix(bare, "pc-"); ok {
		t, _, _ := strings.Cut(rest, "-")
		if t == "i440fx" {
			return "pc"
		}
		return t
	}
	return bare
}

func parseBoot(s string) []string {
	_, kv := propString(s)
	return splitList(kv["order"], ';')
}

func parseAgent(s string) bool {
	bare, kv := propString(s)
	if v, ok := kv["enabled"]; ok {
		bare = v
	}
	return bare == "1"
}

func (c *VMConfig) setNIC(s string) error {
	if s == "" {
		return nil
	}
	_, kv := propString(s)
	for k := range kv {
		if nicModels[k] {
			c.NICModel = k
		}
	}
	if m, ok := kv["model"]; ok {
		c.NICModel = m
	}
	c.MAC = nicMAC(kv)
	c.Bridge = kv["bridge"]
	if tag, ok := kv["tag"]; ok {
		vlan, err := strconv.Atoi(tag)
		if err != nil {
			return err
		}
		c.VLAN = vlan
	}
	return nil
}

func (c *VMConfig) setEFI(s string) error {
	if s == "" {
		return nil
	}
	volid, kv := propString(s)
	storage, _, _ := strings.Cut(volid, ":")
	keys, err := strconv.ParseBool(cmp.Or(kv["pre-enrolled-keys"], "0"))
	if err != nil {
		return err
	}
	c.EFI = &EFIConfig{Storage: storage, VolID: volid, EFIType: kv["efitype"], PreEnrolledKeys: keys}
	return nil
}

func (c *VMConfig) setDisk(slot, s string) error {
	volid, kv := propString(s)
	if kv["media"] == "cdrom" {
		if slot == cdromSlot && volid != "none" {
			c.CDROM = volid
		}
		return nil
	}
	bytes, err := ParseSize(kv["size"])
	if err != nil {
		return err
	}
	storage, _, _ := strings.Cut(volid, ":")
	if c.Disks == nil {
		c.Disks = map[string]DiskConfig{}
	}
	c.Disks[slot] = DiskConfig{
		Storage:  storage,
		VolID:    volid,
		SizeGiB:  int(bytes >> 30),
		Serial:   kv["serial"],
		IOThread: kv["iothread"] == "1",
		Discard:  kv["discard"] == "on",
		Cache:    kv["cache"],
		AIO:      kv["aio"],
	}
	return nil
}

// ParseSize reads PVE's size notation, as Change.Want and Change.Have
// spell disk sizes: bytes, or a number with a K/M/G/T binary suffix.
func ParseSize(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	unit := int64(1)
	if mul, ok := sizeUnits[s[len(s)-1]&^0x20]; ok {
		unit = mul
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("size: %w", err)
	}
	return n * unit, nil
}

// Diff lists where c departs from spec; a key the spec leaves unset is never
// compared, while one the VM lacks is reported with an empty Have.
func (c VMConfig) Diff(spec VMSpec) []Change { //nolint:gocritic // hugeParam: the Driver API passes configs and specs by value
	var d differ
	d.str("name", spec.Name, c.Name, false)
	d.int("cores", spec.Cores, c.Cores, true)
	d.int("sockets", vmSockets, c.Sockets, true)
	d.int("memory", spec.MemoryMiB, c.MemoryMiB, true)
	d.str("cpu", spec.CPUType, c.CPUType, true)
	d.str("machine", vmMachine, c.Machine, true)
	d.str("bios", vmBIOS, c.BIOS, true)
	d.str("scsihw", vmSCSIHW, c.SCSIHW, true)
	d.str("ostype", vmOSType, c.OSType, true)
	d.add("onboot", "1", boolFlag(c.OnBoot), false)
	d.int("balloon", vmBalloon, c.Balloon, false)
	d.add("agent", "enabled=1", "enabled="+boolFlag(c.AgentEnabled), true)
	have := ""
	if len(c.BootOrder) > 0 {
		have = "order=" + strings.Join(c.BootOrder, ";")
	}
	d.add("boot", spec.bootOrder(), have, true)
	d.tags(spec.Tag, c.Tags)
	if c.NICModel == "" {
		d.add("net0", spec.net0(), "", true)
	} else {
		d.add("net0.model", vmNICModel, c.NICModel, true)
		if !strings.EqualFold(spec.MAC, c.MAC) {
			d.add("net0.mac", spec.MAC, c.MAC, true)
		}
		d.add("net0.bridge", spec.Bridge, c.Bridge, false)
		d.add("net0.vlan", strconv.Itoa(spec.VLAN), strconv.Itoa(c.VLAN), false)
	}
	efi := ""
	if c.EFI != nil {
		efi = c.EFI.Storage
	}
	d.str("efidisk0.storage", spec.EFIStorage, efi, false)
	for _, want := range spec.Disks {
		d.disk(want, c.Disks)
	}
	d.add(cdromSlot, spec.ISOVolID, c.CDROM, false)
	return d.changes
}

type differ struct {
	changes []Change
}

func (d *differ) add(field, want, have string, powerCycle bool) {
	if want != have {
		d.changes = append(d.changes, Change{Field: field, Want: want, Have: have, NeedsPowerCycle: powerCycle})
	}
}

func (d *differ) str(field, want, have string, powerCycle bool) {
	if want != "" {
		d.add(field, want, have, powerCycle)
	}
}

func (d *differ) int(field string, want, have int, powerCycle bool) {
	d.add(field, strconv.Itoa(want), strconv.Itoa(have), powerCycle)
}

// tags requires the spec's tag among the VM's, which may carry others the
// operator added; Want is the union so the engine can set it verbatim.
func (d *differ) tags(want string, have []string) {
	if want == "" || slices.ContainsFunc(have, func(t string) bool { return strings.EqualFold(t, want) }) {
		return
	}
	union := append([]string{strings.ToLower(want)}, have...)
	for i, t := range union {
		union[i] = strings.ToLower(t)
	}
	slices.Sort(union)
	d.add("tags", strings.Join(slices.Compact(union), ";"), strings.Join(have, ";"), false)
}

func (d *differ) disk(want DiskSpec, have map[string]DiskConfig) {
	field := "disks." + want.Slot
	got, ok := have[want.Slot]
	if !ok {
		d.add(field, want.Storage+":"+gib(want.SizeGiB), "", false)
		return
	}
	d.add(field+".storage", want.Storage, got.Storage, false)
	d.add(field+".size", gib(want.SizeGiB), gib(got.SizeGiB), false)
	d.add(field+".serial", want.Serial, got.Serial, true)
	d.add(field+".iothread", "1", boolFlag(got.IOThread), true)
	d.add(field+".discard", "on", discardFlag(got.Discard), true)
}

func discardFlag(on bool) string {
	if on {
		return "on"
	}
	return "ignore"
}

func gib(n int) string { return strconv.Itoa(n) + "G" }

func nicMAC(kv map[string]string) string {
	if mac, ok := kv["macaddr"]; ok {
		return mac
	}
	for k, v := range kv {
		if nicModels[k] {
			return v
		}
	}
	return ""
}

func collectMACs(r cfgReader) []string {
	type slot struct {
		n   int
		mac string
	}
	var slots []slot
	for key := range r.raw {
		m := netSlot.FindStringSubmatch(key)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		_, kv := propString(r.str(key))
		if mac := nicMAC(kv); mac != "" {
			slots = append(slots, slot{n, mac})
		}
	}
	if len(slots) == 0 {
		return nil
	}
	slices.SortFunc(slots, func(a, b slot) int { return cmp.Compare(a.n, b.n) })
	macs := make([]string, 0, len(slots))
	for _, s := range slots {
		macs = append(macs, s.mac)
	}
	return macs
}
