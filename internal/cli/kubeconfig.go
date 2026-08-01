package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
	"github.com/qxtaiba/okdctl/internal/system"
	"github.com/qxtaiba/okdctl/internal/workspace"
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
	Args: cobra.NoArgs,
	RunE: runKubeconfig,
}

func init() {
	kubeconfigCmd.Flags().StringVar(&kubeconfigOutput, flagOutputFile, "-", "write kubeconfig to file, overwriting it if present ('-' for stdout)")
	kubeconfigCmd.Flags().BoolVar(&kubeconfigMerge, "merge", false, "merge into $KUBECONFIG or ~/.kube/config (non-destructive: existing entries preserved)")
	rootCmd.AddCommand(kubeconfigCmd)
}

func runKubeconfig(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveProjectRootOrDie()
	if err != nil {
		return err
	}

	workDir := workspace.WorkDir(projectRoot)
	clusterDir := workspace.ClusterConfigDir(workDir)
	src := workspace.KubeconfigPath(clusterDir)

	if !system.FileExists(src) {
		return &errtypes.ConfigError{
			Msg: fmt.Sprintf("kubeconfig not found at %s; run `okdctl deploy` first", src),
			Err: errtypes.ErrConfigMissing,
		}
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read kubeconfig: %w", err)
	}

	if kubeconfigMerge {
		return mergeKubeconfig(data)
	}

	if kubeconfigOutput == "" || kubeconfigOutput == "-" {
		_, err = cmd.OutOrStdout().Write(data)
		return err
	}

	if err := system.EnsureDirForFile(kubeconfigOutput); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := system.AtomicWrite(kubeconfigOutput, data, 0o600); err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}
	logutil.Info("kubeconfig written", logutil.LF("path", kubeconfigOutput))
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
		merged := mergeNamedList(toKubeEntries(destMap[key]), toKubeEntries(srcMap[key]))
		destMap[key] = fromKubeEntries(merged)
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
	logutil.Info("kubeconfig merged", logutil.LF("path", dest))
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

// kubeEntry is one element of a kubeconfig named list (clusters, users, or
// contexts). json.RawMessage values preserve all fields — including unknown
// extension keys — byte-for-byte through marshal→merge→marshal.
type kubeEntry = map[string]json.RawMessage

// toKubeEntries converts the raw []any produced by sigs.k8s.io/yaml into a
// typed slice via a JSON round-trip so each field is captured verbatim.
func toKubeEntries(v any) []kubeEntry {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var entries []kubeEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil
	}
	return entries
}

// fromKubeEntries converts a typed slice back to []any for storage in the
// top-level map[string]any and marshalling by sigs.k8s.io/yaml.
func fromKubeEntries(entries []kubeEntry) any {
	if entries == nil {
		return nil
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return entries
	}
	var out []any
	if err := json.Unmarshal(b, &out); err != nil {
		return entries
	}
	return out
}

// namedEntries returns the set of names present in a typed entry slice.
func namedEntries(items []kubeEntry) map[string]struct{} {
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		var name string
		if raw, ok := item["name"]; ok && json.Unmarshal(raw, &name) == nil {
			result[name] = struct{}{}
		}
	}
	return result
}

// mergeNamedList appends entries from src into dest, skipping any src entry
// whose name already appears in dest.
func mergeNamedList(dest, src []kubeEntry) []kubeEntry {
	if len(src) == 0 {
		return dest
	}
	existing := namedEntries(dest)
	for _, item := range src {
		var name string
		if raw, ok := item["name"]; ok && json.Unmarshal(raw, &name) == nil {
			if _, exists := existing[name]; !exists {
				dest = append(dest, item)
			}
		}
	}
	return dest
}
