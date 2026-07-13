package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
)

// CephHealth summarizes rook-ceph fitness for a node op. Applicable is false
// when no rook-ceph toolbox is present (not every cluster runs Ceph) — callers
// skip the gate. Healthy reflects STRUCTURAL health only: mons in quorum, OSDs
// up/in, PGs active+clean. Benign non-structural warnings (e.g. BlueStore slow
// ops) are intentionally ignored, so a cluster whose steady state is
// HEALTH_WARN still gates cleanly.
type CephHealth struct {
	Applicable   bool
	Healthy      bool
	Reason       string
	MonsInQuorum int
	MonsTotal    int
	OSDsUp       int
	OSDsIn       int
	OSDsTotal    int
	DegradedPGs  int
}

// CephHealthy runs `ceph -s -f json` in the rook-ceph toolbox pod and evaluates
// structural health. When no toolbox pod exists it returns Applicable=false so
// non-Ceph clusters skip the gate rather than failing.
func (c *Client) CephHealthy(ctx context.Context) (CephHealth, error) {
	pods, err := c.PodsForSelector(ctx, "", "app=rook-ceph-tools")
	if err != nil {
		return CephHealth{}, &errtypes.ClusterError{Msg: "ceph: find toolbox pod", Err: err}
	}
	tools := firstScheduledPod(pods)
	if tools == nil {
		return CephHealth{Applicable: false}, nil
	}

	raw, err := c.execCephStatus(ctx, tools.Namespace, tools.Name)
	if err != nil {
		return CephHealth{}, err
	}
	return parseCephHealth(raw)
}

func firstScheduledPod(pods []PodPlacement) *PodPlacement {
	for i := range pods {
		if pods[i].NodeName != "" {
			return &pods[i]
		}
	}
	return nil
}

func (c *Client) execCephStatus(ctx context.Context, namespace, pod string) ([]byte, error) {
	result, err := c.runOutput(ctx, "exec", "-n", namespace, pod, "--", "ceph", "-s", "-f", "json")
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, &errtypes.ClusterError{
			Msg: "ceph: exec ceph -s",
			Err: executor.NewExitError(ctx, c.CLI+" exec rook-ceph-tools", result.ExitCode, strings.TrimSpace(result.Stderr)),
		}
	}
	if result.Truncated {
		return nil, &errtypes.ClusterError{Msg: "ceph: status output truncated; cannot parse"}
	}
	return []byte(result.Stdout), nil
}

// cephStatus is the subset of `ceph -s -f json` the structural check reads.
type cephStatus struct {
	QuorumNames []string `json:"quorum_names"`
	Monmap      struct {
		Mons    []struct{} `json:"mons"`
		NumMons int        `json:"num_mons"`
	} `json:"monmap"`
	Osdmap cephOsdmap `json:"osdmap"`
	Pgmap  struct {
		NumPgs     int `json:"num_pgs"`
		PgsByState []struct {
			StateName string `json:"state_name"`
			Count     int    `json:"count"`
		} `json:"pgs_by_state"`
	} `json:"pgmap"`
}

// cephOsdmap handles both the flat (Reef+) and nested (older) osdmap shapes:
// newer ceph reports counts on osdmap directly, older nests them under
// osdmap.osdmap.
type cephOsdmap struct {
	NumOsds   int         `json:"num_osds"`
	NumUpOsds int         `json:"num_up_osds"`
	NumInOsds int         `json:"num_in_osds"`
	Nested    *cephOsdmap `json:"osdmap"`
}

func (o cephOsdmap) resolve() cephOsdmap {
	if o.NumOsds == 0 && o.Nested != nil {
		return *o.Nested
	}
	return o
}

// parseCephHealth applies the structural rule: healthy iff every mon is in
// quorum, every OSD is up and in, and every PG is active+clean. A PG counts as
// clean when its state contains both "active" and "clean" — this admits routine
// scrubbing (active+clean+scrubbing) while rejecting any recovery/degraded/
// undersized/peering/down state, which is exactly the mid-rebalance condition a
// node op must wait out.
func parseCephHealth(data []byte) (CephHealth, error) {
	var s cephStatus
	if err := json.Unmarshal(data, &s); err != nil {
		return CephHealth{}, &errtypes.ClusterError{Msg: "ceph: parse status json", Err: err}
	}

	monsTotal := len(s.Monmap.Mons)
	if monsTotal == 0 {
		monsTotal = s.Monmap.NumMons
	}
	monsInQuorum := len(s.QuorumNames)

	osd := s.Osdmap.resolve()
	degradedPGs := 0
	for _, pg := range s.Pgmap.PgsByState {
		st := pg.StateName
		if !strings.Contains(st, "active") || !strings.Contains(st, "clean") {
			degradedPGs += pg.Count
		}
	}

	h := CephHealth{
		Applicable:   true,
		MonsInQuorum: monsInQuorum,
		MonsTotal:    monsTotal,
		OSDsUp:       osd.NumUpOsds,
		OSDsIn:       osd.NumInOsds,
		OSDsTotal:    osd.NumOsds,
		DegradedPGs:  degradedPGs,
	}
	switch {
	case monsTotal == 0 || monsInQuorum != monsTotal:
		h.Reason = fmt.Sprintf("mons not all in quorum (%d/%d)", monsInQuorum, monsTotal)
	case osd.NumOsds == 0 || osd.NumUpOsds != osd.NumOsds || osd.NumInOsds != osd.NumOsds:
		h.Reason = fmt.Sprintf("osds not all up/in (up %d, in %d, of %d)", osd.NumUpOsds, osd.NumInOsds, osd.NumOsds)
	case degradedPGs > 0:
		h.Reason = fmt.Sprintf("%d PGs not active+clean (recovery/degraded in progress)", degradedPGs)
	default:
		h.Healthy = true
	}
	return h, nil
}
