package install

import (
	"bytes"
	"sync"
	"testing"
)

func TestParseMilestone(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantOK   bool
		wantKind MilestoneKind
		wantOp   string
	}{
		{
			name:     "bootstrap complete",
			line:     `level=info msg="It is now safe to remove the bootstrap resources"`,
			wantOK:   true,
			wantKind: MilestoneBootstrapComplete,
		},
		{
			name:     "install complete",
			line:     `level=info msg="Install complete!"`,
			wantOK:   true,
			wantKind: MilestoneInstallComplete,
		},
		{
			name:     "operator degraded warning",
			line:     `level=warning msg="Cluster operator authentication Degraded is True with WellKnownEndpoint..."`,
			wantOK:   true,
			wantKind: MilestoneOperatorDegraded,
			wantOp:   "authentication",
		},
		{
			name:     "operator degraded lowercase debug prefix",
			line:     `time="2024-01-02T15:04:05Z" level=info msg="cluster operator console Degraded is True"`,
			wantOK:   true,
			wantKind: MilestoneOperatorDegraded,
			wantOp:   "console",
		},
		{
			name:   "available-false is not a degraded milestone",
			line:   `level=info msg="Cluster operator console Available is False with ..."`,
			wantOK: false,
		},
		{
			name:   "empty line",
			line:   "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, ok := ParseMilestone(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ParseMilestone ok = %v; want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if m.Kind != tt.wantKind {
				t.Errorf("Kind = %d; want %d", m.Kind, tt.wantKind)
			}
			if m.Operator != tt.wantOp {
				t.Errorf("Operator = %q; want %q", m.Operator, tt.wantOp)
			}
		})
	}
}

func TestMilestoneWriter_TeesAndNotifies(t *testing.T) {
	var sink bytes.Buffer
	var mu sync.Mutex
	var got []Milestone
	w := NewMilestoneWriter(&sink, func(m Milestone) {
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	})

	// Chunks deliberately split a line across Write calls.
	chunks := []string{
		"level=info msg=\"Waiting for bootstrap\"\n",
		"level=info msg=\"It is now safe to rem",
		"ove the bootstrap resources\"\nlevel=warning msg=\"Cluster operator etcd Degraded is True\"\n",
		"level=info msg=\"Install complete!\"\n",
	}
	full := ""
	for _, c := range chunks {
		full += c
		if _, err := w.Write([]byte(c)); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	if sink.String() != full {
		t.Errorf("sink did not receive the verbatim stream:\ngot  %q\nwant %q", sink.String(), full)
	}

	wantKinds := []MilestoneKind{MilestoneBootstrapComplete, MilestoneOperatorDegraded, MilestoneInstallComplete}
	if len(got) != len(wantKinds) {
		t.Fatalf("milestones = %+v; want %d", got, len(wantKinds))
	}
	for i, k := range wantKinds {
		if got[i].Kind != k {
			t.Errorf("milestone[%d].Kind = %d; want %d", i, got[i].Kind, k)
		}
	}
	if got[1].Operator != "etcd" {
		t.Errorf("degraded operator = %q; want etcd", got[1].Operator)
	}
}
