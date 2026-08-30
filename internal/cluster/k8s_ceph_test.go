package cluster

import "testing"

func TestParseCephHealth(t *testing.T) {
	tests := []struct {
		name         string
		data         string
		wantHealthy  bool
		wantDegraded int
		wantOSDs     int
	}{
		{
			name: "structurally healthy",
			data: `{
	  "quorum_names": ["a","b","c"],
	  "monmap": {"mons":[{"name":"a"},{"name":"b"},{"name":"c"}]},
	  "osdmap": {"num_osds":3,"num_up_osds":3,"num_in_osds":3},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}
	}`,
			wantHealthy: true, wantOSDs: 3,
		},
		{
			// Structurally clean despite HEALTH_WARN; scrubbing PGs aren't degraded.
			name: "benign warn is healthy",
			data: `{
	  "health": {"status":"HEALTH_WARN","checks":{"BLUESTORE_SLOW_OP_ALERT":{"severity":"HEALTH_WARN"}}},
	  "quorum_names": ["a","b","c"],
	  "monmap": {"mons":[{},{},{}]},
	  "osdmap": {"num_osds":3,"num_up_osds":3,"num_in_osds":3},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[
	    {"state_name":"active+clean","count":90},
	    {"state_name":"active+clean+scrubbing+deep","count":10}]}
	}`,
			wantHealthy: true, wantOSDs: 3,
		},
		{
			name: "degraded pgs gate as unhealthy",
			data: `{
	  "quorum_names": ["a","b","c"],
	  "monmap": {"mons":[{},{},{}]},
	  "osdmap": {"num_osds":3,"num_up_osds":3,"num_in_osds":3},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[
	    {"state_name":"active+clean","count":80},
	    {"state_name":"active+undersized+degraded","count":20}]}
	}`,
			wantDegraded: 20, wantOSDs: 3,
		},
		{
			name: "osd down gates as unhealthy",
			data: `{
	  "quorum_names": ["a","b","c"],
	  "monmap": {"mons":[{},{},{}]},
	  "osdmap": {"num_osds":3,"num_up_osds":2,"num_in_osds":3},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}
	}`,
			wantOSDs: 3,
		},
		{
			name: "mon out of quorum gates as unhealthy",
			data: `{
	  "quorum_names": ["a","b"],
	  "monmap": {"mons":[{},{},{}]},
	  "osdmap": {"num_osds":3,"num_up_osds":3,"num_in_osds":3},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}
	}`,
			wantOSDs: 3,
		},
		{
			// Older ceph nests the OSD counts under osdmap.osdmap.
			name: "nested osdmap resolves",
			data: `{
	  "quorum_names": ["a","b","c"],
	  "monmap": {"mons":[{},{},{}]},
	  "osdmap": {"osdmap":{"num_osds":3,"num_up_osds":3,"num_in_osds":3}},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}
	}`,
			wantHealthy: true, wantOSDs: 3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, err := parseCephHealth([]byte(tc.data))
			if err != nil {
				t.Fatalf("parseCephHealth: %v", err)
			}
			if !h.Applicable {
				t.Fatalf("want applicable, got %+v", h)
			}
			if h.Healthy != tc.wantHealthy {
				t.Fatalf("Healthy = %v; want %v (reason %q, %+v)", h.Healthy, tc.wantHealthy, h.Reason, h)
			}
			if h.DegradedPGs != tc.wantDegraded {
				t.Errorf("DegradedPGs = %d; want %d", h.DegradedPGs, tc.wantDegraded)
			}
			if h.OSDsTotal != tc.wantOSDs {
				t.Errorf("OSDsTotal = %d; want %d", h.OSDsTotal, tc.wantOSDs)
			}
		})
	}
}
