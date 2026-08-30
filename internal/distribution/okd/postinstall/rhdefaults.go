package postinstall

import (
	"context"
	"encoding/json"
	"fmt"
)

// disabledCatalogSources need a Red Hat subscription OKD lacks (okd-project/okd#2058).
var disabledCatalogSources = []string{"redhat-operators", "certified-operators", "redhat-marketplace"}

type operatorHubSource struct {
	Name     string `json:"name"`
	Disabled bool   `json:"disabled"`
}

type operatorHubPatch struct {
	Spec struct {
		Sources []operatorHubSource `json:"sources"`
	} `json:"spec"`
}

func buildOperatorHubPatch(names []string) (string, error) {
	var patch operatorHubPatch
	patch.Spec.Sources = make([]operatorHubSource, len(names))
	for i, name := range names {
		patch.Spec.Sources[i] = operatorHubSource{Name: name, Disabled: true}
	}
	out, err := json.Marshal(patch)
	if err != nil {
		return "", fmt.Errorf("marshal operatorhub patch: %w", err)
	}
	return string(out), nil
}

// Merge-patches operatorhub.config.openshift.io/cluster; wholesale replace of
// spec.sources is safe since this only ever runs once, on a fresh cluster.
func (p *Phase) disableSubscriptionGatedCatalogSources(ctx context.Context) error {
	patch, err := buildOperatorHubPatch(disabledCatalogSources)
	if err != nil {
		return err
	}
	return p.OcPatch(ctx, "operatorhub.config.openshift.io", "cluster", "merge", patch)
}

// insightsConfigManifest silences the permanent InsightsDisabled alert — OKD
// lacks the console.redhat.com token the Insights operator requires (okd-project/okd#2058).
const insightsConfigManifest = `apiVersion: v1
kind: ConfigMap
metadata:
  name: insights-config
  namespace: openshift-insights
data:
  config.yaml: |
    alerting:
      disabled: true
`

func (p *Phase) silenceInsightsDisabledAlert(ctx context.Context) error {
	return p.OcApply(ctx, []byte(insightsConfigManifest))
}
