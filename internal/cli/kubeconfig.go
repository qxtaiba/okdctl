package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/system"
)

var (
	kubeconfigOutput string
	kubeconfigMerge  bool
)

var kubeconfigCmd = &cobra.Command{
	Use:   "kubeconfig",
	Short: "Print or export the cluster kubeconfig",
	Long: `Print the cluster kubeconfig to stdout, write it to a file,
or merge it into an existing kubeconfig.`,
	Example: `  okdctl kubeconfig                       # print to stdout
  okdctl kubeconfig --output-file ~/.kube/okd.cfg    # write to file
  okdctl kubeconfig --merge               # merge into $KUBECONFIG`,
	RunE: runKubeconfig,
}

func init() {
	kubeconfigCmd.Flags().StringVar(&kubeconfigOutput, flagOutputFile, "-", "write kubeconfig to file ('-' for stdout)")
	kubeconfigCmd.Flags().BoolVar(&kubeconfigMerge, "merge", false, "merge into $KUBECONFIG or ~/.kube/config (non-destructive: existing entries preserved)")
	rootCmd.AddCommand(kubeconfigCmd)
}

func runKubeconfig(_ *cobra.Command, _ []string) error {
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	workDir := filepath.Join(projectRoot, "okd-install")
	clusterDir := phase.ClusterConfigDir(workDir)
	src := filepath.Join(clusterDir, "auth", "kubeconfig")

	if !system.FileExists(src) {
		return fmt.Errorf("kubeconfig not found at %s; run `okdctl deploy` first", src)
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read kubeconfig: %w", err)
	}

	if kubeconfigMerge {
		return mergeKubeconfig(data)
	}

	if kubeconfigOutput == "" || kubeconfigOutput == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}

	if err := system.EnsureDirForFile(kubeconfigOutput); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := system.AtomicWrite(kubeconfigOutput, data, 0o600); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}
	fmt.Fprintf(os.Stderr, "kubeconfig written to %s\n", kubeconfigOutput)
	return nil
}

// mergeKubeconfig merges srcData into the first path listed in $KUBECONFIG,
// falling back to ~/.kube/config. Clusters, users, and contexts are merged
// by name; existing entries are not overwritten. current-context is set from
// src only when the destination has no current-context set.
func mergeKubeconfig(srcData []byte) error {
	dest := mergeTargetPath()

	var srcMap map[string]any
	if err := yaml.Unmarshal(srcData, &srcMap); err != nil {
		return fmt.Errorf("parse source kubeconfig: %w", err)
	}

	var destMap map[string]any
	if system.FileExists(dest) {
		raw, err := os.ReadFile(dest)
		if err != nil {
			return fmt.Errorf("read destination kubeconfig %s: %w", dest, err)
		}
		if err := yaml.Unmarshal(raw, &destMap); err != nil {
			return fmt.Errorf("parse destination kubeconfig %s: %w", dest, err)
		}
	}
	if destMap == nil {
		destMap = map[string]any{}
	}

	for _, key := range []string{"clusters", "users", "contexts"} {
		destMap[key] = mergeNamedList(destMap[key], srcMap[key])
	}

	if cc, ok := srcMap["current-context"]; ok {
		if existing, _ := destMap["current-context"].(string); existing == "" {
			destMap["current-context"] = cc
		}
	}

	out, err := yaml.Marshal(destMap)
	if err != nil {
		return fmt.Errorf("marshal merged kubeconfig: %w", err)
	}

	if err := system.EnsureDirForFile(dest); err != nil {
		return fmt.Errorf("create .kube directory: %w", err)
	}
	if err := system.AtomicWrite(dest, out, 0o600); err != nil {
		return fmt.Errorf("write merged kubeconfig: %w", err)
	}
	fmt.Fprintf(os.Stderr, "kubeconfig merged into %s\n", dest)
	return nil
}

func mergeTargetPath() string {
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		if first, _, found := strings.Cut(kc, string(filepath.ListSeparator)); found {
			return first
		}
		return kc
	}
	home, err := system.InvokingUserHomeDir()
	if err != nil {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".kube", "config")
}

// namedEntries converts a raw YAML list ([]any of map[string]any) into a
// name→item map. Entries without a string "name" key are skipped.
func namedEntries(v any) map[string]any {
	items, _ := v.([]any)
	result := make(map[string]any, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			if name, ok := m["name"].(string); ok {
				result[name] = item
			}
		}
	}
	return result
}

// mergeNamedList appends entries from src into dest, skipping any src entry
// whose .name already appears in dest. Both arguments are the raw YAML
// unmarshalled representation ([]any of map[string]any).
func mergeNamedList(dest, src any) any {
	destSlice, _ := dest.([]any)
	srcSlice, _ := src.([]any)
	if len(srcSlice) == 0 {
		return dest
	}

	existing := namedEntries(dest)
	for _, item := range srcSlice {
		if m, ok := item.(map[string]any); ok {
			if name, ok := m["name"].(string); ok && existing[name] == nil {
				destSlice = append(destSlice, item)
			}
		}
	}
	return destSlice
}
