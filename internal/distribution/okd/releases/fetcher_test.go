package releases

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestDeduplicateReleases(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name  string
		input []githubRelease
		want  int
	}{
		{
			name:  "empty",
			input: nil,
			want:  0,
		},
		{
			name: "mixed duplicates",
			input: []githubRelease{
				{TagName: "4.15.0-okd", PublishedAt: now},
				{TagName: "4.16.0-okd", PublishedAt: now},
				{TagName: "4.15.0-okd", PublishedAt: now},
				{TagName: "4.17.0-okd", PublishedAt: now},
				{TagName: "4.16.0-okd", PublishedAt: now},
			},
			want: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deduplicateReleases(tc.input)
			if len(got) != tc.want {
				t.Errorf("deduplicateReleases: got %d entries; want %d", len(got), tc.want)
			}
			seen := make(map[string]bool, len(got))
			for _, r := range got {
				if seen[r.TagName] {
					t.Errorf("duplicate TagName %q in output", r.TagName)
				}
				seen[r.TagName] = true
			}
		})
	}
}

func date(day int) time.Time {
	return time.Date(2024, 10, day, 0, 0, 0, 0, time.UTC)
}

func findVersion(t *testing.T, series []OKDReleaseSeries, tag string) OKDVersion {
	t.Helper()
	for _, s := range series {
		for _, v := range s.Versions {
			if v.Tag == tag {
				return v
			}
		}
	}
	t.Fatalf("tag %q not found in classified series", tag)
	return OKDVersion{}
}

func TestParseReleases_Classification(t *testing.T) {
	releases := []githubRelease{
		{TagName: "4.17.0-okd-scos.ec.9", PublishedAt: date(20)},
		{TagName: "4.17.0-okd-scos.1", PublishedAt: date(15)},
		{TagName: "4.17.0-okd-scos.0", PublishedAt: date(10)},
		{TagName: "4.17.0-okd-scos.2", PublishedAt: date(25), Draft: true},
		{TagName: "4.16.0-okd-scos.rc.3", PublishedAt: date(5)},
		{TagName: "4.16.0-okd-scos.0", PublishedAt: date(3)},
		{TagName: "4.15.0-0.okd-2024-03-10-010116", PublishedAt: date(1)},
		{TagName: "4.4.0-0.okd-2020-05-23-055148-beta5", PublishedAt: date(1)},
		{TagName: "v1.33.0", PublishedAt: date(1)},
	}

	series := parseReleases(releases)

	wantMinors := []int{17, 16, 15, 4}
	if len(series) != len(wantMinors) {
		t.Fatalf("got %d series, want %d: %+v", len(series), len(wantMinors), series)
	}
	for i, want := range wantMinors {
		if series[i].Major != 4 || series[i].Minor != want {
			t.Errorf("series[%d] = %d.%d, want 4.%d", i, series[i].Major, series[i].Minor, want)
		}
	}

	wantTypes := map[string]struct {
		typ    ReleaseType
		stable bool
	}{
		"4.17.0-okd-scos.ec.9":                {ReleaseTypeLatestPreview, false},
		"4.17.0-okd-scos.1":                   {ReleaseTypeLatestStable, true},
		"4.17.0-okd-scos.0":                   {ReleaseTypeStable, true},
		"4.16.0-okd-scos.rc.3":                {ReleaseTypeLatestPreview, false},
		"4.16.0-okd-scos.0":                   {ReleaseTypeStable, true},
		"4.15.0-0.okd-2024-03-10-010116":      {ReleaseTypeLTS, true},
		"4.4.0-0.okd-2020-05-23-055148-beta5": {ReleaseTypeLatestPreview, false},
	}
	total := 0
	for _, s := range series {
		total += len(s.Versions)
	}
	if total != len(wantTypes) {
		t.Errorf("classified %d versions, want %d (draft and non-okd tags must be dropped)", total, len(wantTypes))
	}
	for tag, want := range wantTypes {
		v := findVersion(t, series, tag)
		if v.Type != want.typ {
			t.Errorf("%s type = %v, want %v", tag, v.Type, want.typ)
		}
		if v.Stable != want.stable {
			t.Errorf("%s stable = %v, want %v", tag, v.Stable, want.stable)
		}
	}

	if got := series[0].Latest.Version; got != "4.17.0-okd-scos.1" {
		t.Errorf("newest series Latest = %q; the draft 4.17.0-okd-scos.2 must not win", got)
	}
	if got := series[1].Latest.Version; got != "4.16.0-okd-scos.0" {
		t.Errorf("4.16 series Latest = %q, want the stable build, not the newer rc", got)
	}
}

func TestParseReleases_TagVariantTieBreak(t *testing.T) {
	t.Run("published_at wins over raw tag order", func(t *testing.T) {
		series := parseReleases([]githubRelease{
			{TagName: "4.12.0-0.okd+fcos", PublishedAt: date(20)},
			{TagName: "4.12.0-0.okd+scos", PublishedAt: date(10)},
		})
		if len(series) != 1 || len(series[0].Versions) != 2 {
			t.Fatalf("unexpected shape: %+v", series)
		}
		if got := series[0].Versions[0].Tag; got != "4.12.0-0.okd+fcos" {
			t.Errorf("first version = %q, want the later-published variant", got)
		}
	})

	t.Run("equal published_at falls back to descending raw tag", func(t *testing.T) {
		series := parseReleases([]githubRelease{
			{TagName: "4.12.0-0.okd+fcos", PublishedAt: date(10)},
			{TagName: "4.12.0-0.okd+scos", PublishedAt: date(10)},
		})
		if got := series[0].Versions[0].Tag; got != "4.12.0-0.okd+scos" {
			t.Errorf("first version = %q, want the lexically greater variant", got)
		}
	})
}

func pageFetcher(h http.HandlerFunc) *OKDVersionFetcher {
	return &OKDVersionFetcher{httpClient: &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec.Result(), nil
	})}}
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func writePage(t *testing.T, w http.ResponseWriter, page, n int) {
	t.Helper()
	rels := make([]githubRelease, n)
	for i := range rels {
		rels[i] = githubRelease{TagName: fmt.Sprintf("4.%d.0-okd-scos.%d", page, i)}
	}
	if err := json.NewEncoder(w).Encode(rels); err != nil {
		t.Fatal(err)
	}
}

func TestFetchAllPages_FirstPageFailureFails(t *testing.T) {
	f := pageFetcher(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	got, err := f.fetchAllPages(context.Background(), "example/fixture")
	if err == nil {
		t.Fatal("first-page failure must fail the fetch")
	}
	if len(got) != 0 {
		t.Errorf("got %d releases on first-page failure, want none", len(got))
	}
}

func TestFetchAllPages_LaterPageFailureReturnsPartial(t *testing.T) {
	f := pageFetcher(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("page") != "1" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writePage(t, w, 1, 100)
	})
	got, err := f.fetchAllPages(context.Background(), "example/fixture")
	if err != nil {
		t.Fatalf("later-page failure must return the partial result, got error: %v", err)
	}
	if len(got) != 100 {
		t.Errorf("got %d releases, want the 100 from page 1", len(got))
	}
}

func TestFetchAllPages_ShortPageEndsPagination(t *testing.T) {
	var requests int
	f := pageFetcher(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		writePage(t, w, 1, 3)
	})
	got, err := f.fetchAllPages(context.Background(), "example/fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || requests != 1 {
		t.Errorf("got %d releases in %d requests, want 3 in 1", len(got), requests)
	}
}

func TestFetchAllPages_StopsAtPageCap(t *testing.T) {
	var requests int
	f := pageFetcher(func(w http.ResponseWriter, req *http.Request) {
		requests++
		page, err := strconv.Atoi(req.URL.Query().Get("page"))
		if err != nil {
			t.Errorf("bad page param: %v", err)
		}
		writePage(t, w, page, 100)
	})
	got, err := f.fetchAllPages(context.Background(), "example/fixture")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 10 {
		t.Errorf("made %d requests, want the 10-page safety cap", requests)
	}
	if len(got) != 1000 {
		t.Errorf("got %d releases, want 1000", len(got))
	}
}
