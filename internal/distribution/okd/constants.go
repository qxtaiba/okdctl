package okd

const (
	DefaultHAProxyConfigPath = "/etc/haproxy/haproxy.cfg"
	DefaultHTTPServerRoot    = "/var/www/html"
)

// Minimum resource requirements for OKD control plane nodes.
const (
	MinControlPlaneMemoryMB = 8192
	MinControlPlaneCPUs     = 4
	MinControlPlaneDiskGB   = 50
)
