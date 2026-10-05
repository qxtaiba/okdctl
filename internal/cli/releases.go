package cli

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/releases"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui"
)

const (
	channelStable = "stable"
	channelAll    = "all"
)

var releasesListChannel string

var releasesCmd = &cobra.Command{
	Use:     "releases",
	Aliases: []string{"release"},
	Short:   "Query available OKD versions",
	Long:    "List and inspect OKD releases resolved from the GitHub releases feed.",
}

var releasesListCmd = &cobra.Command{
	Use:   cmdNameList,
	Short: "List available OKD versions",
	Long: `List OKD versions resolved from the GitHub releases feed.

By default only stable releases are shown; pass --channel=all to include every
non-draft release. Results are served from a 1-hour on-disk cache
(~/.okdctl/cache/okd-versions.json) to avoid repeated network round-trips.`,
	Example: `  okdctl releases list
  okdctl releases list --channel all`,
	Args: cobra.NoArgs,
	RunE: runReleasesList,
}

var releasesShowCmd = &cobra.Command{
	Use:   "show <version>",
	Short: "Show release info for a single OKD version",
	Long: `Print metadata for a single OKD release identified by its full GitHub tag
(e.g. "4.21.3-okd-scos.0"). The version list is resolved from the disk cache;
use --channel=all with 'releases list' to discover pre-release tags.`,
	Example: "  okdctl releases show 4.21.3-okd-scos.0",
	Args:    cobra.ExactArgs(1),
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

	releasesCmd.AddCommand(releasesListCmd)
	releasesCmd.AddCommand(releasesShowCmd)
	rootCmd.AddCommand(releasesCmd)
}

func runReleasesList(cmd *cobra.Command, _ []string) error {
	if err := validateChannel(releasesListChannel); err != nil {
		return err
	}

	versions, err := fetchFlatVersions(cmd.Context())
	if err != nil {
		return err
	}
	if releasesListChannel == channelStable {
		versions = slices.DeleteFunc(versions, func(v releases.OKDVersion) bool { return !v.Stable })
	}
	return printVersionList(cmd.OutOrStdout(), versions)
}

func runReleasesShow(cmd *cobra.Command, args []string) error {
	versions, err := fetchFlatVersions(cmd.Context())
	if err != nil {
		return err
	}

	v, ok := findVersion(versions, args[0])
	if !ok {
		return &errtypes.UsageError{Msg: fmt.Sprintf("version %q not found; try `okdctl releases list --channel all`", args[0])}
	}
	return printVersionDetail(cmd.OutOrStdout(), v)
}

func fetchFlatVersions(ctx context.Context) ([]releases.OKDVersion, error) {
	fetcher := releases.NewOKDVersionFetcher()
	series, err := fetcher.FetchVersions(ctx)
	if err != nil {
		return nil, (&errtypes.NetworkError{Msg: "no releases loaded", Err: err}).WithHint("check your connection")
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

func printVersionList(w io.Writer, versions []releases.OKDVersion) error {
	if len(versions) == 0 {
		_, err := fmt.Fprintln(w, tui.EmptyState("no releases", "try --channel all"))
		return err
	}
	rows := make([][]string, 0, len(versions))
	for _, v := range versions {
		rows = append(rows, []string{
			v.Version,
			v.ReleaseDate.Format("2006-01-02"),
			yesNo(v.Stable),
			v.Type.String(),
		})
	}
	return printTable(w, []string{"VERSION", "RELEASED", "STABLE", "TYPE"}, rows, tui.TableOptions{})
}

func printVersionDetail(w io.Writer, v releases.OKDVersion) error {
	return printLeaders(w, [][2]string{
		{"version", v.Version},
		{"tag", v.Tag},
		{"series", v.ShortVersion()},
		{"released", v.ReleaseDate.Format("2006-01-02")},
		{"stable", yesNo(v.Stable)},
		{"latest-in-series", yesNo(v.Latest)},
		{"release-type", v.Type.String()},
	})
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
