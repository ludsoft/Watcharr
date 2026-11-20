package tmdb

import (
	"testing"
	"time"
)

func TestApplyDiscoverOptionsToMap(t *testing.T) {
	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	o := DiscoverOptions{
		ReleaseDateMin:        d("2010-01-01"),
		ReleaseDateMax:        d("2019-12-31"),
		WithReleaseType:       "2|3",
		PrimaryReleaseDateMin: d("2000-01-01"),
		PrimaryReleaseDateMax: d("2009-12-31"),
	}
	movie := map[string]string{}
	(&TMDB{}).applyDiscoverOptionsToMap(true, o, movie)
	want := map[string]string{
		"release_date.gte":         "2010-01-01",
		"release_date.lte":         "2019-12-31",
		"with_release_type":        "2|3",
		"primary_release_date.gte": "2000-01-01",
		"primary_release_date.lte": "2009-12-31",
	}
	for k, v := range want {
		if movie[k] != v {
			t.Errorf("movie %s = %q, want %q", k, movie[k], v)
		}
	}

	// Shows: first air dates, primary release dates don't exist there.
	tv := map[string]string{}
	(&TMDB{}).applyDiscoverOptionsToMap(false, DiscoverOptions{
		ReleaseDateMin:        d("2020-01-01"),
		PrimaryReleaseDateMin: d("2000-01-01"),
	}, tv)
	if tv["first_air_date.gte"] != "2020-01-01" || len(tv) != 1 {
		t.Errorf("tv params = %v", tv)
	}
}
