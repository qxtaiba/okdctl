package steps

import (
	"testing"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/distribution/okd/releases"
)

func TestDemoReleaseSeriesWellFormed(t *testing.T) {
	series := DemoReleaseSeries()
	if len(series) == 0 {
		t.Fatal("DemoReleaseSeries returned no series")
	}

	for i, s := range series {
		if s.Latest.Version == "" {
			t.Errorf("series[%d] (%d.%d): Latest.Version is empty", i, s.Major, s.Minor)
		}
		if i > 0 && s.Minor >= series[i-1].Minor {
			t.Errorf("series[%d].Minor = %d, want < series[%d].Minor = %d (descending)", i, s.Minor, i-1, series[i-1].Minor)
		}
		for _, v := range s.Versions {
			if err := config.ValidateOKDVersion(v.Version); err != nil {
				t.Errorf("series[%d] (%d.%d): Version %q fails the real okd version validator: %v", i, s.Major, s.Minor, v.Version, err)
			}
		}
	}

	if got := series[0].Latest.Type; got != releases.ReleaseTypeLatestStable {
		t.Errorf("series[0].Latest.Type = %v, want ReleaseTypeLatestStable", got)
	}
}
