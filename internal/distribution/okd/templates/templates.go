// Package templates provides embedded manifest templates for OKD clusters.
package templates

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed *.tmpl
var templateFS embed.FS

// FstrimMachineConfigData is the template binding for the fstrim
// MachineConfig manifest, rendered once per node pool (master, worker).
type FstrimMachineConfigData struct {
	Role string // machineconfiguration.openshift.io/role label value
	Name string // metadata.name
}

// RenderFstrimMachineConfig renders the fstrim MachineConfig manifest for
// data.Role. It masks FCOS's stock fstrim.timer (no /etc/fstab for `fstrim
// --fstab`, see coreos/fedora-coreos-tracker#468) and ships a replacement
// unit for explicit mountpoints.
func RenderFstrimMachineConfig(data FstrimMachineConfigData) (string, error) {
	return renderTemplate("fstrim-machineconfig.yaml.tmpl", data)
}

func renderTemplate(name string, data any) (string, error) {
	content, err := templateFS.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", name, err)
	}

	tmpl, err := template.New(name).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", name, err)
	}

	return buf.String(), nil
}
