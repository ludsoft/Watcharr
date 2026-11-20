package discover

import (
	"time"

	"github.com/sbondCo/Watcharr/domain"
	"github.com/sbondCo/Watcharr/media/tmdb"
)

// Errors caused by the request, safe to show to the user.
type filterError string

func (e filterError) Error() string { return string(e) }

// Validate the optional year/country filters of a discover request.
// They are only supported for popular movies (year + country) and popular
// shows (year only, tmdb has no per country air dates for shows).
func validateFilters(r domain.DiscoverRequest) error {
	if !r.HasFilters() {
		return nil
	}
	if r.Filter != domain.DiscoverFilterPopular ||
		(r.Type != domain.SearchTypeMovie && r.Type != domain.SearchTypeShow) {
		return filterError("year and country filters are only supported for popular movies and shows")
	}
	if r.Type == domain.SearchTypeShow && r.Country != "" {
		return filterError("the country filter is only supported for movies")
	}
	if r.YearFrom != 0 && r.YearTo != 0 && r.YearFrom > r.YearTo {
		return filterError("year from must be before year to")
	}
	return nil
}

func yearStart(y int) time.Time {
	if y == 0 {
		return time.Time{}
	}
	return time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
}

func yearEnd(y int) time.Time {
	if y == 0 {
		return time.Time{}
	}
	return time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC)
}

// Tmdb options (and region) for popular movies.
//
//   - With a country: movies with a theatrical release (types 2|3, so
//     re-releases count) in that country within the years (or up to now
//     when no end year). tmdb applies release_date.* to the `region`'s
//     release dates.
//   - Without a country: the years apply to the original release date.
func popularMovieOptions(r domain.DiscoverRequest, userRegion string, now time.Time) (tmdb.DiscoverOptions, string) {
	if r.Country != "" {
		o := tmdb.DiscoverOptions{
			WithReleaseType: "2|3",
			ReleaseDateMin:  yearStart(r.YearFrom),
			ReleaseDateMax:  yearEnd(r.YearTo),
		}
		if o.ReleaseDateMax.IsZero() {
			o.ReleaseDateMax = now
		}
		return o, r.Country
	}
	return tmdb.DiscoverOptions{
		PrimaryReleaseDateMin: yearStart(r.YearFrom),
		PrimaryReleaseDateMax: yearEnd(r.YearTo),
	}, userRegion
}

// Tmdb options for popular shows: years apply to the first air date.
func popularShowOptions(r domain.DiscoverRequest) tmdb.DiscoverOptions {
	return tmdb.DiscoverOptions{
		ReleaseDateMin: yearStart(r.YearFrom),
		ReleaseDateMax: yearEnd(r.YearTo),
	}
}
