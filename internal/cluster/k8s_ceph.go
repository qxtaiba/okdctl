package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// CephHealth summarizes rook-ceph fitness for a node op. Applicable is false
// with no toolbox pod; Healthy means structural health only (quorum, OSD
// up/in, PG active+clean) — benign warnings like BlueStore slow ops don't count.
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

// CephHealthy runs `ceph -s -f json` in the toolbox pod and evaluates
// structural health, returning Applicable=false when no toolbox pod exists.
func (c *Client) CephHealthy(ctx context.Context) (CephHealth, error) {
	pods, err := c.PodsForSelector(ctx, "", "app=rook-ceph-tools")
	if err != nil {
		return CephHealth{}, &errtypes.ClusterError{Msg: "ceph: find toolbox pod", Err: err}
	}
	tools := firstScheduledPod(pods)
	if tools == nil {
		return CephHealth{Applicable: false}, nil
	}

	raw, err := c.getJSONChecked(ctx, "ceph: exec ceph -s",
		"exec", "-n", tools.Namespace, tools.Name, "--", "ceph", "-s", "-f", "json")
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

// cephOsdmap handles both the flat (Reef+) and nested (older) osdmap shapes.
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

// parseCephHealth requires every mon in quorum, every OSD up/in, and every
// PG active+clean; scrubbing counts as clean, degraded/recovering does not.
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
