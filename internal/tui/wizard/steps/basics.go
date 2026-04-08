package steps

import (
	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/tui/wizard"
)

var BasicsStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDBasics,
	Title:        "cluster basics",
	DisplayTitle: "configure your cluster basics",
	Description:  "configure cluster identity and topology",
	Sections: []wizard.SectionDefinition{
		{
			Title: "cluster identity",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "cluster_name",
					Label:     "cluster name",
					Default:   "mycluster",
					Help:      "lowercase letters, numbers, and hyphens only",
					Required:  true,
					Validate:  ValidateClusterName,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Cluster.Name = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Cluster.Name }),
				},
				{
					Key:       "domain",
					Label:     "domain",
					Default:   "k8s.local",
					Help:      "base domain for cluster services",
					Required:  true,
					Validate:  ValidateDomain,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Cluster.Domain = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Cluster.Domain }),
				},
			},
		},
		{
			Title: "node topology",
			Fields: []wizard.FieldDefinition{
				{
					Key:       "control_plane_count",
					Label:     "control plane",
					Default:   "3",
					Help:      "number of control plane nodes (1, 3, or 5 recommended)",
					Required:  true,
					Validate:  ValidateNodeCount,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.ControlPlane.Count = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.ControlPlane.Count }),
				},
				{
					Key:       "worker_count",
					Label:     "workers",
					Default:   "3",
					Help:      "number of worker nodes",
					Required:  true,
					Validate:  ValidateNodeCount,
					ConfigSet: wizard.SetInt(func(c *config.Config, v int) { c.Topology.Workers.Count = v }),
					ConfigGet: wizard.GetInt(func(c *config.Config) int { return c.Topology.Workers.Count }),
				},
			},
		},
	},
}

func NewBasicsStep() (*wizard.DataDrivenStep, *wizard.DataDrivenStep) {
	step := wizard.NewDataDrivenStep(BasicsStepDefinition)
	return step, step
}
