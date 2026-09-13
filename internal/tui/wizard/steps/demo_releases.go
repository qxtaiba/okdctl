package steps

import (
	"time"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/releases"
)

func demoDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// DemoReleaseSeries returns a fixed three-minor OKD release catalog for
// deterministic wizard demos and golden-frame tests.
func DemoReleaseSeries() []releases.OKDReleaseSeries {
	series420 := []releases.OKDVersion{
		{Version: "4.20.1", Tag: "4.20.1", ReleaseDate: demoDate(2026, time.August, 1), Stable: true, Latest: true, Type: releases.ReleaseTypeLatestStable},
		{Version: "4.20.0", Tag: "4.20.0", ReleaseDate: demoDate(2026, time.June, 1), Stable: true, Type: releases.ReleaseTypeStable},
	}
	series419 := []releases.OKDVersion{
		{Version: "4.19.4", Tag: "4.19.4", ReleaseDate: demoDate(2026, time.April, 1), Stable: true, Latest: true, Type: releases.ReleaseTypeStable},
		{Version: "4.19.3", Tag: "4.19.3", ReleaseDate: demoDate(2026, time.February, 1), Stable: true, Type: releases.ReleaseTypeStable},
	}
	series418 := []releases.OKDVersion{
		{Version: "4.18.7", Tag: "4.18.7", ReleaseDate: demoDate(2025, time.December, 1), Stable: true, Latest: true, Type: releases.ReleaseTypeLTS},
		{Version: "4.18.6", Tag: "4.18.6", ReleaseDate: demoDate(2025, time.October, 1), Stable: true, Type: releases.ReleaseTypeLTS},
	}

	return []releases.OKDReleaseSeries{
		{Major: 4, Minor: 20, Versions: series420, Latest: series420[0]},
		{Major: 4, Minor: 19, Versions: series419, Latest: series419[0]},
		{Major: 4, Minor: 18, Versions: series418, Latest: series418[0]},
	}
}
