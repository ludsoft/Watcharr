package domain

import (
	"github.com/go-playground/validator/v10"
	"github.com/sbondCo/Watcharr/util"
)

type DiscoverFilter string

const (
	// Generic "What's Trending?" (all)
	DiscoverFilterTrending DiscoverFilter = "trending"
	// Popular stuff (basically trending but over months instead of just a day).
	DiscoverFilterPopular DiscoverFilter = "popular"
	// Upcoming content (all).
	DiscoverFilterUpcoming DiscoverFilter = "upcoming"
	// What's streaming (movies/tv).
	DiscoverFilterStreaming DiscoverFilter = "streaming"
	// What's in theatres (movies).
	DiscoverFilterInTheatres DiscoverFilter = "intheatres"
)

type DiscoverRequest struct {
	// The type of content we want to discover.
	// Reusing the SearchType enum here, but if this needs to diverge,
	// then make our own enum in this file.
	Type SearchType `form:"type" binding:"validsearchtype"`
	// A main filter.
	// Not every `Type` of discover will support all Filters (service funcs
	// will error individually based on what they support).
	Filter DiscoverFilter `form:"filter" binding:"validdiscoverfilter"`

	// Optional filters, only supported for the popular filter of movies
	// and shows (the service validates this).

	// Released from this year (inclusive).
	YearFrom int `form:"yearFrom" binding:"omitempty,min=1870,max=2200"`
	// Released until this year (inclusive).
	YearTo int `form:"yearTo" binding:"omitempty,min=1870,max=2200"`
	// Released (in theatres) in this country (ISO 3166-1). Movies only.
	Country string `form:"country" binding:"omitempty,iso3166_1_alpha2"`
}

// Are any of the optional filters set?
func (r DiscoverRequest) HasFilters() bool {
	return r.YearFrom != 0 || r.YearTo != 0 || r.Country != ""
}

// Extra data that we provide to the Discover service func.
type DiscoverRequestMeta struct {
	PageParams util.PaginationParams
	Region     string
}

type DiscoverResponse struct {
	util.PaginationResponse[Media, util.None]
}

var ValidDiscoverFilter validator.Func = func(fl validator.FieldLevel) bool {
	st, ok := fl.Field().Interface().(DiscoverFilter)
	if ok {
		switch st {
		case DiscoverFilterTrending,
			DiscoverFilterPopular,
			DiscoverFilterUpcoming,
			DiscoverFilterStreaming,
			DiscoverFilterInTheatres:
			return true
		}
	}
	return false
}
