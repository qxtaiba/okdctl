package postinstall

import (
	"context"
	"encoding/json"
	"fmt"
)

// disabledCatalogSources are the default OperatorHub CatalogSources whose
// index images require a Red Hat subscription to pull. No OKD cluster
// carries one, so these index pods never resolve and the operators they
// list can never be installed (okd-project/okd#2058). community-operators
// needs no subscription and is left enabled.
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

// disableSubscriptionGatedCatalogSources merge-patches
// operatorhub.config.openshift.io/cluster to disable the subscription-gated
// default CatalogSources. A merge patch replaces spec.sources wholesale,
// which is safe here since this runs once against a freshly installed
// cluster with an empty sources list.
func (p *Phase) disableSubscriptionGatedCatalogSources(ctx context.Context) error {
	patch, err := buildOperatorHubPatch(disabledCatalogSources)
	if err != nil {
		return err
	}
	return p.OcPatch(ctx, "operatorhub.config.openshift.io", "cluster", "merge", patch)
}

// insightsConfigManifest silences the InsightsDisabled alert (and its
// SimpleContentAccessNotAvailable/InsightsRecommendationActive siblings),
// which fires permanently on every OKD cluster because the Insights
// operator requires a console.redhat.com token no OKD install can supply
// (okd-project/okd#2058). Two other documented mechanisms were considered
// and rejected: the openshift-config/support secret's disableInsightsAlerts
// key also turns off legitimate remote-health data upload, a broader
// behaviour change than silencing one unresolvable alert; an Alertmanager
// silence lives in Alertmanager's own state store rather than a manifest,
// so it cannot be applied idempotently via oc and does not survive a
// cluster rebuild. The insights-config ConfigMap's alerting.disabled field
// is insights-operator's own purpose-built, declarative switch for exactly
// this alert, so it is the least invasive of the three (mechanism documented
// at github.com/openshift/insights-operator/blob/master/docs/arch.md).
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
