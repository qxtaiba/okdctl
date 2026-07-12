package cluster

import "testing"

func TestParseOperatorAvailableStable(t *testing.T) {
	stable := `{"status":{"conditions":[
	  {"type":"Available","status":"True"},
	  {"type":"Degraded","status":"False"},
	  {"type":"Progressing","status":"False"}
	]}}`
	ok, err := parseOperatorAvailableStable([]byte(stable))
	if err != nil || !ok {
		t.Fatalf("want stable, got %v (%v)", ok, err)
	}

	progressing := `{"status":{"conditions":[
	  {"type":"Available","status":"True"},
	  {"type":"Progressing","status":"True"}
	]}}`
	ok, err = parseOperatorAvailableStable([]byte(progressing))
	if err != nil || ok {
		t.Fatalf("want not-stable while progressing, got %v (%v)", ok, err)
	}

	degraded := `{"status":{"conditions":[
	  {"type":"Available","status":"True"},
	  {"type":"Degraded","status":"True"}
	]}}`
	ok, _ = parseOperatorAvailableStable([]byte(degraded))
	if ok {
		t.Fatal("want not-stable while degraded")
	}
}

func TestParsePodsReady(t *testing.T) {
	data := `{"items":[
	  {"status":{"conditions":[{"type":"Ready","status":"True"}]}},
	  {"status":{"conditions":[{"type":"Ready","status":"True"}]}},
	  {"status":{"conditions":[{"type":"Ready","status":"False"}]}}
	]}`
	ready, total, err := parsePodsReady([]byte(data))
	if err != nil {
		t.Fatalf("parsePodsReady: %v", err)
	}
	if ready != 2 || total != 3 {
		t.Fatalf("want 2/3, got %d/%d", ready, total)
	}
}

func TestParseEtcdRolloutInFlight(t *testing.T) {
	rolling := `{"status":{"conditions":[{"type":"NodeInstallerProgressing","status":"True"}]}}`
	inFlight, err := parseEtcdRolloutInFlight([]byte(rolling))
	if err != nil || !inFlight {
		t.Fatalf("want in-flight, got %v (%v)", inFlight, err)
	}
	stable := `{"status":{"conditions":[{"type":"NodeInstallerProgressing","status":"False"}]}}`
	inFlight, _ = parseEtcdRolloutInFlight([]byte(stable))
	if inFlight {
		t.Fatal("want not in-flight")
	}
}
