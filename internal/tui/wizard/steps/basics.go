package steps

import (
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// BasicsStepDefinition declares the cluster-basics step fields.
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
					Key:       fieldDomain,
					Label:     fieldDomain,
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
					Help:      "number of control plane nodes (odd for etcd quorum)",
					Type:      wizard.FieldTypeSelect,
					Options:   []string{"1", "3", "5"},
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

// NewBasicsStep returns the basics wizard step.
func NewBasicsStep() *wizard.DataDrivenStep {
	return wizard.NewDataDrivenStep(&BasicsStepDefinition)
}
