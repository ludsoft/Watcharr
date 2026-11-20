package discover

import (
	"testing"
	"time"

	"github.com/sbondCo/Watcharr/domain"
)

func TestValidateFilters(t *testing.T) {
	ok := []domain.DiscoverRequest{
		{Type: domain.SearchTypeMulti, Filter: domain.DiscoverFilterTrending}, // no filters
		{Type: domain.SearchTypeMovie, Filter: domain.DiscoverFilterPopular, YearFrom: 2010, YearTo: 2019, Country: "FR"},
		{Type: domain.SearchTypeMovie, Filter: domain.DiscoverFilterPopular, Country: "FR"},
		{Type: domain.SearchTypeShow, Filter: domain.DiscoverFilterPopular, YearFrom: 2020},
		{Type: domain.SearchTypeShow, Filter: domain.DiscoverFilterPopular, YearFrom: 2020, YearTo: 2020},
	}
	for _, r := range ok {
		if err := validateFilters(r); err != nil {
			t.Errorf("%+v: unexpected error %v", r, err)
		}
	}
	bad := []domain.DiscoverRequest{
		{Type: domain.SearchTypeMovie, Filter: domain.DiscoverFilterTrending, YearFrom: 2010},
		{Type: domain.SearchTypeMovie, Filter: domain.DiscoverFilterUpcoming, Country: "FR"},
		{Type: domain.SearchTypeMulti, Filter: domain.DiscoverFilterPopular, YearFrom: 2010},
		{Type: domain.SearchTypeGame, Filter: domain.DiscoverFilterPopular, YearFrom: 2010},
		{Type: domain.SearchTypeShow, Filter: domain.DiscoverFilterPopular, Country: "FR"},
		{Type: domain.SearchTypeMovie, Filter: domain.DiscoverFilterPopular, YearFrom: 2020, YearTo: 2019},
	}
	for _, r := range bad {
		if err := validateFilters(r); err == nil {
			t.Errorf("%+v: expected an error", r)
		}
	}
}

func day(o time.Time) string {
	if o.IsZero() {
		return ""
	}
	return o.Format("2006-01-02")
}

func TestPopularMovieOptions(t *testing.T) {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	popular := domain.DiscoverRequest{Type: domain.SearchTypeMovie, Filter: domain.DiscoverFilterPopular}

	// No filters: unchanged behaviour (users region, no dates).
	o, region := popularMovieOptions(popular, "US", now)
	if region != "US" || !o.ReleaseDateMin.IsZero() || !o.PrimaryReleaseDateMin.IsZero() || o.WithReleaseType != "" {
		t.Errorf("no filters: %+v region %s", o, region)
	}

	// Years only: original release date.
	r := popular
	r.YearFrom, r.YearTo = 2010, 2019
	o, region = popularMovieOptions(r, "US", now)
	if region != "US" || day(o.PrimaryReleaseDateMin) != "2010-01-01" || day(o.PrimaryReleaseDateMax) != "2019-12-31" || !o.ReleaseDateMin.IsZero() {
		t.Errorf("years only: %+v region %s", o, region)
	}

	// Country + years: theatrical releases in that country.
	r.Country = "FR"
	o, region = popularMovieOptions(r, "US", now)
	if region != "FR" || o.WithReleaseType != "2|3" || day(o.ReleaseDateMin) != "2010-01-01" || day(o.ReleaseDateMax) != "2019-12-31" || !o.PrimaryReleaseDateMin.IsZero() {
		t.Errorf("country + years: %+v region %s", o, region)
	}

	// Country only: released there up to now.
	r = popular
	r.Country = "JP"
	o, region = popularMovieOptions(r, "US", now)
	if region != "JP" || !o.ReleaseDateMin.IsZero() || day(o.ReleaseDateMax) != "2026-11-20" {
		t.Errorf("country only: %+v region %s", o, region)
	}
}

func TestPopularShowOptions(t *testing.T) {
	o := popularShowOptions(domain.DiscoverRequest{YearFrom: 2020})
	if day(o.ReleaseDateMin) != "2020-01-01" || !o.ReleaseDateMax.IsZero() || !o.PrimaryReleaseDateMin.IsZero() {
		t.Errorf("show options: %+v", o)
	}
}
