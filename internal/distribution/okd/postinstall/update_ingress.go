package postinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qxtaiba/okd-proxmox-cli/internal/config"
	"github.com/qxtaiba/okd-proxmox-cli/internal/distribution/okd/templates"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/netutil"
	"github.com/qxtaiba/okd-proxmox-cli/internal/utils/system"
)

const (
	DefaultIngressLBTimeout  = 10 * time.Minute
	defaultConversionTimeout = 5 * time.Minute
	routerGonePollInterval   = 5 * time.Second
)

type UpdateIngressOptions struct {
	RemoveHAProxy     bool
	ConfirmConversion func(hostNetworkICs []string) bool
}

// IngressEntry represents a discovered IngressController and its LB IP.
type IngressEntry struct {
	Name        string // IngressController name (e.g., "default", "grappleberry")
	Domain      string // Domain served (e.g., "apps.grappleberry.k8s.local", "grappleberry.xyz")
	LBIP        string // LoadBalancer IP assigned by MetalLB
	Converted   bool   // true if this IC was converted from HostNetwork
	HostNetwork bool   // true if this IC is still using HostNetwork (not converted)
}

type UpdateIngressResult struct {
	Entries        []IngressEntry
	KubeVipIP      string
	HAProxyRemoved bool
	ConvertedCount int
}

// UpdateIngress discovers all IngressControllers from the cluster, optionally
// converts HostNetwork controllers to LoadBalancerService, waits for their
// LoadBalancer IPs, deploys production DNS, and optionally removes HAProxy.
func (p *Phase) UpdateIngress(ctx context.Context, cfg *config.Config, opts UpdateIngressOptions) (*UpdateIngressResult, error) {
	vip, err := netutil.DeriveVIPFromStaticIP(cfg.Networking.StaticIP.Start)
	if err != nil {
		return nil, fmt.Errorf("failed to derive VIP from static IP start: %w", err)
	}

	postOpts := Options{
		Timeout: DefaultTimeout,
	}

	p.Log.Info("update-ingress: detecting ingress strategy and loadbalancer ips...")

	controllers, err := p.discoverIngressControllers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to discover IngressControllers: %w", err)
	}

	if len(controllers) == 0 {
		return nil, fmt.Errorf("no IngressControllers found in the cluster")
	}

	// Log what we found.
	var descriptions []string
	for _, ic := range controllers {
		descriptions = append(descriptions, fmt.Sprintf("%s (%s)", ic.Name, ic.Strategy))
	}
	p.Log.Info(fmt.Sprintf("update-ingress: found %d controller(s): %s",
		len(controllers), strings.Join(descriptions, ", ")))

	// Separate by strategy.
	var hostNetworkICs, lbICs []ingressControllerInfo
	for _, ic := range controllers {
		if ic.Strategy == strategyHostNetwork {
			hostNetworkICs = append(hostNetworkICs, ic)
		} else {
			lbICs = append(lbICs, ic)
		}
	}

	// Attempt conversion of HostNetwork ICs.
	convertedCount := 0
	var convertedNames map[string]bool
	if len(hostNetworkICs) > 0 {
		converted, names, err := p.handleHostNetworkConversion(ctx, hostNetworkICs, opts, postOpts)
		if err != nil {
			return nil, err
		}
		convertedCount = converted
		convertedNames = names

		if converted > 0 {
			// Re-discover to pick up the recreated controllers.
			controllers, err = p.discoverIngressControllers(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to re-discover IngressControllers after conversion: %w", err)
			}

			// Re-separate after conversion, marking converted ICs.
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

	return p.finalizeIngress(ctx, cfg, opts, entries, customDomains, defaultAppsIP, vip, convertedCount, len(hostNetworkICs))
}

// collectLBEntries waits for LoadBalancer IPs on all LB-type controllers and
// builds the entries list. HostNetwork controllers that were not converted get
// entries pointing at the bastion IP.
func (p *Phase) collectLBEntries(
	ctx context.Context,
	lbICs, hostNetworkICs []ingressControllerInfo,
	bastionIP string,
	postOpts Options,
) ([]IngressEntry, []templates.DNSCustomDomain, string, error) {
	var defaultAppsIP string
	var entries []IngressEntry
	var customDomains []templates.DNSCustomDomain

	for _, ic := range lbICs {
		svcName := fmt.Sprintf("router-%s", ic.Name)
		p.Log.Info(fmt.Sprintf("update-ingress: waiting for %s loadbalancer ip", svcName))

		ip, err := p.waitForServiceLB(ctx, svcName, postOpts)
		if err != nil {
			if ic.Name == "default" {
				return nil, nil, "", fmt.Errorf("router-default has no LoadBalancer IP: %w", err)
			}
			p.Log.Warn(fmt.Sprintf("update-ingress: %s has no loadbalancer ip: %v", svcName, err))
			continue
		}

		p.Log.Info(fmt.Sprintf("update-ingress: %s loadbalancer ip is %s", svcName, ip))

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

	// HostNetwork ICs that weren't converted — DNS stays at bastion.
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

// finalizeIngress deploys production DNS with the collected LB IPs and
// optionally removes HAProxy.
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
		return nil, fmt.Errorf("failed to deploy production DNS: %w", err)
	}
	p.Log.Info(fmt.Sprintf("update-ingress: dns updated — *.apps → %s, api.* → %s", appsIP, vip))

	result := &UpdateIngressResult{
		Entries:        entries,
		KubeVipIP:      vip,
		ConvertedCount: convertedCount,
	}

	// Only remove HAProxy if ALL ICs are LoadBalancerService (none remain HostNetwork).
	if opts.RemoveHAProxy && hostNetworkCount == 0 {
		p.Log.Info("update-ingress: removing haproxy from bastion")
		if err := p.RemoveHAProxy(ctx, vip); err != nil {
			p.Log.Warn(fmt.Sprintf("update-ingress: haproxy removal failed: %v", err))
		} else {
			result.HAProxyRemoved = true
			p.Log.Info("update-ingress: haproxy removed from bastion")
		}
	} else if opts.RemoveHAProxy && hostNetworkCount > 0 {
		p.Log.Warn("update-ingress: skipping haproxy removal — hostnetwork controllers still active")
	}

	return result, nil
}

// handleHostNetworkConversion checks if MetalLB is available and, if the user
// confirms, converts HostNetwork IngressControllers to LoadBalancerService.
// Returns the number of controllers converted and the set of converted names.
func (p *Phase) handleHostNetworkConversion(
	ctx context.Context,
	hostNetworkICs []ingressControllerInfo,
	opts UpdateIngressOptions,
	postOpts Options,
) (int, map[string]bool, error) {
	convertedNames := make(map[string]bool)

	var names []string
	for _, ic := range hostNetworkICs {
		names = append(names, ic.Name)
	}

	p.Log.Warn(fmt.Sprintf("update-ingress: found %d controller(s) using HostNetwork: %s",
		len(hostNetworkICs), strings.Join(names, ", ")))

	metalLBAvailable, err := p.checkMetalLBAvailable(ctx)
	if err != nil {
		p.Log.Warn(fmt.Sprintf("update-ingress: could not check metallb availability: %v", err))
		return 0, convertedNames, nil
	}
	if !metalLBAvailable {
		p.Log.Warn("update-ingress: metallb not detected — skipping hostnetwork conversion")
		return 0, convertedNames, nil
	}

	p.Log.Info("update-ingress: metallb detected with available ips")

	if opts.ConfirmConversion == nil {
		p.Log.Warn("update-ingress: no conversion confirmation callback — skipping hostnetwork conversion")
		return 0, convertedNames, nil
	}

	var icNames []string
	for _, ic := range hostNetworkICs {
		icNames = append(icNames, ic.Name)
	}
	if !opts.ConfirmConversion(icNames) {
		p.Log.Info("update-ingress: user declined hostnetwork conversion — skipping")
		return 0, convertedNames, nil
	}

	timeout := defaultConversionTimeout
	if postOpts.Timeout > 0 {
		timeout = postOpts.Timeout
	}

	converted := 0
	for _, ic := range hostNetworkICs {
		p.Log.Info(fmt.Sprintf("update-ingress: converting %q from hostnetwork to loadbalancerservice...", ic.Name))
		if err := p.convertToLoadBalancer(ctx, ic, timeout); err != nil {
			return converted, convertedNames, fmt.Errorf("failed to convert IngressController %q: %w", ic.Name, err)
		}
		convertedNames[ic.Name] = true
		converted++
		p.Log.Info(fmt.Sprintf("update-ingress: %q converted successfully", ic.Name))
	}

	return converted, convertedNames, nil
}

const (
	strategyHostNetwork  = "HostNetwork"
	strategyLoadBalancer = "LoadBalancerService"
)

type ingressControllerInfo struct {
	Name      string
	Domain    string
	Strategy  string
	RawJSON   json.RawMessage
	converted bool // set after conversion
}

// discoverIngressControllers queries the cluster for all IngressControllers
// and returns their names, domains, strategies, and raw JSON.
func (p *Phase) discoverIngressControllers(ctx context.Context) ([]ingressControllerInfo, error) {
	result, err := p.Exec.Run(ctx, "oc", "get", "ingresscontroller",
		"-n", "openshift-ingress-operator",
		"-o", "json")
	if err != nil {
		return nil, fmt.Errorf("failed to query IngressControllers: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("oc get ingresscontroller failed: %s", result.Stderr)
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
					Type string `json:"type"`
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
			strategy = item.Spec.EndpointPublishingStrategy.Type
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

// checkMetalLBAvailable returns true if MetalLB is installed and has at least
// one IPAddressPool configured.
func (p *Phase) checkMetalLBAvailable(ctx context.Context) (bool, error) {
	// Check namespace exists.
	nsResult, err := p.Exec.Run(ctx, "oc", "get", "namespace", "metallb-system",
		"--no-headers", "--ignore-not-found")
	if err != nil {
		return false, fmt.Errorf("failed to check metallb-system namespace: %w", err)
	}
	if nsResult.ExitCode != 0 || strings.TrimSpace(nsResult.Stdout) == "" {
		return false, nil
	}

	// Check at least one IPAddressPool exists.
	poolResult, err := p.Exec.Run(ctx, "oc", "get", "ipaddresspool",
		"-n", "metallb-system", "--no-headers", "--ignore-not-found")
	if err != nil {
		return false, fmt.Errorf("failed to check IPAddressPools: %w", err)
	}
	if poolResult.ExitCode != 0 || strings.TrimSpace(poolResult.Stdout) == "" {
		return false, nil
	}

	return true, nil
}

// convertToLoadBalancer deletes a HostNetwork IngressController and recreates
// it with LoadBalancerService strategy.
func (p *Phase) convertToLoadBalancer(ctx context.Context, ic ingressControllerInfo, timeout time.Duration) error {
	// Build the replacement JSON before deleting.
	replacementJSON, err := buildLBIngressController(ic)
	if err != nil {
		return fmt.Errorf("failed to build replacement IngressController: %w", err)
	}

	// Delete the existing IC.
	delResult, err := p.Exec.Run(ctx, "oc", "delete", "ingresscontroller", ic.Name,
		"-n", "openshift-ingress-operator")
	if err != nil {
		return fmt.Errorf("failed to delete IngressController %q: %w", ic.Name, err)
	}
	if delResult.ExitCode != 0 {
		return fmt.Errorf("oc delete ingresscontroller %s failed: %s", ic.Name, delResult.Stderr)
	}

	// Wait for the router deployment to be gone.
	p.Log.Info(fmt.Sprintf("update-ingress: waiting for router-%s deployment to terminate...", ic.Name))
	if err := p.waitForRouterGone(ctx, ic.Name, timeout); err != nil {
		return fmt.Errorf("router-%s did not terminate: %w", ic.Name, err)
	}

	// Create the replacement via stdin.
	createResult, err := p.Exec.RunWithStdin(ctx, replacementJSON, "oc", "create", "-f", "-")
	if err != nil {
		p.Log.Warn(fmt.Sprintf("update-ingress: failed to create replacement, attempting rollback: %v", err))
		p.attemptRollback(ctx, ic)
		return fmt.Errorf("failed to create replacement IngressController: %w", err)
	}
	if createResult == nil {
		p.Log.Warn("update-ingress: oc create returned nil result, attempting rollback")
		p.attemptRollback(ctx, ic)
		return fmt.Errorf("oc create returned nil result for IngressController %q", ic.Name)
	}
	if createResult.ExitCode != 0 {
		p.Log.Warn(fmt.Sprintf("update-ingress: create failed (%s), attempting rollback", createResult.Stderr))
		p.attemptRollback(ctx, ic)
		return fmt.Errorf("oc create ingresscontroller failed: %s", createResult.Stderr)
	}

	return nil
}

// buildLBIngressController constructs a clean IngressController JSON with
// LoadBalancerService strategy, preserving key fields from the original.
func buildLBIngressController(ic ingressControllerInfo) (string, error) {
	// Parse the original to extract fields we want to preserve.
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

	// Build the replacement spec.
	spec := map[string]any{
		"endpointPublishingStrategy": map[string]any{
			"type": strategyLoadBalancer,
		},
	}

	if original.Spec.Domain != "" {
		spec["domain"] = original.Spec.Domain
	}
	if original.Spec.Replicas != nil {
		spec["replicas"] = *original.Spec.Replicas
	}
	if original.Spec.DefaultCertificate != nil {
		spec["defaultCertificate"] = original.Spec.DefaultCertificate
	}
	if original.Spec.RouteSelector != nil {
		spec["routeSelector"] = original.Spec.RouteSelector
	}
	if original.Spec.RouteAdmission != nil {
		spec["routeAdmission"] = original.Spec.RouteAdmission
	}
	if original.Spec.NodePlacement != nil {
		spec["nodePlacement"] = original.Spec.NodePlacement
	}

	namespace := original.Metadata.Namespace
	if namespace == "" {
		namespace = "openshift-ingress-operator"
	}

	replacement := map[string]any{
		"apiVersion": "operator.openshift.io/v1",
		"kind":       "IngressController",
		"metadata": map[string]any{
			"name":      original.Metadata.Name,
			"namespace": namespace,
		},
		"spec": spec,
	}

	data, err := json.Marshal(replacement)
	if err != nil {
		return "", fmt.Errorf("failed to marshal replacement IngressController: %w", err)
	}

	return string(data), nil
}

// buildRollbackJSON strips server-managed fields from the original RawJSON
// for use with oc create during rollback.
func buildRollbackJSON(ic ingressControllerInfo) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(ic.RawJSON, &obj); err != nil {
		return "", err
	}

	// Strip server-managed fields from metadata.
	if metaRaw, ok := obj["metadata"]; ok {
		var meta map[string]json.RawMessage
		if err := json.Unmarshal(metaRaw, &meta); err == nil {
			for _, field := range []string{
				"creationTimestamp", "generation", "resourceVersion",
				"selfLink", "uid", "managedFields",
			} {
				delete(meta, field)
			}
			if cleaned, err := json.Marshal(meta); err == nil {
				obj["metadata"] = cleaned
			}
		}
	}

	// Strip status — not needed for create.
	delete(obj, "status")

	data, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// attemptRollback tries to recreate the original IngressController from its
// captured RawJSON. Errors are logged but not returned — the caller already
// has a primary error to report.
func (p *Phase) attemptRollback(ctx context.Context, ic ingressControllerInfo) {
	rollbackJSON, err := buildRollbackJSON(ic)
	if err != nil {
		p.Log.Warn(fmt.Sprintf("update-ingress: rollback failed — could not build rollback json: %v", err))
		return
	}

	result, err := p.Exec.RunWithStdin(ctx, rollbackJSON, "oc", "create", "-f", "-")
	if err != nil || (result != nil && result.ExitCode != 0) {
		stderr := ""
		if result != nil {
			stderr = result.Stderr
		}
		p.Log.Warn(fmt.Sprintf("update-ingress: rollback create failed: %v %s", err, stderr))
		return
	}

	p.Log.Info(fmt.Sprintf("update-ingress: rollback succeeded — %q restored with original strategy", ic.Name))

}

// waitForRouterGone polls until the router-<name> deployment no longer exists.
func (p *Phase) waitForRouterGone(ctx context.Context, icName string, timeout time.Duration) error {
	deployName := fmt.Sprintf("router-%s", icName)

	return system.WaitFor(ctx, "ingress", deployName+" termination", func() bool {
		result, _ := p.Exec.Run(ctx, "oc", "get", "deployment", deployName,
			"-n", "openshift-ingress", "--no-headers", "--ignore-not-found")
		if result == nil {
			return false
		}
		// Gone when stdout is empty (--ignore-not-found returns empty for missing resources).
		return strings.TrimSpace(result.Stdout) == ""
	}, system.WaitForOptions{
		Interval: routerGonePollInterval,
		Timeout:  timeout,
		Logger:   p.Log,
	})
}

// waitForServiceLB polls a service in openshift-ingress until a LoadBalancer IP is assigned.
func (p *Phase) waitForServiceLB(ctx context.Context, svcName string, opts Options) (string, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultIngressLBTimeout
	}

	var ip string
	if err := system.WaitForWithTimeout(ctx, "ingress", svcName+" lb", func() bool {
		result, _ := p.Exec.Run(ctx, "oc", "get", "svc", svcName,
			"-n", "openshift-ingress",
			"-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
		if result == nil || result.ExitCode != 0 {
			return false
		}
		candidate := strings.TrimSpace(result.Stdout)
		if candidate == "" {
			return false
		}
		ip = candidate
		return true
	}, timeout, p.Log); err != nil {
		return "", fmt.Errorf("%s did not receive a LoadBalancer IP within %v: %w", svcName, timeout, err)
	}

	return ip, nil
}
