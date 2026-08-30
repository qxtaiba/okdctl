package logutil

// Log attr key glossary — one concept, one key, so log queries can join
// records on them. Pick from here before minting a new key.
//
//	err            error value, passed structured ("err", err) — never err.Error()
//	path           filesystem path (file or directory), absolute where known
//	file           base filename of an artifact (ISO, tarball, binary download)
//	dir            directory when the record is about the directory itself
//	url            full URL of a fetch or probe target
//	cluster        bare cluster name (cfg.Cluster.Name); never the FQDN
//	domain         cluster base domain, carried separately when the FQDN matters
//	node           cluster (guest) node name
//	host_node      Proxmox host node name
//	vmid           Proxmox VM ID
//	vip            virtual IP (kube-vip or ingress)
//	role           node role, passed as string(role)
//	tool           name of an installed executable
//	version        bare version string, no label prefix inside the value
//	cmd, argc      executor: command name and argument count (never argv)
//	exit           subprocess exit code
//	duration       elapsed time of a completed span (time.Duration)
//	count          integer cardinality of a set
//	files          list of filenames, passed as a []string slice
//	failed_steps   names of failed teardown/cleanup steps, []string slice
//	skipped_steps  names of skipped steps, []string slice
//	controllers    ingress controller names, []string slice
//	approved       CSRs approved this tick (delta)
//	csrs_approved  CSRs approved so far (running total)
//	expires        certificate NotAfter date (all branches, expired included)
//	days_remaining days until expiry; negative once expired
//	expected       the value this build supports, next to the offending value
//	marker_age     deploy-state marker age as a rounded time.Duration string
//	stale          boolean judgment that a marker/resource is likely abandoned
//	phase, run_id  deployment phase and run correlation ID
//	addon          name of the addon
//	backend        firewall backend name
//	stderr         subprocess stderr, always wrapped in RedactableStderr
