package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/releases"
	"github.com/qxtaiba/okdctl/internal/tui"
)

const (
	channelStable = "stable"
	channelAll    = "all"
	outputText    = "text"
	outputJSON    = "json"
)

var (
	releasesListChannel string
	releasesListOutput  string
	releasesShowOutput  string
)

// releasesCmd groups the read-only subcommands that query the OKD releases
// feed, backed by the fetcher's disk cache.
var releasesCmd = &cobra.Command{
	Use:   "releases",
	Short: "Query available OKD versions",
	Long:  "List and inspect OKD releases resolved from the GitHub releases feed.",
}

// releasesListCmd prints available versions, filtered to Stable=true by
// default or every non-draft release when --channel=all.
var releasesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available OKD versions",
	RunE:  runReleasesList,
}

// releasesShowCmd prints release info for a single version matching either
// the version string ("4.15.0") or the GitHub tag.
var releasesShowCmd = &cobra.Command{
	Use:   "show <version>",
	Short: "Show release info for a single OKD version",
	Args:  cobra.ExactArgs(1),
	RunE:  runReleasesShow,
}

func init() {
	releasesListCmd.Flags().StringVar(&releasesListChannel, "channel", channelStable,
		"filter versions: stable|all")
	releasesListCmd.Flags().StringVar(&releasesListOutput, "output", outputText,
		"output format: text|json")
	releasesShowCmd.Flags().StringVar(&releasesShowOutput, "output", outputText,
		"output format: text|json")

	releasesCmd.AddCommand(releasesListCmd)
	releasesCmd.AddCommand(releasesShowCmd)
	rootCmd.AddCommand(releasesCmd)
}

func runReleasesList(cmd *cobra.Command, _ []string) error {
	if err := validateChannel(releasesListChannel); err != nil {
		return err
	}
	if err := validateOutput(releasesListOutput); err != nil {
		return err
	}

	versions, err := fetchFlatVersions(cmd.Context())
	if err != nil {
		return err
	}
	if releasesListChannel == channelStable {
		versions = filterStable(versions)
	}

	if releasesListOutput == outputJSON {
		return writeJSON(cmd.OutOrStdout(), versions)
	}
	return printVersionList(cmd.OutOrStdout(), versions)
}

func runReleasesShow(cmd *cobra.Command, args []string) error {
	if err := validateOutput(releasesShowOutput); err != nil {
		return err
	}

	versions, err := fetchFlatVersions(cmd.Context())
	if err != nil {
		return err
	}

	v, ok := findVersion(versions, args[0])
	if !ok {
		return fmt.Errorf("version %q not found; try `okdctl releases list --channel all`", args[0])
	}

	if releasesShowOutput == outputJSON {
		return writeJSON(cmd.OutOrStdout(), v)
	}
	return printVersionDetail(cmd.OutOrStdout(), v)
}

func fetchFlatVersions(ctx context.Context) ([]releases.OKDVersion, error) {
	fetcher := releases.NewOKDVersionFetcher()
	series, err := fetcher.FetchVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch OKD versions: %w", err)
	}
	var out []releases.OKDVersion
	for _, s := range series {
		out = append(out, s.Versions...)
	}
	return out, nil
}

func filterStable(versions []releases.OKDVersion) []releases.OKDVersion {
	out := make([]releases.OKDVersion, 0, len(versions))
	for _, v := range versions {
		if v.Stable {
			out = append(out, v)
		}
	}
	return out
}

func findVersion(versions []releases.OKDVersion, query string) (releases.OKDVersion, bool) {
	query = strings.TrimSpace(query)
	for _, v := range versions {
		if v.Version == query || v.Tag == query {
			return v, true
		}
	}
	return releases.OKDVersion{}, false
}

func validateChannel(ch string) error {
	switch ch {
	case channelStable, channelAll:
		return nil
	default:
		return fmt.Errorf("invalid --channel %q (want stable|all)", ch)
	}
}

func validateOutput(format string) error {
	switch format {
	case outputText, outputJSON:
		return nil
	default:
		return fmt.Errorf("invalid --output %q (want text|json)", format)
	}
}

func printVersionList(w io.Writer, versions []releases.OKDVersion) error {
	if len(versions) == 0 {
		_, err := fmt.Fprintln(w, "no versions found")
		return err
	}
	fmt.Fprintf(w, "%-12s  %-10s  %-6s  %s\n", "VERSION", "RELEASED", "STABLE", "TYPE")
	for _, v := range versions {
		fmt.Fprintf(w, "%-12s  %-10s  %-6s  %s\n",
			v.Version,
			v.ReleaseDate.Format("2006-01-02"),
			yesNo(v.Stable),
			releaseTypeLabel(v.Type),
		)
	}
	return nil
}

func printVersionDetail(w io.Writer, v releases.OKDVersion) error {
	lines := []struct{ k, val string }{
		{"version", v.Version},
		{"tag", v.Tag},
		{"series", v.ShortVersion()},
		{"released", v.ReleaseDate.Format("2006-01-02")},
		{"stable", yesNo(v.Stable)},
		{"latest-in-series", yesNo(v.Latest)},
		{"release-type", releaseTypeLabel(v.Type)},
	}
	for _, ln := range lines {
		fmt.Fprintln(w, tui.DottedKeyValueFull(ln.k, ln.val, tui.DefaultKeyColWidth, 0))
	}
	return nil
}

func releaseTypeLabel(t releases.ReleaseType) string {
	switch t {
	case releases.ReleaseTypeStable:
		return "stable"
	case releases.ReleaseTypeLatestStable:
		return "latest-stable"
	case releases.ReleaseTypePreview:
		return "preview"
	case releases.ReleaseTypeLatestPreview:
		return "latest-preview"
	case releases.ReleaseTypeLTS:
		return "lts"
	default:
		return "unknown"
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
