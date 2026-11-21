package domain

import (
	"errors"
	"time"

	"github.com/sbondCo/Watcharr/database/entity"
	"github.com/sbondCo/Watcharr/util"
)

type ImportContentType string

const (
	ImportContentTypeMovie       ImportContentType = "movie"
	ImportContentTypeShow        ImportContentType = "tv"
	ImportContentTypeShowEpisode ImportContentType = "tv_episode"
	ImportContentTypeGame        ImportContentType = "game"
)

func ImportContentTypeToSearchType(t ImportContentType) SearchType {
	switch t {
	case ImportContentTypeMovie:
		return SearchTypeMovie
	case ImportContentTypeShow:
		return SearchTypeShow
	case ImportContentTypeGame:
		return SearchTypeGame
	}
	// Empty string should be caught as an error.
	return ""
}

type ImportResponseType string

var (
	// Successful import
	IMPORT_SUCCESS ImportResponseType = "IMPORT_SUCCESS"
	// Import failed for reasons user cant fix
	IMPORT_FAILED ImportResponseType = "IMPORT_FAILED"
	// Import query returned multiple results, user must decide
	IMPORT_MULTI ImportResponseType = "IMPORT_MULTI"
	// Import query returned zero results, user must provide more info
	IMPORT_NOTFOUND ImportResponseType = "IMPORT_NOTFOUND"
	// Item already exists so couldn't import (unique constraint hit when adding)
	IMPORT_EXISTS ImportResponseType = "IMPORT_EXISTS"
)

type ImportRequest struct {
	TmdbID int    `json:"tmdbId"`
	ImdbID string `json:"imdbId"`
	IgdbID int    `json:"igdbId"`

	Name             string                  `json:"name"`
	Year             int                     `json:"year"`
	Type             ImportContentType       `json:"type"`
	Rating           float64                 `json:"rating" binding:"max=10"`
	RatingCustomDate *time.Time              `json:"ratingCustomDate"`
	Status           entity.WatchedStatus    `json:"status"`
	Thoughts         string                  `json:"thoughts"`
	DatesWatched     []time.Time             `json:"datesWatched"`
	Activity         []entity.Activity       `json:"activity"`
	WatchedEpisodes  []entity.WatchedEpisode `json:"watchedEpisodes"`
	WatchedSeason    []entity.WatchedSeason  `json:"watchedSeasons"`
	Tags             []TagAddRequest         `json:"tags"`

	// Fork (ludsoft): set by the Netflix importer, see import_netflix.go.
	Netflix *NetflixImportData `json:"netflix"`
}

// Internal struct given to the SuccessfulImport function.
type SuccessfulImportProps struct {
	TmdbID      int
	IgdbID      int
	ContentType util.SupportedMedia
}

func NewSuccessfulImportPropsFromMedia(m *Media) (SuccessfulImportProps, error) {
	p := SuccessfulImportProps{ContentType: m.GetMediaType()}
	switch p.ContentType {
	case util.SupportedMediaMovie, util.SupportedMediaShow:
		p.TmdbID = m.IDs.TMDB
	case util.SupportedMediaGame:
		p.IgdbID = m.IDs.IGDB
	default:
		return p, errors.New("unsupported content type on media")
	}
	return p, nil
}

type ImportResponse struct {
	Type    ImportResponseType `json:"type"`
	Results []Media            `json:"results,omitempty"`
	Match   Media              `json:"match,omitzero"`
	// On success this will be filled with the new watched entry
	WatchedEntry entity.Watched `json:"watchedEntry,omitzero"`
	// Fork (ludsoft): Netflix import, episodes that couldn't be matched
	// with an episode on TMDB.
	UnmatchedEpisodes []NetflixEpisode `json:"unmatchedEpisodes,omitempty"`
	// Fork (ludsoft): Netflix import, the grouped rows are movies: the client
	// should import them as separate movies.
	NetflixSplit bool `json:"netflixSplit,omitempty"`
}
