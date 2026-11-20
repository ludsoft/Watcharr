package analytics

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sbondCo/Watcharr/database/entity"
	"github.com/sbondCo/Watcharr/internal/testutil"
	"gorm.io/gorm"
)

func mustCreate(t *testing.T, db *gorm.DB, v any) {
	t.Helper()
	if err := db.Create(v).Error; err != nil {
		t.Fatalf("create %T: %v", v, err)
	}
}

func utc(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func epData(season, episode int, status entity.WatchedStatus) string {
	b, _ := json.Marshal(map[string]any{"season": season, "episode": episode, "status": status})
	return string(b)
}

type fixture struct {
	db  *gorm.DB
	svc *Service
	uid uint
}

// Builds a user with a varied watch history. "Now" is 2026-11-20 12:00 UTC.
func setup(t *testing.T) fixture {
	t.Helper()
	testutil.SetupLogging()
	db := testutil.SetupDB(t)

	user := entity.User{Username: "me", Password: "x"}
	other := entity.User{Username: "other", Password: "x"}
	mustCreate(t, db, &user)
	mustCreate(t, db, &other)

	movieA := entity.Content{TmdbID: 1, Title: "A", Type: entity.MOVIE, Runtime: 120}
	movieB := entity.Content{TmdbID: 2, Title: "B", Type: entity.MOVIE, Runtime: 90}
	movieGone := entity.Content{TmdbID: 3, Title: "Gone", Type: entity.MOVIE, Runtime: 100}
	show := entity.Content{TmdbID: 4, Title: "S", Type: entity.SHOW, Runtime: 0}
	mustCreate(t, db, &movieA)
	mustCreate(t, db, &movieB)
	mustCreate(t, db, &movieGone)
	mustCreate(t, db, &show)
	game := entity.Game{IgdbID: 10, Name: "G"}
	mustCreate(t, db, &game)

	wA := entity.Watched{UserID: user.ID, ContentID: &movieA.ID, Status: entity.FINISHED}
	wB := entity.Watched{UserID: user.ID, ContentID: &movieB.ID, Status: entity.FINISHED}
	wGone := entity.Watched{UserID: user.ID, ContentID: &movieGone.ID, Status: entity.FINISHED}
	wS := entity.Watched{UserID: user.ID, ContentID: &show.ID, Status: entity.FINISHED}
	wG := entity.Watched{UserID: user.ID, GameID: &game.ID, Status: entity.FINISHED}
	wOther := entity.Watched{UserID: other.ID, ContentID: &movieA.ID, Status: entity.FINISHED}
	for _, w := range []*entity.Watched{&wA, &wB, &wGone, &wS, &wG, &wOther} {
		mustCreate(t, db, w)
	}

	act := func(uid, wid uint, typ entity.ActivityType, play bool, data string, at *time.Time) {
		mustCreate(t, db, &entity.Activity{
			UserID: uid, WatchedID: wid, Type: typ, CountAsPlay: play, Data: data, CustomDate: at,
		})
	}
	// Movie A watched, then rewatched, plus a rating change (not a play).
	act(user.ID, wA.ID, entity.ADDED_WATCHED, true, `{"status":"FINISHED"}`, utc("2026-11-03T10:00:00Z"))
	act(user.ID, wA.ID, entity.STATUS_CHANGED, true, "FINISHED", utc("2026-11-10T20:00:00Z"))
	act(user.ID, wA.ID, entity.RATING_CHANGED, false, "8", utc("2026-11-11T20:00:00Z"))
	// Movie B: 23:30 UTC on Oct 31 is 00:30 on Nov 1 in Paris.
	act(user.ID, wB.ID, entity.ADDED_WATCHED, true, `{"status":"FINISHED"}`, utc("2026-10-31T23:30:00Z"))
	// Removed watched item, must never count.
	act(user.ID, wGone.ID, entity.ADDED_WATCHED, true, `{"status":"FINISHED"}`, utc("2026-11-04T10:00:00Z"))
	if err := db.Delete(&wGone).Error; err != nil {
		t.Fatal(err)
	}
	// Another users play, must never count.
	act(other.ID, wOther.ID, entity.ADDED_WATCHED, true, `{"status":"FINISHED"}`, utc("2026-11-05T10:00:00Z"))
	// Show finished.
	act(user.ID, wS.ID, entity.STATUS_CHANGED, true, "FINISHED", utc("2026-11-05T10:00:00Z"))
	// Episode 1x1: finished, unfinished, finished again -> counted once, on first date.
	act(user.ID, wS.ID, entity.EPISODE_ADDED, false, epData(1, 1, entity.FINISHED), utc("2026-11-04T10:00:00Z"))
	act(user.ID, wS.ID, entity.EPISODE_STATUS_CHANGED, false, epData(1, 1, entity.WATCHING), utc("2026-11-05T10:00:00Z"))
	act(user.ID, wS.ID, entity.EPISODE_STATUS_CHANGED, false, epData(1, 1, entity.FINISHED), utc("2026-11-06T10:00:00Z"))
	// Episode 1x2: was finished, but currently isn't -> not counted.
	act(user.ID, wS.ID, entity.EPISODE_ADDED, false, epData(1, 2, entity.FINISHED), utc("2026-11-04T10:00:00Z"))
	// Episode 1x3: finished last year.
	act(user.ID, wS.ID, entity.EPISODE_ADDED, false, epData(1, 3, entity.FINISHED), utc("2025-12-01T10:00:00Z"))
	// Game finished, no custom date (falls back to created_at).
	mustCreate(t, db, &entity.Activity{
		UserID: user.ID, WatchedID: wG.ID, Type: entity.STATUS_CHANGED, CountAsPlay: true, Data: "FINISHED",
	})
	if err := db.Model(&entity.Activity{}).
		Where("watched_id = ?", wG.ID).
		Update("created_at", *utc("2026-02-01T10:00:00Z")).Error; err != nil {
		t.Fatal(err)
	}

	// Imported movie: play recorded at import time (Nov 15), the real watch
	// date (2025-12-24) is on a non play IMPORTED_ADDED_WATCHED.
	movieImp := entity.Content{TmdbID: 5, Title: "Imp", Type: entity.MOVIE, Runtime: 60}
	movieUndated := entity.Content{TmdbID: 6, Title: "Undated", Type: entity.MOVIE, Runtime: 70}
	mustCreate(t, db, &movieImp)
	mustCreate(t, db, &movieUndated)
	wImp := entity.Watched{UserID: user.ID, ContentID: &movieImp.ID, Status: entity.FINISHED}
	wUndated := entity.Watched{UserID: user.ID, ContentID: &movieUndated.ID, Status: entity.FINISHED}
	mustCreate(t, db, &wImp)
	mustCreate(t, db, &wUndated)
	for _, a := range []entity.Activity{
		{UserID: user.ID, WatchedID: wImp.ID, Type: entity.ADDED_WATCHED, CountAsPlay: true,
			CreatedBy: entity.ActivityCreatedByGenericImport},
		{UserID: user.ID, WatchedID: wImp.ID, Type: entity.IMPORTED_ADDED_WATCHED, CountAsPlay: false,
			CreatedBy: entity.ActivityCreatedByGenericImport, CustomDate: utc("2025-12-24T20:00:00Z")},
		// Imported without any watch date.
		{UserID: user.ID, WatchedID: wUndated.ID, Type: entity.ADDED_WATCHED, CountAsPlay: true,
			CreatedBy: entity.ActivityCreatedByPlexImport},
	} {
		mustCreate(t, db, &a)
	}
	if err := db.Model(&entity.Activity{}).
		Where("watched_id IN ? AND type = ?", []uint{wImp.ID, wUndated.ID}, entity.ADDED_WATCHED).
		Update("created_at", *utc("2026-11-15T10:00:00Z")).Error; err != nil {
		t.Fatal(err)
	}

	for _, e := range []entity.WatchedEpisode{
		{UserID: user.ID, WatchedID: wS.ID, SeasonNumber: 1, EpisodeNumber: 1, Status: entity.FINISHED},
		{UserID: user.ID, WatchedID: wS.ID, SeasonNumber: 1, EpisodeNumber: 2, Status: entity.WATCHING},
		{UserID: user.ID, WatchedID: wS.ID, SeasonNumber: 1, EpisodeNumber: 3, Status: entity.FINISHED},
	} {
		mustCreate(t, db, &e)
	}

	svc := NewService(db)
	svc.now = func() time.Time { return *utc("2026-11-20T12:00:00Z") }
	return fixture{db, svc, user.ID}
}

func get(t *testing.T, f fixture, p Period, tz string) Response {
	t.Helper()
	r, err := f.svc.Get(f.uid, Request{Period: p, TZ: tz})
	if err != nil {
		t.Fatalf("Get(%s, %s): %v", p, tz, err)
	}
	return r
}

func TestCurrentMonthParis(t *testing.T) {
	f := setup(t)
	r := get(t, f, PeriodCurrentMonth, "Europe/Paris")

	if r.Start != "2026-11-01" || r.End != "2026-11-30" {
		t.Errorf("range = %s..%s", r.Start, r.End)
	}
	// A twice (rewatch) + B (Nov 1 in Paris).
	if r.Movies.Watched != 3 || r.Movies.Minutes != 120*2+90 {
		t.Errorf("movies = %+v, want 3 watched / 330 min", r.Movies)
	}
	// Only 1x1 (1x2 not finished anymore, 1x3 is last year). Default runtime 30.
	if r.Shows.Episodes != 1 || r.Shows.MinutesEstimate != 30 || r.Shows.Finished != 1 {
		t.Errorf("shows = %+v", r.Shows)
	}
	if r.Games.Finished != 0 {
		t.Errorf("games = %+v", r.Games)
	}
	if len(r.Months) != 1 || r.Months[0].Month != "2026-11" || r.Months[0].Movies != 3 || r.Months[0].Episodes != 1 {
		t.Errorf("months = %+v", r.Months)
	}
	want := map[string]Counts{
		"2026-11-01": {Movies: 1},
		"2026-11-03": {Movies: 1},
		"2026-11-04": {Episodes: 1}, // first finish date of 1x1
		"2026-11-10": {Movies: 1},
	}
	if len(r.Days) != len(want) {
		t.Fatalf("days = %+v", r.Days)
	}
	for _, d := range r.Days {
		if want[d.Date] != d.Counts {
			t.Errorf("day %s = %+v, want %+v", d.Date, d.Counts, want[d.Date])
		}
	}
}

func TestCurrentMonthUTCExcludesBoundaryMovie(t *testing.T) {
	f := setup(t)
	r := get(t, f, PeriodCurrentMonth, "")
	if r.Movies.Watched != 2 {
		t.Errorf("movies watched in UTC = %d, want 2 (B is Oct 31 in UTC)", r.Movies.Watched)
	}
}

func TestInvalidTZFallsBackToUTC(t *testing.T) {
	f := setup(t)
	r := get(t, f, PeriodCurrentMonth, "Not/AZone")
	if r.Movies.Watched != 2 {
		t.Errorf("movies watched = %d, want 2 (UTC)", r.Movies.Watched)
	}
}

func TestYearPeriods(t *testing.T) {
	f := setup(t)

	cy := get(t, f, PeriodCurrentYear, "Europe/Paris")
	if cy.Start != "2026-01-01" || cy.End != "2026-12-31" || len(cy.Months) != 12 {
		t.Errorf("current year range = %s..%s, %d months", cy.Start, cy.End, len(cy.Months))
	}
	if cy.Games.Finished != 1 || cy.Months[1].Games != 1 {
		t.Errorf("current year games = %+v, feb = %+v", cy.Games, cy.Months[1])
	}
	if cy.Shows.Episodes != 1 {
		t.Errorf("current year episodes = %d, want 1", cy.Shows.Episodes)
	}

	ly := get(t, f, PeriodLastYear, "Europe/Paris")
	if ly.Start != "2025-01-01" || ly.End != "2025-12-31" {
		t.Errorf("last year range = %s..%s", ly.Start, ly.End)
	}
	if ly.Shows.Episodes != 1 || ly.Movies.Watched != 1 || ly.Months[11].Episodes != 1 {
		t.Errorf("last year = %+v / %+v", ly.Shows, ly.Movies)
	}

	l12 := get(t, f, PeriodLast12Months, "Europe/Paris")
	if l12.Start != "2025-12-01" || l12.End != "2026-11-30" || len(l12.Months) != 12 {
		t.Errorf("last 12 months range = %s..%s, %d months", l12.Start, l12.End, len(l12.Months))
	}
	if l12.Shows.Episodes != 2 || l12.Movies.Watched != 4 || l12.Games.Finished != 1 {
		t.Errorf("last 12 months = %+v %+v %+v", l12.Movies, l12.Shows, l12.Games)
	}
}

func TestAllTime(t *testing.T) {
	f := setup(t)
	r := get(t, f, PeriodAllTime, "Europe/Paris")
	if r.Start != "2025-12-01" || r.End != "2026-11-30" {
		t.Errorf("all time range = %s..%s", r.Start, r.End)
	}
	if r.Movies.Watched != 4 || r.Shows.Episodes != 2 || r.Shows.Finished != 1 || r.Games.Finished != 1 {
		t.Errorf("all time = %+v %+v %+v", r.Movies, r.Shows, r.Games)
	}
}

func TestAllTimeNoData(t *testing.T) {
	testutil.SetupLogging()
	db := testutil.SetupDB(t)
	u := entity.User{Username: "empty", Password: "x"}
	mustCreate(t, db, &u)
	svc := NewService(db)
	svc.now = func() time.Time { return *utc("2026-11-20T12:00:00Z") }
	r, err := svc.Get(u.ID, Request{Period: PeriodAllTime})
	if err != nil {
		t.Fatal(err)
	}
	if r.Start != "2026-11-01" || len(r.Months) != 1 || len(r.Days) != 0 || r.Movies.Watched != 0 {
		t.Errorf("empty all time = %+v", r)
	}
}

func TestImportedPlaysUseImportedWatchDate(t *testing.T) {
	f := setup(t)

	// Not on the import day (Nov 15).
	cm := get(t, f, PeriodCurrentMonth, "Europe/Paris")
	for _, d := range cm.Days {
		if d.Date == "2026-11-15" {
			t.Errorf("imported plays counted on import day: %+v", d)
		}
	}
	if cm.Movies.Watched != 3 {
		t.Errorf("current month movies = %d, want 3", cm.Movies.Watched)
	}
	// Undated imports are reported, not counted.
	if cm.Undated.Movies != 1 {
		t.Errorf("undated = %+v, want 1 movie", cm.Undated)
	}

	// On the real watch date instead.
	ly := get(t, f, PeriodLastYear, "Europe/Paris")
	if ly.Movies.Watched != 1 || ly.Movies.Minutes != 60 || ly.Months[11].Movies != 1 {
		t.Errorf("last year movies = %+v, dec = %+v", ly.Movies, ly.Months[11])
	}
}
