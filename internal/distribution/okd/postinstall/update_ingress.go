package postinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/dns"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/templates"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
)

// DefaultIngressLBTimeout caps how long update-ingress waits for the
// ingress LB service to report a ready external IP. 10 minutes is the
// empirical upper bound observed on kube-vip-managed deployments; the
// conversion timeout is shorter because the delete-then-create cycle
// doesn't wait for pods to schedule.
const (
	DefaultIngressLBTimeout  = 10 * time.Minute
	defaultConversionTimeout = 5 * time.Minute
	routerGonePollInterval   = 5 * time.Second
)

// UpdateIngressOptions configures the update-ingress flow. ConfirmConversion
// is called when HostNetwork IngressControllers are detected; returning
// false aborts conversion.
type UpdateIngressOptions struct {
	RemoveHAProxy     bool
	ConfirmConversion func(hostNetworkICs []string) bool
	// WorkDir is the okdctl work directory (parent of cluster-config/); used to
	// locate the kubeconfig CA when re-verifying the VIP after HAProxy removal.
	WorkDir string
}

// IngressEntry describes one IngressController observed (or converted)
// during update-ingress.
type IngressEntry struct {
	Name        string
	Domain      string
	LBIP        string
	Converted   bool
	HostNetwork bool
}

// UpdateIngressResult summarises the outcome of an update-ingress run.
// DNSReconciled is true when update-ingress detected that the on-disk dnsmasq
// config was still in bootstrap state and re-deployed production DNS.
type UpdateIngressResult struct {
	Entries        []IngressEntry
	KubeVipIP      string
	HAProxyRemoved bool
	ConvertedCount int
	DNSReconciled  bool
}

// UpdateIngress discovers all IngressControllers from the cluster, optionally
// converts HostNetwork controllers to LoadBalancerService, waits for their
// LoadBalancer IPs, deploys production DNS, and optionally removes HAProxy.
func (p *Phase) UpdateIngress(ctx context.Context, cfg *config.Config, opts UpdateIngressOptions) (*UpdateIngressResult, error) {
	vip, err := phase.ResolveClusterVIP(cfg)
	if err != nil {
		return nil, &errtypes.ConfigError{Msg: "failed to resolve cluster VIP", Err: err}
	}

	bootstrapDNS, err := dns.IsBootstrapDNS(cfg)
	if err != nil {
		p.Log.Warn("update-ingress: could not check dns state", "err", err)
	}
	if bootstrapDNS {
		p.Log.Warn("update-ingress: dns is bootstrap-pointed — api.* resolves to bastion; will reconcile to vip after ingress detection")
	}

	postOpts := &Options{
		Timeout: DefaultTimeout,
	}

	p.Log.Info("update-ingress: detecting ingress strategy and loadbalancer ips...")

	controllers, err := p.discoverIngressControllers(ctx)
	if err != nil {
		if !bootstrapDNS {
			return nil, &errtypes.ClusterError{Msg: "failed to discover IngressControllers", Err: err}
		}
		p.Log.Warn("update-ingress: cluster unreachable — reconciling bootstrap dns to kube-vip only",
			"err", err)
		return p.reconcileBootstrapDNSOnly(ctx, cfg, vip)
	}

	if len(controllers) == 0 {
		if !bootstrapDNS {
			return nil, &errtypes.ClusterError{Msg: "no IngressControllers found in the cluster"}
		}
		p.Log.Warn("update-ingress: no controllers found — reconciling bootstrap dns to kube-vip only")
		return p.reconcileBootstrapDNSOnly(ctx, cfg, vip)
	}

	var descriptions []string
	for _, ic := range controllers {
		descriptions = append(descriptions, fmt.Sprintf("%s (%s)", ic.Name, ic.Strategy))
	}
	p.Log.Info("update-ingress: discovered controllers",
		"count", len(controllers), "controllers", strings.Join(descriptions, ", "))

	var hostNetworkICs, lbICs []ingressControllerInfo
	for _, ic := range controllers {
		if ic.Strategy == strategyHostNetwork {
			hostNetworkICs = append(hostNetworkICs, ic)
		} else {
			lbICs = append(lbICs, ic)
		}
	}

	convertedCount := 0
	var convertedNames map[string]bool
	if len(hostNetworkICs) > 0 {
		converted, names, err := p.handleHostNetworkConversion(ctx, hostNetworkICs, opts, postOpts)
		if err != nil {
			return nil, &errtypes.ClusterError{Msg: "hostnetwork ingress conversion failed", Err: err}
		}
		convertedCount = converted
		convertedNames = names

		if converted > 0 {
			controllers, err = p.discoverIngressControllers(ctx)
			if err != nil {
				return nil, &errtypes.ClusterError{Msg: "failed to re-discover IngressControllers after conversion", Err: err}
			}

			hostNetworkICs = nil
			lbICs = nil
			for _, ic := range controllers {
				if convertedNames[ic.Name] {
					ic.converted = true
				}
				if ic.Strategy == strategyHostNetwork {
					hostNetworkICs = append(hostNetworkICs, ic)
				} else {
					lbICs = append(lbICs, ic)
				}
			}
		}
	}

	entries, customDomains, defaultAppsIP, err := p.collectLBEntries(ctx, lbICs, hostNetworkICs, cfg.Networking.Bastion.IP, postOpts)
	if err != nil {
		return nil, err
	}

	result, err := p.finalizeIngress(ctx, cfg, opts, entries, customDomains, defaultAppsIP, vip, convertedCount, len(hostNetworkICs))
	if err != nil {
		return nil, err
	}
	result.DNSReconciled = bootstrapDNS
	return result, nil
}

// reconcileBootstrapDNSOnly deploys production DNS without querying the cluster.
// Used when bootstrap-state DNS is detected but ingress discovery fails or
// returns no controllers (e.g. kube-vip-only deploys, or a cluster API that
// is unreachable mid-postinstall). *.apps stays on the bastion until a
// LoadBalancer addon is installed later.
func (p *Phase) reconcileBootstrapDNSOnly(ctx context.Context, cfg *config.Config, vip string) (*UpdateIngressResult, error) {
	appsIP := cfg.Networking.Bastion.IP
	if err := p.deployProductionDNS(ctx, cfg, appsIP, vip, nil); err != nil {
		return nil, err
	}
	p.Log.Info("update-ingress: dns updated (kube-vip only)", "apps", appsIP, "api", vip)
	return &UpdateIngressResult{
		KubeVipIP:     vip,
		DNSReconciled: true,
	}, nil
}

// collectLBEntries waits for LoadBalancer IPs on all LB-type controllers.
// HostNetwork controllers that were not converted get entries pointing at the
// bastion IP.
func (p *Phase) collectLBEntries(
	ctx context.Context,
	lbICs, hostNetworkICs []ingressControllerInfo,
	bastionIP string,
	postOpts *Options,
) ([]IngressEntry, []templates.DNSCustomDomain, string, error) {
	var defaultAppsIP string
	var entries []IngressEntry
	var customDomains []templates.DNSCustomDomain

	for _, ic := range lbICs {
		svcName := fmt.Sprintf("router-%s", ic.Name)
		p.Log.Info("update-ingress: waiting for loadbalancer ip", "svc", svcName)

		ip, err := p.waitForServiceLB(ctx, svcName, postOpts)
		if err != nil {
			if ic.Name == "default" {
				return nil, nil, "", &errtypes.ClusterError{Msg: "router-default has no LoadBalancer IP", Err: err}
			}
			p.Log.Warn("update-ingress: service has no loadbalancer ip", "svc", svcName, "err", err)
			continue
		}

		p.Log.Info("update-ingress: service loadbalancer ip resolved", "svc", svcName, "ip", ip)

		entry := IngressEntry{
			Name:      ic.Name,
			Domain:    ic.Domain,
			LBIP:      ip,
			Converted: ic.converted,
		}
		entries = append(entries, entry)

		if ic.Name == "default" {
			defaultAppsIP = ip
		} else {
			customDomains = append(customDomains, templates.DNSCustomDomain{
				Domain: ic.Domain,
				IP:     ip,
			})
		}
	}

	for _, ic := range hostNetworkICs {
		entries = append(entries, IngressEntry{
			Name:        ic.Name,
			Domain:      ic.Domain,
			LBIP:        bastionIP,
			HostNetwork: true,
		})
	}

	return entries, customDomains, defaultAppsIP, nil
}

func (p *Phase) finalizeIngress(
	ctx context.Context,
	cfg *config.Config,
	opts UpdateIngressOptions,
	entries []IngressEntry,
	customDomains []templates.DNSCustomDomain,
	defaultAppsIP, vip string,
	convertedCount, hostNetworkCount int,
) (*UpdateIngressResult, error) {
	appsIP := defaultAppsIP
	if appsIP == "" {
		appsIP = cfg.Networking.Bastion.IP
		p.Log.Warn("update-ingress: default controller has no loadbalancer ip, using bastion ip for *.apps dns")
	}

	p.Log.Info("update-ingress: deploying production dns with loadbalancer ips")
	if err := p.deployProductionDNS(ctx, cfg, appsIP, vip, customDomains); err != nil {
		return nil, &errtypes.ClusterError{Msg: "failed to deploy production DNS", Err: err}
	}
	p.Log.Info("update-ingress: dns updated", "apps", appsIP, "api", vip)

	result := &UpdateIngressResult{
		Entries:        entries,
		KubeVipIP:      vip,
		ConvertedCount: convertedCount,
	}

	// DNS swap must precede HAProxy removal: RemoveHAProxy verifies the new
	// path by resolving api.* via dnsmasq, which must already point at the
	// VIP — not the bastion — before HAProxy stops listening.
	if opts.RemoveHAProxy && hostNetworkCount == 0 {
		p.Log.Info("update-ingress: removing haproxy from bastion")
		if err := p.RemoveHAProxy(ctx, vip, phase.ClusterConfigDir(opts.WorkDir)); err != nil {
			p.Log.Warn("update-ingress: haproxy removal failed — rolling back dns to bootstrap", "err", err)
			dnsRolledBack := false
			if rbErr := dns.DeployBootstrap(ctx, cfg); rbErr != nil {
				p.Log.Warn("update-ingress: dns rollback to bootstrap failed", "err", rbErr)
			} else {
				dnsRolledBack = true
			}
			cfgRestored := p.restoreHAProxyBackup()
			if dnsRolledBack && cfgRestored {
				p.Log.Info("update-ingress: rollback complete — dns restored to bootstrap, haproxy config rehydrated")
			}
			return nil, &errtypes.ClusterError{Msg: "haproxy removal failed after dns swap — dns rolled back to bootstrap", Err: err}
		}
		result.HAProxyRemoved = true
		p.Log.Info("update-ingress: haproxy removed from bastion")
	} else if opts.RemoveHAProxy && hostNetworkCount > 0 {
		p.Log.Warn("update-ingress: skipping haproxy removal — hostnetwork controllers still active")
	}

	return result, nil
}

// handleHostNetworkConversion converts HostNetwork IngressControllers to
// LoadBalancerService when MetalLB is available and the caller confirms.
// Returns the number converted and the set of converted names.
func (p *Phase) handleHostNetworkConversion(
	ctx context.Context,
	hostNetworkICs []ingressControllerInfo,
	opts UpdateIngressOptions,
	postOpts *Options,
) (convertedCount int, names map[string]bool, retErr error) {
	names = make(map[string]bool)

	icNames := make([]string, len(hostNetworkICs))
	for i, ic := range hostNetworkICs {
		icNames[i] = ic.Name
	}

	p.Log.Warn("update-ingress: controllers using HostNetwork",
		"count", len(hostNetworkICs), "controllers", strings.Join(icNames, ", "))

	metalLBAvailable, err := p.checkMetalLBAvailable(ctx)
	if err != nil {
		p.Log.Warn("update-ingress: could not check metallb availability", "err", err)
		return 0, names, nil
	}
	if !metalLBAvailable {
		p.Log.Warn("update-ingress: metallb not detected — skipping hostnetwork conversion")
		return 0, names, nil
	}

	p.Log.Info("update-ingress: metallb detected with available ips")

	if opts.ConfirmConversion == nil {
		p.Log.Warn("update-ingress: no conversion confirmation callback — skipping hostnetwork conversion")
		return 0, names, nil
	}

	if !opts.ConfirmConversion(icNames) {
		p.Log.Info("update-ingress: user declined hostnetwork conversion — skipping")
		return 0, names, nil
	}

	timeout := defaultConversionTimeout
	if postOpts.Timeout > 0 {
		timeout = postOpts.Timeout
	}

	for i := range hostNetworkICs {
		ic := &hostNetworkICs[i]
		p.Log.Info("update-ingress: converting from hostnetwork to loadbalancerservice", "name", ic.Name)
		if err := p.convertToLoadBalancer(ctx, ic, timeout); err != nil {
			return convertedCount, names, fmt.Errorf("failed to convert IngressController %q: %w", ic.Name, err)
		}
		names[ic.Name] = true
		convertedCount++
		p.Log.Info("update-ingress: converted successfully", "name", ic.Name)
	}

	return convertedCount, names, nil
}

// IngressStrategy mirrors IngressController.spec.endpointPublishingStrategy.type.
type IngressStrategy string

const (
	strategyHostNetwork  IngressStrategy = "HostNetwork"
	strategyLoadBalancer IngressStrategy = "LoadBalancerService"
)

// parseIngressStrategy returns the typed constant for s when s is one of
// the handled strategies, and ok=false for any other non-empty value.
// Callers must handle ok=false: unknown strategies should be skipped with
// a warning rather than silently routing through HostNetwork logic.
func parseIngressStrategy(s string) (IngressStrategy, bool) {
	switch IngressStrategy(s) {
	case strategyHostNetwork, strategyLoadBalancer:
		return IngressStrategy(s), true
	default:
		return "", false
	}
}

type ingressControllerInfo struct {
	Name      string
	Domain    string
	Strategy  IngressStrategy
	RawJSON   json.RawMessage
	converted bool
}

func (p *Phase) discoverIngressControllers(ctx context.Context) ([]ingressControllerInfo, error) {
	result, err := p.Exec.RunChecked(ctx, "oc", "get", "ingresscontroller",
		"-n", "openshift-ingress-operator",
		"-o", "json")
	if err != nil {
		return nil, fmt.Errorf("failed to query IngressControllers: %w", err)
	}

	var list struct {
		Items []json.RawMessage `json:"items"`
	}

	if err := json.Unmarshal([]byte(result.Stdout), &list); err != nil {
		return nil, fmt.Errorf("failed to parse IngressController list: %w", err)
	}

	var controllers []ingressControllerInfo
	for _, raw := range list.Items {
		var item struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				EndpointPublishingStrategy *struct {
					Type IngressStrategy `json:"type"`
				} `json:"endpointPublishingStrategy"`
			} `json:"spec"`
			Status struct {
				Domain string `json:"domain"`
			} `json:"status"`
		}

		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("failed to parse IngressController item: %w", err)
		}

		// OKD's bare-metal default has endpointPublishingStrategy unset (null) —
		// treat null/empty as HostNetwork.
		strategy := strategyHostNetwork
		if item.Spec.EndpointPublishingStrategy != nil && item.Spec.EndpointPublishingStrategy.Type != "" {
			raw := string(item.Spec.EndpointPublishingStrategy.Type)
			parsed, ok := parseIngressStrategy(raw)
			if !ok {
				p.Log.Warn("update-ingress: skipping controller with unrecognised ingress strategy",
					"name", item.Metadata.Name, "strategy", raw)
				continue
			}
			strategy = parsed
		}

		if item.Metadata.Name == "" {
			continue
		}

		controllers = append(controllers, ingressControllerInfo{
			Name:     item.Metadata.Name,
			Domain:   item.Status.Domain,
			Strategy: strategy,
			RawJSON:  raw,
		})
	}

	return controllers, nil
}

func (p *Phase) checkMetalLBAvailable(ctx context.Context) (bool, error) {
	if ok, err := p.OcResourceExists(ctx, "failed to check metallb-system namespace", "namespace", "metallb-system"); err != nil || !ok {
		return false, err
	}
	if ok, err := p.OcResourceExists(ctx, "failed to check IPAddressPools", "ipaddresspool", "-n", "metallb-system"); err != nil || !ok {
		return false, err
	}
	return true, nil
}

func (p *Phase) convertToLoadBalancer(ctx context.Context, ic *ingressControllerInfo, timeout time.Duration) error {
	replacementJSON, err := buildLBIngressController(ic)
	if err != nil {
		return fmt.Errorf("failed to build replacement IngressController: %w", err)
	}

	_, err = p.Exec.RunChecked(ctx, "oc", "delete", "ingresscontroller", ic.Name,
		"-n", "openshift-ingress-operator")
	if err != nil {
		return fmt.Errorf("failed to delete IngressController %q: %w", ic.Name, err)
	}

	p.Log.Info("update-ingress: waiting for router deployment to terminate", "name", ic.Name)
	if err := p.waitForRouterGone(ctx, ic.Name, timeout); err != nil {
		return fmt.Errorf("router-%s did not terminate: %w", ic.Name, err)
	}
	p.Log.Info("update-ingress: router terminated", "name", ic.Name)

	_, err = p.Exec.RunWithStdinChecked(ctx, replacementJSON, "oc", "create", "-f", "-")
	if err != nil {
		p.Log.Warn("update-ingress: failed to create replacement, attempting rollback", "err", err)
		p.attemptRollback(ctx, ic)
		return fmt.Errorf("failed to create replacement IngressController: %w", err)
	}

	return nil
}

func buildLBIngressController(ic *ingressControllerInfo) (string, error) {
	var original struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Domain             string           `json:"domain,omitempty"`
			Replicas           *int32           `json:"replicas,omitempty"`
			DefaultCertificate *json.RawMessage `json:"defaultCertificate,omitempty"`
			RouteSelector      *json.RawMessage `json:"routeSelector,omitempty"`
			RouteAdmission     *json.RawMessage `json:"routeAdmission,omitempty"`
			NodePlacement      *json.RawMessage `json:"nodePlacement,omitempty"`
		} `json:"spec"`
	}

	if err := json.Unmarshal(ic.RawJSON, &original); err != nil {
		return "", fmt.Errorf("failed to parse original IngressController: %w", err)
	}

	namespace := original.Metadata.Namespace
	if namespace == "" {
		namespace = "openshift-ingress-operator"
	}

	type endpointPublishingStrategy struct {
		Type IngressStrategy `json:"type"`
	}
	type specOut struct {
		EndpointPublishingStrategy endpointPublishingStrategy `json:"endpointPublishingStrategy"`
		Domain                     string                     `json:"domain,omitempty"`
		Replicas                   *int32                     `json:"replicas,omitempty"`
		DefaultCertificate         *json.RawMessage           `json:"defaultCertificate,omitempty"`
		RouteSelector              *json.RawMessage           `json:"routeSelector,omitempty"`
		RouteAdmission             *json.RawMessage           `json:"routeAdmission,omitempty"`
		NodePlacement              *json.RawMessage           `json:"nodePlacement,omitempty"`
	}
	type metadataOut struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	type replacementOut struct {
		APIVersion string      `json:"apiVersion"`
		Kind       string      `json:"kind"`
		Metadata   metadataOut `json:"metadata"`
		Spec       specOut     `json:"spec"`
	}

	replacement := replacementOut{
		APIVersion: "operator.openshift.io/v1",
		Kind:       "IngressController",
		Metadata:   metadataOut{Name: original.Metadata.Name, Namespace: namespace},
		Spec: specOut{
			EndpointPublishingStrategy: endpointPublishingStrategy{Type: strategyLoadBalancer},
			Domain:                     original.Spec.Domain,
			Replicas:                   original.Spec.Replicas,
			DefaultCertificate:         original.Spec.DefaultCertificate,
			RouteSelector:              original.Spec.RouteSelector,
			RouteAdmission:             original.Spec.RouteAdmission,
			NodePlacement:              original.Spec.NodePlacement,
		},
	}

	data, err := json.Marshal(replacement)
	if err != nil {
		return "", fmt.Errorf("failed to marshal replacement IngressController: %w", err)
	}

	return string(data), nil
}

// buildRollbackJSON strips server-managed fields from the original RawJSON so
// the payload round-trips through `oc create` during rollback.
func buildRollbackJSON(ic *ingressControllerInfo) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(ic.RawJSON, &obj); err != nil {
		return "", err
	}

	if metaRaw, ok := obj["metadata"]; ok {
		var meta map[string]json.RawMessage
		if err := json.Unmarshal(metaRaw, &meta); err == nil {
			kept := make(map[string]json.RawMessage, 6)
			for _, field := range []string{
				"name", "namespace", "labels", "annotations",
				"ownerReferences", "finalizers",
			} {
				if v, exists := meta[field]; exists {
					kept[field] = v
				}
			}
			if cleaned, err := json.Marshal(kept); err == nil {
				obj["metadata"] = cleaned
			}
		}
	}

	delete(obj, "status")

	data, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// attemptRollback recreates the original IngressController from its captured
// RawJSON. Errors are logged only — the caller already has a primary error.
func (p *Phase) attemptRollback(ctx context.Context, ic *ingressControllerInfo) {
	p.Log.Info("update-ingress: rollback: starting", "name", ic.Name)
	rollbackJSON, err := buildRollbackJSON(ic)
	if err != nil {
		p.Log.Warn("update-ingress: rollback failed — could not build rollback json", "err", err)
		return
	}

	result, err := p.Exec.RunWithStdin(ctx, rollbackJSON, "oc", "create", "-f", "-")
	if err != nil || result.ExitCode != 0 {
		p.Log.Warn("update-ingress: rollback create failed", "err", err, "stderr", logutil.RedactableStderr(result.Stderr))
		return
	}

	p.Log.Info("update-ingress: rollback succeeded — restored with original strategy", "name", ic.Name)
}

// restoreHAProxyBackup finds the most recent haproxy.cfg.backup.* file left
// by RemoveHAProxy and copies it back to haproxyConfigPath so DNS rollback
// has a coherent recovery target. Returns true only when the restore
// succeeds; errors are logged as warnings.
func (p *Phase) restoreHAProxyBackup() bool {
	pattern := haproxyConfigPath + ".backup.*"
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		p.Log.Warn("update-ingress: rollback: no haproxy backup found", "pattern", pattern)
		return false
	}
	sort.Strings(matches)
	latest := matches[len(matches)-1]
	data, err := os.ReadFile(latest)
	if err != nil {
		p.Log.Warn("update-ingress: rollback: could not read haproxy backup", "path", latest, "err", err)
		return false
	}
	if err := os.WriteFile(haproxyConfigPath, data, 0o640); err != nil {
		p.Log.Warn("update-ingress: rollback: could not restore haproxy config", "path", haproxyConfigPath, "err", err)
		return false
	}
	p.Log.Info("update-ingress: rollback: haproxy config restored from backup", "backup", latest)
	return true
}

func (p *Phase) waitForRouterGone(ctx context.Context, icName string, timeout time.Duration) error {
	deployName := fmt.Sprintf("router-%s", icName)

	return system.WaitFor(ctx, "ingress", deployName+" termination", func(context.Context) bool {
		result, _ := p.Exec.Run(ctx, "oc", "get", "deployment", deployName,
			"-n", "openshift-ingress", "--no-headers", "--ignore-not-found")
		// --ignore-not-found returns empty stdout when the deployment is gone.
		return strings.TrimSpace(result.Stdout) == ""
	}, system.WaitForOptions{
		Interval: routerGonePollInterval,
		Timeout:  timeout,
		Logger:   p.Log,
	})
}

func (p *Phase) waitForServiceLB(ctx context.Context, svcName string, opts *Options) (string, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultIngressLBTimeout
	}
	ip, err := p.OcPollOutput(ctx, "ingress", svcName+" lb", timeout,
		func(v string) bool { return v != "" },
		"get", "svc", svcName, "-n", "openshift-ingress",
		"-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
	if err != nil {
		return "", fmt.Errorf("%s did not receive a LoadBalancer IP within %v: %w", svcName, timeout, err)
	}
	return ip, nil
}
