package steps

import (
	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

// BasicsStepDefinition declares the cluster identity fields.
var BasicsStepDefinition = wizard.StepDefinition{
	ID:           wizard.StepIDBasics,
	Title:        "cluster basics",
	DisplayTitle: "configure your cluster basics",
	Description:  "name the cluster and its domain",
	Sections: []wizard.SectionDefinition{
		{
			Title: "cluster identity",
			Fields: []wizard.FieldDefinition{
				{
					Key:       config.FieldClusterName,
					Label:     "cluster name",
					Default:   "mycluster",
					Help:      "lowercase letters, numbers, and hyphens only — nodes are named <cluster>-master0..2 and <cluster>-worker<n>",
					Required:  true,
					Validate:  validateClusterName,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Cluster.Name = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Cluster.Name }),
				},
				{
					Key:       config.FieldClusterDomain,
					Label:     "domain",
					Default:   "k8s.local",
					Help:      "dot-separated domain name for cluster services, e.g. example.com",
					Required:  true,
					Validate:  validateDomain,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Cluster.Domain = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Cluster.Domain }),
				},
			},
		},
	},
}

// NewBasicsStep returns the basics wizard step.
func NewBasicsStep() *wizard.DataDrivenStep {
	return wizard.NewDataDrivenStep(&BasicsStepDefinition)
}
