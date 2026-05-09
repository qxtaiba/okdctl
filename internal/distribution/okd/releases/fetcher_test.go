package releases

import (
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
			name: "no duplicates",
			input: []githubRelease{
				{TagName: "4.15.0-okd", PublishedAt: now},
				{TagName: "4.16.0-okd", PublishedAt: now},
			},
			want: 2,
		},
		{
			name: "all same tag",
			input: []githubRelease{
				{TagName: "4.15.0-okd", PublishedAt: now},
				{TagName: "4.15.0-okd", PublishedAt: now},
				{TagName: "4.15.0-okd", PublishedAt: now},
			},
			want: 1,
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

	f := &OKDVersionFetcher{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := f.deduplicateReleases(tc.input)
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
