package steps

import (
	"regexp"
	"strconv"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
)

var nodeKeySuffix = regexp.MustCompile(`-(master|worker)(0|[1-9]\d*)$`)

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
					Validate:  config.CheckClusterName,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Cluster.Name = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Cluster.Name }),
				},
				{
					Key:       config.FieldClusterDomain,
					Label:     "domain",
					Default:   "k8s.local",
					Help:      "dot-separated domain name for cluster services, e.g. example.com",
					Required:  true,
					Validate:  config.CheckDomain,
					ConfigSet: wizard.SetString(func(c *config.Config, v string) { c.Cluster.Domain = v }),
					ConfigGet: wizard.GetString(func(c *config.Config) string { return c.Cluster.Domain }),
				},
			},
		},
	},
	Apply: func(_ *wizard.DataDrivenStep, cfg *config.Config) error {
		rekeyNodeOverrides(cfg)
		return nil
	},
}

// rekeyNodeOverrides moves a nodes.<old>-<role><i> override under the
// current cluster name so a rename keeps the operator's placement; a key
// with no role suffix stays for Validate to report.
func rekeyNodeOverrides(cfg *config.Config) {
	for key, override := range cfg.Nodes {
		if _, _, ok := cfg.ParseNodeKey(key); ok {
			continue
		}
		m := nodeKeySuffix.FindStringSubmatch(key)
		if m == nil {
			continue
		}
		index, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		name := cfg.NodeName(config.Role(m[1]), index)
		if _, taken := cfg.Nodes[name]; !taken {
			cfg.Nodes[name] = override
		}
		delete(cfg.Nodes, key)
	}
}

// NewBasicsStep returns the basics wizard step.
func NewBasicsStep() *wizard.DataDrivenStep {
	return wizard.NewDataDrivenStep(&BasicsStepDefinition)
}
