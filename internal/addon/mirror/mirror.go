// Package mirror discovers container image references from Helm charts
// declared by MirrorableAddon implementations.
package mirror

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/addon"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/executor"
)

// ChartImages runs helm template against ref and returns the deduplicated
// set of container image refs found in the rendered manifests. An empty
// Version omits --version so helm resolves the latest chart version.
//
// Caller must ensure helm is on PATH before calling.
func ChartImages(ctx context.Context, exec *executor.Executor, ref addon.ChartRef) ([]string, error) {
	if !executor.CommandExists("helm") {
		return nil, &errtypes.ConfigError{Msg: "helm is required for chart image discovery but was not found in PATH"}
	}

	args := []string{"template", "mirror-probe", ref.OCIRef}
	if ref.Version != "" {
		args = append(args, "--version", ref.Version)
	}

	result, err := exec.RunChecked(ctx, "helm", args...)
	if err != nil {
		return nil, &errtypes.NetworkError{
			Msg: fmt.Sprintf("helm template failed for %s", ref.OCIRef),
			Err: err,
		}
	}

	return extractImages(result.Stdout), nil
}

// SpecImages returns all container image refs declared by spec, expanding
// each ChartRef through ChartImages and deduplicating across charts and
// StaticImages.
func SpecImages(ctx context.Context, exec *executor.Executor, spec addon.MirrorSpec) ([]string, error) {
	seen := make(map[string]bool)
	var all []string

	for _, ref := range spec.Charts {
		imgs, err := ChartImages(ctx, exec, ref)
		if err != nil {
			return nil, err
		}
		for _, img := range imgs {
			if !seen[img] {
				seen[img] = true
				all = append(all, img)
			}
		}
	}
	for _, img := range spec.StaticImages {
		if !seen[img] {
			seen[img] = true
			all = append(all, img)
		}
	}
	return all, nil
}

// AddonImages returns the deduplicated image inventory keyed by addon name
// for every MirrorableAddon implementation in addons.
func AddonImages(ctx context.Context, exec *executor.Executor, addons []addon.Addon) (map[string][]string, error) {
	result := make(map[string][]string)
	for _, a := range addons {
		m, ok := a.(addon.MirrorableAddon)
		if !ok {
			continue
		}
		spec := m.MirrorArtifacts()
		if len(spec.Charts) == 0 && len(spec.StaticImages) == 0 {
			continue
		}
		imgs, err := SpecImages(ctx, exec, spec)
		if err != nil {
			return nil, fmt.Errorf("addon %s: %w", a.Info().Name, err)
		}
		result[a.Info().Name] = imgs
	}
	return result, nil
}

// extractImages walks parsed YAML documents and collects every "image" key
// whose value is a non-empty string. Walks the structure rather than
// grepping so indentation variants and multi-document streams parse safely.
func extractImages(rendered string) []string {
	seen := make(map[string]bool)
	var images []string

	for _, doc := range strings.Split(rendered, "\n---") {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		var obj map[string]any
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			continue
		}
		collectImages(obj, seen, &images)
	}
	return images
}

func collectImages(node any, seen map[string]bool, out *[]string) {
	switch v := node.(type) {
	case map[string]any:
		for key, val := range v {
			if key == "image" {
				if img, ok := val.(string); ok && img != "" && !seen[img] {
					seen[img] = true
					*out = append(*out, img)
				}
			}
			collectImages(val, seen, out)
		}
	case []any:
		for _, elem := range v {
			collectImages(elem, seen, out)
		}
	}
}
