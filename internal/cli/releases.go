package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/releases"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

const (
	channelStable = "stable"
	channelAll    = "all"
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
	Long: `List OKD versions resolved from the GitHub releases feed.

By default only stable releases are shown; pass --channel=all to include every
non-draft release. Results are served from a 1-hour on-disk cache
(~/.okdctl/cache/okd-versions.json) to avoid repeated network round-trips.`,
	Example: `  okdctl releases list
  okdctl releases list --channel all
  okdctl releases list --output json`,
	RunE: runReleasesList,
}

// releasesShowCmd prints release info for a single version matching either
// the version string ("4.15.0") or the GitHub tag.
var releasesShowCmd = &cobra.Command{
	Use:   "show <version>",
	Short: "Show release info for a single OKD version",
	Long: `Print metadata for a single OKD release identified by its version string
("4.21.3") or GitHub tag. The version list is resolved from the disk cache;
use --channel=all with 'releases list' to discover pre-release tags.`,
	Example: `  okdctl releases show 4.21.3
  okdctl releases show 4.21.3 --output json`,
	Args: cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		fetcher := releases.NewOKDVersionFetcher()
		series, err := fetcher.FetchVersions(cmd.Context())
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var versions []string
		for _, s := range series {
			for _, v := range s.Versions {
				versions = append(versions, v.Version)
			}
		}
		return versions, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: runReleasesShow,
}

func init() {
	releasesListCmd.Flags().StringVar(&releasesListChannel, "channel", channelStable,
		"filter versions: stable|all")
	_ = releasesListCmd.RegisterFlagCompletionFunc("channel", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{channelStable, channelAll}, cobra.ShellCompDirectiveNoFileComp
	})
	releasesListCmd.Flags().StringVarP(&releasesListOutput, flagOutput, flagOutputShort, outputText,
		"output format: text|json")
	releasesShowCmd.Flags().StringVarP(&releasesShowOutput, flagOutput, flagOutputShort, outputText,
		"output format: text|json")

	releasesCmd.AddCommand(releasesListCmd)
	releasesCmd.AddCommand(releasesShowCmd)
	rootCmd.AddCommand(releasesCmd)
}

func runReleasesList(cmd *cobra.Command, _ []string) error {
	if err := validateChannel(releasesListChannel); err != nil {
		return err
	}
	if err := validateFormat(releasesListOutput); err != nil {
		return err
	}
	quietForJSON(releasesListOutput)

	versions, err := fetchFlatVersions(cmd.Context())
	if err != nil {
		return err
	}
	if releasesListChannel == channelStable {
		versions = slices.DeleteFunc(versions, func(v releases.OKDVersion) bool { return !v.Stable })
	}

	if releasesListOutput == outputJSON {
		return writeJSON(cmd.OutOrStdout(), versions)
	}
	return printVersionList(cmd.OutOrStdout(), versions)
}

func runReleasesShow(cmd *cobra.Command, args []string) error {
	if err := validateFormat(releasesShowOutput); err != nil {
		return err
	}
	quietForJSON(releasesShowOutput)

	versions, err := fetchFlatVersions(cmd.Context())
	if err != nil {
		return err
	}

	v, ok := findVersion(versions, args[0])
	if !ok {
		return &errtypes.UsageError{Msg: fmt.Sprintf("version %q not found; try `okdctl releases list --channel all`", args[0])}
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
		return nil, &errtypes.NetworkError{Msg: "fetch OKD versions", Err: err}
	}
	out := make([]releases.OKDVersion, 0, len(series))
	for _, s := range series {
		out = append(out, s.Versions...)
	}
	return out, nil
}

func findVersion(versions []releases.OKDVersion, query string) (releases.OKDVersion, bool) {
	query = strings.TrimSpace(query)
	i := slices.IndexFunc(versions, func(v releases.OKDVersion) bool {
		return v.Version == query || v.Tag == query
	})
	if i < 0 {
		return releases.OKDVersion{}, false
	}
	return versions[i], true
}

func validateChannel(ch string) error {
	switch ch {
	case channelStable, channelAll:
		return nil
	default:
		return &errtypes.UsageError{Msg: fmt.Sprintf("invalid --channel %q (want stable|all)", ch)}
	}
}

func validateFormat(format string) error {
	switch format {
	case outputText, outputJSON:
		return nil
	default:
		return &errtypes.UsageError{Msg: fmt.Sprintf("invalid --output %q (want text|json)", format)}
	}
}

func printVersionList(w io.Writer, versions []releases.OKDVersion) error {
	if len(versions) == 0 {
		_, err := fmt.Fprintln(w, "no versions found")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "VERSION\tRELEASED\tSTABLE\tTYPE")
	for _, v := range versions {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			v.Version,
			v.ReleaseDate.Format("2006-01-02"),
			yesNo(v.Stable),
			v.Type.String(),
		)
	}
	return tw.Flush()
}

func printVersionDetail(w io.Writer, v releases.OKDVersion) error {
	lines := []struct{ k, val string }{
		{"version", v.Version},
		{"tag", v.Tag},
		{"series", v.ShortVersion()},
		{"released", v.ReleaseDate.Format("2006-01-02")},
		{"stable", yesNo(v.Stable)},
		{"latest-in-series", yesNo(v.Latest)},
		{"release-type", v.Type.String()},
	}
	for _, ln := range lines {
		fmt.Fprintln(w, tui.DottedKeyValueFull(ln.k, ln.val, tui.DefaultKeyColWidth, 0))
	}
	return nil
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
