package analytics

import (
	"errors"
	"log/slog"
	"sort"
	"time"

	"github.com/sbondCo/Watcharr/database/entity"
)

var ErrInvalidDate = errors.New("invalid date, expected YYYY-MM-DD")

type DayRequest struct {
	// Day to get the details of (YYYY-MM-DD), in the users timezone.
	Date string `form:"date" binding:"required"`
	// IANA timezone name of the user (eg: Europe/Paris). Defaults to UTC.
	TZ string `form:"tz"`
}

type DayEpisode struct {
	Season  int `json:"season"`
	Episode int `json:"episode"`
}

// One watched item of the day, with everything watched of it that day.
type DayEntry struct {
	WatchedID uint `json:"watchedId"`
	// "movie", "tv" or "game" (same as the frontend routes).
	Type string `json:"type"`
	// TMDB id for movies/shows, IGDB id for games.
	ID    int    `json:"id"`
	Title string `json:"title"`
	// Local poster path, relative to the api base url. Empty if none.
	PosterPath string `json:"posterPath,omitempty"`
	// Number of plays (movies and games).
	Plays int `json:"plays,omitempty"`
	// Episodes finished that day (shows), sorted.
	Episodes []DayEpisode `json:"episodes,omitempty"`
}

type DayResponse struct {
	Date string `json:"date"`
	// Ordered by the first watch of the day.
	Entries []DayEntry `json:"entries"`
}

// GetDay lists what userId watched on a day. Uses the same events as Get, so
// it always matches the activity history counts of that day.
func (s *Service) GetDay(userId uint, req DayRequest) (DayResponse, error) {
	loc := location(req.TZ)
	start, err := time.ParseInLocation(dayFmt, req.Date, loc)
	if err != nil {
		return DayResponse{}, ErrInvalidDate
	}
	end := start.AddDate(0, 0, 1)

	events, err := s.loadEvents(userId)
	if err != nil {
		return DayResponse{}, err
	}
	events = eventsBetween(events, start, end)

	ids := make([]uint, 0, len(events))
	for _, e := range events {
		ids = append(ids, e.watchedID)
	}
	watcheds := []entity.Watched{}
	if len(ids) > 0 {
		err = s.db.
			Preload("Content").
			Preload("Game.Poster").
			Where("user_id = ? AND id IN ?", userId, ids).
			Find(&watcheds).Error
		if err != nil {
			slog.Error("analytics.GetDay: Watched query failed", "error", err)
			return DayResponse{}, errors.New("failed to get watched items")
		}
	}
	byID := make(map[uint]*entity.Watched, len(watcheds))
	for i := range watcheds {
		byID[watcheds[i].ID] = &watcheds[i]
	}
	return DayResponse{Date: req.Date, Entries: dayEntries(events, byID)}, nil
}

// Dated viewing events (the ones shown in the activity history) in
// [start, end), sorted by time.
func eventsBetween(events []event, start, end time.Time) []event {
	r := []event{}
	for _, e := range events {
		if e.undated || e.kind == kindShow || e.at.Before(start) || !e.at.Before(end) {
			continue
		}
		r = append(r, e)
	}
	sort.SliceStable(r, func(i, j int) bool { return r[i].at.Before(r[j].at) })
	return r
}

// Group sorted events by watched item.
func dayEntries(events []event, watcheds map[uint]*entity.Watched) []DayEntry {
	entries := []DayEntry{}
	idx := map[uint]int{}
	for _, e := range events {
		i, ok := idx[e.watchedID]
		if !ok {
			w := watcheds[e.watchedID]
			if w == nil {
				continue
			}
			entry, ok := newDayEntry(w)
			if !ok {
				continue
			}
			i = len(entries)
			idx[e.watchedID] = i
			entries = append(entries, entry)
		}
		if e.kind == kindEpisode {
			entries[i].Episodes = append(entries[i].Episodes, DayEpisode{e.season, e.episode})
		} else {
			entries[i].Plays++
		}
	}
	for i := range entries {
		eps := entries[i].Episodes
		sort.Slice(eps, func(a, b int) bool {
			if eps[a].Season != eps[b].Season {
				return eps[a].Season < eps[b].Season
			}
			return eps[a].Episode < eps[b].Episode
		})
	}
	return entries
}

func newDayEntry(w *entity.Watched) (DayEntry, bool) {
	e := DayEntry{WatchedID: w.ID}
	switch {
	case w.Game != nil:
		e.Type = "game"
		e.ID = w.Game.IgdbID
		e.Title = w.Game.Name
		if w.Game.Poster != nil {
			e.PosterPath = w.Game.Poster.Path
		}
	case w.Content != nil:
		e.Type = string(w.Content.Type)
		e.ID = w.Content.TmdbID
		e.Title = w.Content.Title
		if w.Content.PosterPath != "" {
			// Cached locally when the content was added (see content.saveContent).
			e.PosterPath = "img" + w.Content.PosterPath
		}
	default:
		return e, false
	}
	return e, true
}
