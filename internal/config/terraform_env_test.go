package config

import "testing"

func TestTerraformEnvName(t *testing.T) {
	if got := (&Config{}).TerraformEnvName(); got != "production" {
		t.Errorf("TerraformEnvName() = %q; want production default", got)
	}
	cfg := &Config{Deployment: DeploymentConfig{TerraformEnv: "staging"}}
	if got := cfg.TerraformEnvName(); got != "staging" {
		t.Errorf("TerraformEnvName() = %q; want staging", got)
	}
}
