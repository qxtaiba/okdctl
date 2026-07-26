package infrastructure

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var (
	tmplAssignment  = regexp.MustCompile(`(?m)^(\w+)\s*=`)
	varDeclaration  = regexp.MustCompile(`(?m)^variable\s+"(\w+)"`)
	modulePassthrough = regexp.MustCompile(`(?m)^\s*(\w+)\s*=\s*var\.(\w+)\b`)
)

// TestTfvarsTemplateVarsWired guards the render→root→module pipeline: every
// variable okdctl's terraform.tfvars template renders must be declared in the
// production root and passed through to the module. Terraform silently
// ignores tfvars values for undeclared variables, so a template addition
// without root wiring means the module default wins over user config.
func TestTfvarsTemplateVarsWired(t *testing.T) {
	tmplPath := filepath.Join("..", "internal", "distribution", "okd", "templates", "terraform.tfvars.tmpl")
	tmpl, err := os.ReadFile(tmplPath)
	if err != nil {
		t.Fatalf("read tfvars template: %v", err)
	}
	varsTF, err := fs.ReadFile(TerraformFS, "terraform/environments/production/variables.tf")
	if err != nil {
		t.Fatalf("read embedded production variables.tf: %v", err)
	}
	mainTF, err := fs.ReadFile(TerraformFS, "terraform/environments/production/main.tf")
	if err != nil {
		t.Fatalf("read embedded production main.tf: %v", err)
	}

	declared := map[string]bool{}
	for _, m := range varDeclaration.FindAllStringSubmatch(string(varsTF), -1) {
		declared[m[1]] = true
	}
	passed := map[string]string{}
	for _, m := range modulePassthrough.FindAllStringSubmatch(string(mainTF), -1) {
		passed[m[1]] = m[2]
	}

	rendered := tmplAssignment.FindAllStringSubmatch(string(tmpl), -1)
	if len(rendered) == 0 {
		t.Fatal("no variable assignments parsed from terraform.tfvars.tmpl; template shape changed, update the parser")
	}
	for _, m := range rendered {
		name := m[1]
		if !declared[name] {
			t.Errorf("template renders %q but production variables.tf never declares it; terraform silently ignores the rendered value — declare it and pass it through in main.tf", name)
		}
		if passed[name] != name {
			t.Errorf("template renders %q but production main.tf never passes var.%s to the module; the module default wins over user config", name, name)
		}
	}
}
