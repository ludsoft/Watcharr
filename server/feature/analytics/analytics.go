// analytics computes a user's personal watch statistics over a period.
//
// Everything is aggregated in Go (not SQL) so day/month boundaries can be
// computed in the user's own timezone. Data volume is per user, so this is
// cheap.

package analytics

import (
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"time"

	// Embed timezone database, our production image (alpine) doesn't ship one.
	_ "time/tzdata"

	"github.com/sbondCo/Watcharr/database/entity"
	"gorm.io/gorm"
)

type Period string

const (
	PeriodCurrentMonth Period = "current_month"
	PeriodCurrentYear  Period = "current_year"
	PeriodLastYear     Period = "last_year"
	PeriodLast12Months Period = "last_12_months"
	PeriodAllTime      Period = "all_time"
)

// Runtime used for episodes of shows that have no runtime set.
const defaultEpisodeRuntime = 30

const dayFmt = "2006-01-02"
const monthFmt = "2006-01"

type Request struct {
	Period Period `form:"period" binding:"required,oneof=current_month current_year last_year last_12_months all_time"`
	// IANA timezone name of the user (eg: Europe/Paris). Defaults to UTC.
	TZ string `form:"tz"`
}

type Counts struct {
	Movies   int `json:"movies"`
	Episodes int `json:"episodes"`
	Games    int `json:"games"`
}

type DayCounts struct {
	Date string `json:"date"`
	Counts
}

type MonthCounts struct {
	Month string `json:"month"`
	Counts
}

type Response struct {
	// First day of the period (inclusive).
	Start string `json:"start"`
	// Last day of the period (inclusive).
	End    string `json:"end"`
	Movies struct {
		Watched int    `json:"watched"`
		Minutes uint64 `json:"minutes"`
	} `json:"movies"`
	Shows struct {
		Finished        int    `json:"finished"`
		Episodes        int    `json:"episodes"`
		MinutesEstimate uint64 `json:"minutesEstimate"`
	} `json:"shows"`
	Games struct {
		Finished int `json:"finished"`
	} `json:"games"`
	// Only days with activity, sorted ascending.
	Days []DayCounts `json:"days"`
	// Every month in the period (including empty ones), sorted ascending.
	Months []MonthCounts `json:"months"`
	// Imported watches that have no watch date, so can't be placed in any
	// period (not limited to the period, not included in anything above).
	Undated struct {
		Movies int `json:"movies"`
		Shows  int `json:"shows"`
		Games  int `json:"games"`
	} `json:"undated"`
}

type Service struct {
	db *gorm.DB
	// Overridable for tests.
	now func() time.Time
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, now: time.Now}
}

// A watch event, already resolved to a point in time.
type event struct {
	at      time.Time
	kind    eventKind
	minutes uint32
	// Imported without any watch date, `at` is meaningless.
	undated bool
	// Watched (list) item the event is for.
	watchedID uint
	// Only for kindEpisode.
	season  int
	episode int
}

type eventKind int

const (
	kindMovie eventKind = iota
	kindShow
	kindEpisode
	kindGame
)

// Row from our activity query.
type activityRow struct {
	ActivityType entity.ActivityType
	CreatedBy    entity.ActivityCreatedBy
	Data         string
	CountAsPlay  bool
	CustomDate   *time.Time
	CreatedAt    time.Time
	WatchedID    uint
	ContentType  *entity.ContentType
	Runtime      *uint32
	GameID       *int
}

type episodeKey struct {
	watchedID uint
	season    int
	episode   int
}

// Get analytics for userId.
func (s *Service) Get(userId uint, req Request) (Response, error) {
	loc := location(req.TZ)
	events, err := s.loadEvents(userId)
	if err != nil {
		return Response{}, err
	}

	start, end := periodRange(req.Period, s.now().In(loc), events, loc)
	return aggregate(events, start, end, loc), nil
}

// Load the users timezone, falling back to UTC when empty or invalid.
func location(tz string) *time.Location {
	if tz == "" {
		return time.UTC
	}
	l, err := time.LoadLocation(tz)
	if err != nil {
		slog.Warn("analytics: Invalid timezone, falling back to UTC", "tz", tz, "error", err)
		return time.UTC
	}
	return l
}

// Load all watch events of the user (all time). Filtering to the period
// happens after converting to the users timezone.
func (s *Service) loadEvents(userId uint) ([]event, error) {
	rows := []activityRow{}
	err := s.db.Raw(`
		SELECT a.type AS activity_type, a.created_by, a.data, a.count_as_play, a.custom_date,
		       a.created_at, a.watched_id, c.type AS content_type, c.runtime,
		       w.game_id
		FROM activities a
		INNER JOIN watcheds w ON w.id = a.watched_id
		       AND w.user_id = a.user_id AND w.deleted_at IS NULL
		LEFT JOIN contents c ON c.id = w.content_id
		WHERE a.user_id = ? AND a.deleted_at IS NULL
		  AND (a.count_as_play = 1 OR a.type IN ?)`,
		userId,
		[]entity.ActivityType{entity.EPISODE_ADDED, entity.EPISODE_STATUS_CHANGED},
	).Scan(&rows).Error
	if err != nil {
		slog.Error("analytics.loadEvents: Activity query failed", "error", err)
		return nil, errors.New("failed to get activity")
	}

	// Episodes are only counted if they are currently finished.
	finishedEps := []struct {
		WatchedID     uint
		SeasonNumber  int
		EpisodeNumber int
	}{}
	err = s.db.Raw(`
		SELECT e.watched_id, e.season_number, e.episode_number
		FROM watched_episodes e
		INNER JOIN watcheds w ON w.id = e.watched_id AND w.deleted_at IS NULL
		WHERE e.user_id = ? AND e.status = ?`,
		userId, entity.FINISHED,
	).Scan(&finishedEps).Error
	if err != nil {
		slog.Error("analytics.loadEvents: Episode query failed", "error", err)
		return nil, errors.New("failed to get episodes")
	}
	isFinishedEp := make(map[episodeKey]bool, len(finishedEps))
	for _, e := range finishedEps {
		isFinishedEp[episodeKey{e.WatchedID, e.SeasonNumber, e.EpisodeNumber}] = true
	}

	// Imports record the play on an ADDED_WATCHED activity dated at import
	// time, and the real watch date on a separate (non play) dated
	// IMPORTED_ADDED_WATCHED activity, so we need those dates.
	// (Not using MIN() in sql, sqlite then returns a string, not a time.)
	importDates := []struct {
		WatchedID  uint
		CustomDate time.Time
	}{}
	err = s.db.Raw(`
		SELECT watched_id, custom_date
		FROM activities
		WHERE user_id = ? AND deleted_at IS NULL AND type = ?
		  AND count_as_play = 0 AND custom_date IS NOT NULL`,
		userId, entity.IMPORTED_ADDED_WATCHED,
	).Scan(&importDates).Error
	if err != nil {
		slog.Error("analytics.loadEvents: Import dates query failed", "error", err)
		return nil, errors.New("failed to get import dates")
	}
	importDateOf := make(map[uint]time.Time, len(importDates))
	for _, d := range importDates {
		if prev, ok := importDateOf[d.WatchedID]; !ok || d.CustomDate.Before(prev) {
			importDateOf[d.WatchedID] = d.CustomDate
		}
	}

	events := []event{}
	// First time each episode was marked finished.
	episodeFirst := map[episodeKey]event{}
	for _, r := range rows {
		at := r.CreatedAt
		if r.CustomDate != nil && !r.CustomDate.IsZero() {
			at = *r.CustomDate
		}
		var runtime uint32
		if r.Runtime != nil {
			runtime = *r.Runtime
		}

		if r.ActivityType == entity.EPISODE_ADDED || r.ActivityType == entity.EPISODE_STATUS_CHANGED {
			var d struct {
				Season  int                  `json:"season"`
				Episode int                  `json:"episode"`
				Status  entity.WatchedStatus `json:"status"`
			}
			if err := json.Unmarshal([]byte(r.Data), &d); err != nil || d.Status != entity.FINISHED {
				continue
			}
			k := episodeKey{r.WatchedID, d.Season, d.Episode}
			if !isFinishedEp[k] {
				continue
			}
			if runtime == 0 {
				runtime = defaultEpisodeRuntime
			}
			if prev, ok := episodeFirst[k]; !ok || at.Before(prev.at) {
				episodeFirst[k] = event{
					at: at, kind: kindEpisode, minutes: runtime,
					watchedID: r.WatchedID, season: d.Season, episode: d.Episode,
				}
			}
			continue
		}

		if !r.CountAsPlay {
			continue
		}
		undated := false
		if r.ActivityType == entity.ADDED_WATCHED && isImport(r.CreatedBy) &&
			(r.CustomDate == nil || r.CustomDate.IsZero()) {
			if d, ok := importDateOf[r.WatchedID]; ok {
				at = d
			} else {
				undated = true
			}
		}
		e := event{at: at, undated: undated, watchedID: r.WatchedID}
		switch {
		case r.GameID != nil:
			e.kind = kindGame
		case r.ContentType != nil && *r.ContentType == entity.MOVIE:
			e.kind = kindMovie
			e.minutes = runtime
		case r.ContentType != nil && *r.ContentType == entity.SHOW:
			e.kind = kindShow
		default:
			continue
		}
		events = append(events, e)
	}
	for _, e := range episodeFirst {
		events = append(events, e)
	}
	return events, nil
}

func isImport(c entity.ActivityCreatedBy) bool {
	return c == entity.ActivityCreatedByGenericImport ||
		c == entity.ActivityCreatedByJellyfinImport ||
		c == entity.ActivityCreatedByPlexImport
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// Get the [start, end) range of the period, in loc.
// Periods are aligned to whole months, so every month bar is complete.
func periodRange(p Period, now time.Time, events []event, loc *time.Location) (time.Time, time.Time) {
	thisMonth := monthStart(now)
	thisYear := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, loc)
	switch p {
	case PeriodCurrentMonth:
		return thisMonth, thisMonth.AddDate(0, 1, 0)
	case PeriodCurrentYear:
		return thisYear, thisYear.AddDate(1, 0, 0)
	case PeriodLastYear:
		return thisYear.AddDate(-1, 0, 0), thisYear
	case PeriodLast12Months:
		return thisMonth.AddDate(0, -11, 0), thisMonth.AddDate(0, 1, 0)
	}
	// All time: from the month of the first event to the end of this month.
	start := thisMonth
	for _, e := range events {
		if e.undated {
			continue
		}
		if m := monthStart(e.at.In(loc)); m.Before(start) {
			start = m
		}
	}
	return start, thisMonth.AddDate(0, 1, 0)
}

func aggregate(events []event, start, end time.Time, loc *time.Location) Response {
	r := Response{
		Start:  start.Format(dayFmt),
		End:    end.AddDate(0, 0, -1).Format(dayFmt),
		Days:   []DayCounts{},
		Months: []MonthCounts{},
	}
	monthIdx := map[string]int{}
	for m := start; m.Before(end); m = m.AddDate(0, 1, 0) {
		monthIdx[m.Format(monthFmt)] = len(r.Months)
		r.Months = append(r.Months, MonthCounts{Month: m.Format(monthFmt)})
	}
	days := map[string]*Counts{}

	for _, e := range events {
		if e.undated {
			switch e.kind {
			case kindMovie:
				r.Undated.Movies++
			case kindShow:
				r.Undated.Shows++
			case kindGame:
				r.Undated.Games++
			}
			continue
		}
		at := e.at.In(loc)
		if at.Before(start) || !at.Before(end) {
			continue
		}
		if e.kind == kindShow {
			// Finishing a show isn't a viewing event on its own (the
			// episodes are), so it only counts towards the total.
			r.Shows.Finished++
			continue
		}
		dk := at.Format(dayFmt)
		dc := days[dk]
		if dc == nil {
			dc = &Counts{}
			days[dk] = dc
		}
		mc := &r.Months[monthIdx[at.Format(monthFmt)]].Counts
		switch e.kind {
		case kindMovie:
			r.Movies.Watched++
			r.Movies.Minutes += uint64(e.minutes)
			dc.Movies++
			mc.Movies++
		case kindEpisode:
			r.Shows.Episodes++
			r.Shows.MinutesEstimate += uint64(e.minutes)
			dc.Episodes++
			mc.Episodes++
		case kindGame:
			r.Games.Finished++
			dc.Games++
			mc.Games++
		}
	}

	for d, c := range days {
		r.Days = append(r.Days, DayCounts{Date: d, Counts: *c})
	}
	sort.Slice(r.Days, func(i, j int) bool { return r.Days[i].Date < r.Days[j].Date })
	return r
}
