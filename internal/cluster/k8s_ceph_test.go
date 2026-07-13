package cluster

import "testing"

func TestParseCephHealthStructurallyHealthy(t *testing.T) {
	data := `{
	  "quorum_names": ["a","b","c"],
	  "monmap": {"mons":[{"name":"a"},{"name":"b"},{"name":"c"}]},
	  "osdmap": {"num_osds":3,"num_up_osds":3,"num_in_osds":3},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}
	}`
	h, err := parseCephHealth([]byte(data))
	if err != nil {
		t.Fatalf("parseCephHealth: %v", err)
	}
	if !h.Applicable || !h.Healthy {
		t.Fatalf("want applicable+healthy, got %+v", h)
	}
}

func TestParseCephHealthBenignWarnIsHealthy(t *testing.T) {
	// Steady-state HEALTH_WARN (slow ops) plus a scrubbing PG: structurally clean,
	// so it must gate as healthy. The health block is intentionally ignored.
	data := `{
	  "health": {"status":"HEALTH_WARN","checks":{"BLUESTORE_SLOW_OP_ALERT":{"severity":"HEALTH_WARN"}}},
	  "quorum_names": ["a","b","c"],
	  "monmap": {"mons":[{},{},{}]},
	  "osdmap": {"num_osds":3,"num_up_osds":3,"num_in_osds":3},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[
	    {"state_name":"active+clean","count":90},
	    {"state_name":"active+clean+scrubbing+deep","count":10}]}
	}`
	h, err := parseCephHealth([]byte(data))
	if err != nil {
		t.Fatalf("parseCephHealth: %v", err)
	}
	if !h.Healthy {
		t.Fatalf("benign warn + scrubbing must be healthy, got reason %q", h.Reason)
	}
	if h.DegradedPGs != 0 {
		t.Errorf("scrubbing PGs must not count as degraded, got %d", h.DegradedPGs)
	}
}

func TestParseCephHealthDegradedPGs(t *testing.T) {
	data := `{
	  "quorum_names": ["a","b","c"],
	  "monmap": {"mons":[{},{},{}]},
	  "osdmap": {"num_osds":3,"num_up_osds":3,"num_in_osds":3},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[
	    {"state_name":"active+clean","count":80},
	    {"state_name":"active+undersized+degraded","count":20}]}
	}`
	h, _ := parseCephHealth([]byte(data))
	if h.Healthy {
		t.Fatal("degraded PGs must gate as unhealthy")
	}
	if h.DegradedPGs != 20 {
		t.Errorf("want 20 degraded PGs, got %d", h.DegradedPGs)
	}
}

func TestParseCephHealthOSDDown(t *testing.T) {
	data := `{
	  "quorum_names": ["a","b","c"],
	  "monmap": {"mons":[{},{},{}]},
	  "osdmap": {"num_osds":3,"num_up_osds":2,"num_in_osds":3},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}
	}`
	h, _ := parseCephHealth([]byte(data))
	if h.Healthy {
		t.Fatal("an OSD down must gate as unhealthy")
	}
}

func TestParseCephHealthMonOutOfQuorum(t *testing.T) {
	data := `{
	  "quorum_names": ["a","b"],
	  "monmap": {"mons":[{},{},{}]},
	  "osdmap": {"num_osds":3,"num_up_osds":3,"num_in_osds":3},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}
	}`
	h, _ := parseCephHealth([]byte(data))
	if h.Healthy {
		t.Fatal("a mon out of quorum must gate as unhealthy")
	}
}

func TestParseCephHealthNestedOsdmap(t *testing.T) {
	// Older ceph nests the OSD counts under osdmap.osdmap.
	data := `{
	  "quorum_names": ["a","b","c"],
	  "monmap": {"mons":[{},{},{}]},
	  "osdmap": {"osdmap":{"num_osds":3,"num_up_osds":3,"num_in_osds":3}},
	  "pgmap": {"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}
	}`
	h, err := parseCephHealth([]byte(data))
	if err != nil {
		t.Fatalf("parseCephHealth: %v", err)
	}
	if !h.Healthy || h.OSDsTotal != 3 {
		t.Fatalf("nested osdmap must resolve to healthy 3 OSDs, got %+v", h)
	}
}
