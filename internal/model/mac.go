package model

import (
	"crypto/sha256"
	"fmt"

	"github.com/qxtaiba/okdctl/internal/config"
)

// MAC returns the locally administered unicast address 02:h1:h2:v2:v1:v0,
// where h1 h2 are the first two SHA-256 bytes of infraID and v2 v1 v0 the
// VMID big-endian.
func MAC(infraID string, vmid int) string {
	sum := sha256.Sum256([]byte(infraID))
	v := fmt.Sprintf("%06x", vmid&config.MaxVMID)
	return fmt.Sprintf("02:%02x:%02x:%s:%s:%s", sum[0], sum[1], v[0:2], v[2:4], v[4:6])
}

// DiskSerial returns the serial okd<vmid>d<n> of a node's n-th disk.
func DiskSerial(vmid, n int) string {
	return fmt.Sprintf("okd%dd%d", vmid, n)
}
