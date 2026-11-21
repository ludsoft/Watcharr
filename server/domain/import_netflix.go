package domain

import "time"

// Fork (ludsoft): data sent with an ImportRequest by the Netflix viewing
// history importer. The client parses `NetflixViewingHistory.csv` and
// groups its rows per movie/show.

// One row of a show in the Netflix history.
type NetflixEpisode struct {
	// Season part of the title as Netflix wrote it (eg: "Season 1",
	// "Part 2", "Limited Series"). Empty for shows without seasons.
	SeasonLabel string    `json:"seasonLabel"`
	Title       string    `json:"title"`
	Date        time.Time `json:"date"`
}

type NetflixImportData struct {
	// Dates the movie was watched (one play each).
	Dates []time.Time `json:"dates"`
	// Episodes watched (for shows).
	Episodes []NetflixEpisode `json:"episodes"`
	// For ambiguous rows ("Name: Something"), imported as a movie first.
	// If no movie is found, they are imported as this show instead.
	FallbackShow *NetflixFallbackShow `json:"fallbackShow"`

	// Rows sharing a prefix but without a season label ("Gotham: Arkham",
	// "Mission: Impossible - Fallout") are grouped as a show item. The server
	// checks the rows' full titles (RowTitles) for exact movie matches first,
	// and asks the client to split the group into movies (NetflixSplit in
	// the response) when they are movies.
	Group     bool     `json:"group"`
	RowTitles []string `json:"rowTitles"`
	// Show names to try in order, for an exact match (eg: "Star Trek:
	// Discovery" then "Star Trek").
	ShowNames []string `json:"showNames"`
}

type NetflixFallbackShow struct {
	Name     string           `json:"name"`
	Episodes []NetflixEpisode `json:"episodes"`
}
