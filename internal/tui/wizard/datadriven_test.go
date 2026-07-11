package wizard

import (
	"errors"
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
)

const (
	testValYes = "yes"
	testValABC = "abc"
)

func testStepDefinition() *StepDefinition {
	return &StepDefinition{
		ID:           StepIDBasics,
		Title:        "test step",
		DisplayTitle: "test step",
		Sections: []SectionDefinition{
			{
				Title: "section one",
				Fields: []FieldDefinition{
					{
						Key:       "name",
						Label:     "name",
						Required:  true,
						ConfigSet: SetString(func(c *config.Config, v string) { c.Cluster.Name = v }),
						ConfigGet: GetString(func(c *config.Config) string { return c.Cluster.Name }),
					},
					{
						Key:       "count",
						Label:     "count",
						ConfigSet: SetInt(func(c *config.Config, v int) { c.Topology.ControlPlane.Count = v }),
						ConfigGet: GetInt(func(c *config.Config) int { return c.Topology.ControlPlane.Count }),
					},
					{
						Key:       "approve",
						Label:     "approve",
						Default:   "no",
						Type:      FieldTypeSelect,
						Options:   []string{"no", testValYes},
						ConfigSet: SetBool(func(c *config.Config, v bool) { c.Deployment.AutoApprove = v }),
						ConfigGet: func(c *config.Config) string {
							if c.Deployment.AutoApprove {
								return testValYes
							}
							return "no"
						},
					},
				},
			},
		},
		Validate: func(values map[string]string) error {
			if values["name"] == "forbidden" {
				return errors.New("name is forbidden")
			}
			return nil
		},
		Apply: func(step *DataDrivenStep, cfg *config.Config) error {
			cfg.Cluster.Domain = "applied." + step.Value("name")
			return nil
		},
		ShouldShow: func(cfg *config.Config) bool {
			return cfg.Distribution.Type != "skip-me"
		},
	}
}

func TestDataDrivenStep_ValueSetValue(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())

	if got := step.Value("name"); got != "" {
		t.Fatalf("Value(name) = %q, want empty before any set", got)
	}

	step.setValue("name", "cluster-a")
	if got := step.Value("name"); got != "cluster-a" {
		t.Fatalf("Value(name) after setValue = %q, want cluster-a", got)
	}

	// Unknown keys are a no-op, not a panic.
	step.setValue("does-not-exist", "x")
	if got := step.Value("does-not-exist"); got != "" {
		t.Fatalf("Value(unknown key) = %q, want empty", got)
	}
}

func TestDataDrivenStep_ValueInt(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())

	if got := step.ValueInt("count", 42); got != 42 {
		t.Fatalf("ValueInt(count) on empty field = %d, want fallback 42", got)
	}

	step.setValue("count", "9")
	if got := step.ValueInt("count", 42); got != 9 {
		t.Fatalf("ValueInt(count) = %d, want 9", got)
	}

	step.setValue("count", "not-a-number")
	if got := step.ValueInt("count", 42); got != 42 {
		t.Fatalf("ValueInt(count) with unparsable value = %d, want fallback 42", got)
	}
}

func TestDataDrivenStep_LoadFromConfig(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	cfg := &config.Config{}
	cfg.Cluster.Name = "loaded-cluster"
	cfg.Topology.ControlPlane.Count = 5
	cfg.Deployment.AutoApprove = true

	step.LoadFromConfig(cfg)

	if got := step.Value("name"); got != "loaded-cluster" {
		t.Errorf("Value(name) = %q, want loaded-cluster", got)
	}
	if got := step.ValueInt("count", -1); got != 5 {
		t.Errorf("ValueInt(count) = %d, want 5", got)
	}
	if got := step.Value("approve"); got != testValYes {
		t.Errorf("Value(approve) = %q, want yes", got)
	}
}

func TestDataDrivenStep_Validate(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	step.setValue("name", "cluster-a")

	if err := step.Validate(); err != nil {
		t.Fatalf("Validate() with valid values: %v", err)
	}

	step.setValue("name", "forbidden")
	if err := step.Validate(); err == nil {
		t.Fatal("Validate() with forbidden name: want error, got nil")
	}
}

func TestDataDrivenStep_Validate_RequiredFieldBlocks(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	// "name" is Required and starts empty (no setValue/LoadFromConfig yet).
	if err := step.Validate(); err == nil {
		t.Fatal("Validate() with empty required field: want error, got nil")
	}
}

func TestDataDrivenStep_Apply(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	step.setValue("name", "cluster-a")
	step.setValue("count", "7")
	step.setValue("approve", testValYes)

	cfg := &config.Config{}
	if err := step.Apply(cfg); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	if cfg.Cluster.Name != "cluster-a" {
		t.Errorf("cfg.Cluster.Name = %q, want cluster-a", cfg.Cluster.Name)
	}
	if cfg.Topology.ControlPlane.Count != 7 {
		t.Errorf("cfg.Topology.ControlPlane.Count = %d, want 7", cfg.Topology.ControlPlane.Count)
	}
	if !cfg.Deployment.AutoApprove {
		t.Error("cfg.Deployment.AutoApprove = false, want true")
	}
	// The step-level Apply hook runs after field auto-binding and observes
	// the already-applied value via step.Value.
	if cfg.Cluster.Domain != "applied.cluster-a" {
		t.Errorf("cfg.Cluster.Domain = %q, want applied.cluster-a", cfg.Cluster.Domain)
	}
}

func TestDataDrivenStep_Apply_PropagatesConfigSetError(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	step.setValue("count", "not-a-number")

	cfg := &config.Config{}
	if err := step.Apply(cfg); err == nil {
		t.Fatal("Apply() with unparsable int field: want error, got nil")
	}
}

func TestDataDrivenStep_ShouldShow(t *testing.T) {
	step := NewDataDrivenStep(testStepDefinition())
	cfg := &config.Config{}

	if !step.ShouldShow(cfg) {
		t.Error("ShouldShow() = false, want true for default config")
	}

	cfg.Distribution.Type = "skip-me"
	if step.ShouldShow(cfg) {
		t.Error("ShouldShow() = true, want false when the predicate rejects")
	}
}

func TestDataDrivenStep_ShouldShow_DefaultsToTrue(t *testing.T) {
	def := &StepDefinition{ID: StepIDBasics}
	step := NewDataDrivenStep(def)
	if !step.ShouldShow(&config.Config{}) {
		t.Error("ShouldShow() with nil predicate = false, want true")
	}
}

func TestSetStringSetIntSetBool(t *testing.T) {
	cfg := &config.Config{}

	setStr := SetString(func(c *config.Config, v string) { c.Cluster.Name = v })
	if err := setStr(cfg, testValABC); err != nil {
		t.Fatalf("SetString: %v", err)
	}
	if cfg.Cluster.Name != testValABC {
		t.Errorf("SetString did not set value: got %q", cfg.Cluster.Name)
	}

	setInt := SetInt(func(c *config.Config, v int) { c.Topology.ControlPlane.Count = v })
	if err := setInt(cfg, "5"); err != nil {
		t.Fatalf("SetInt: %v", err)
	}
	if cfg.Topology.ControlPlane.Count != 5 {
		t.Errorf("SetInt did not set value: got %d", cfg.Topology.ControlPlane.Count)
	}
	if err := setInt(cfg, "not-an-int"); err == nil {
		t.Fatal("SetInt with non-numeric input: want error, got nil")
	}

	setBool := SetBool(func(c *config.Config, v bool) { c.Deployment.AutoApprove = v })
	for _, truthy := range []string{testValYes, "true", "1", "y", "YES"} {
		cfg.Deployment.AutoApprove = false
		if err := setBool(cfg, truthy); err != nil {
			t.Fatalf("SetBool(%q): %v", truthy, err)
		}
		if !cfg.Deployment.AutoApprove {
			t.Errorf("SetBool(%q) did not set true", truthy)
		}
	}
	for _, falsy := range []string{"no", "false", "0", "", "n"} {
		cfg.Deployment.AutoApprove = true
		if err := setBool(cfg, falsy); err != nil {
			t.Fatalf("SetBool(%q): %v", falsy, err)
		}
		if cfg.Deployment.AutoApprove {
			t.Errorf("SetBool(%q) did not set false", falsy)
		}
	}
}

func TestGetStringGetInt(t *testing.T) {
	cfg := &config.Config{}
	cfg.Cluster.Name = testValABC
	cfg.Topology.ControlPlane.Count = 5

	getStr := GetString(func(c *config.Config) string { return c.Cluster.Name })
	if got := getStr(cfg); got != testValABC {
		t.Errorf("GetString() = %q, want abc", got)
	}

	getInt := GetInt(func(c *config.Config) int { return c.Topology.ControlPlane.Count })
	if got := getInt(cfg); got != "5" {
		t.Errorf("GetInt() = %q, want \"5\"", got)
	}
}
