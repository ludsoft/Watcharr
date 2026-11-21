package analytics

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sbondCo/Watcharr/database/entity"
)

func getDay(t *testing.T, f fixture, date, tz string) DayResponse {
	t.Helper()
	r, err := f.svc.GetDay(f.uid, DayRequest{Date: date, TZ: tz})
	if err != nil {
		t.Fatalf("GetDay(%s, %s): %v", date, tz, err)
	}
	return r
}

func TestDayUsesUserTimezone(t *testing.T) {
	f := setup(t)
	// Movie B: Oct 31 23:30 UTC is Nov 1 in Paris.
	paris := getDay(t, f, "2026-11-01", "Europe/Paris")
	if len(paris.Entries) != 1 {
		t.Fatalf("paris entries = %+v", paris.Entries)
	}
	e := paris.Entries[0]
	if e.Type != "movie" || e.ID != 2 || e.Title != "B" || e.Plays != 1 || e.Episodes != nil {
		t.Errorf("entry = %+v", e)
	}
	if utcDay := getDay(t, f, "2026-11-01", ""); len(utcDay.Entries) != 0 {
		t.Errorf("utc entries = %+v, want none", utcDay.Entries)
	}
	if utcDay := getDay(t, f, "2026-10-31", ""); len(utcDay.Entries) != 1 {
		t.Errorf("utc Oct 31 entries = %+v, want B", utcDay.Entries)
	}
}

func TestDayEpisodes(t *testing.T) {
	f := setup(t)
	// 1x1 first finished on Nov 4 (1x2 too, but it isn't finished anymore).
	// The removed movie watched that day must not show.
	r := getDay(t, f, "2026-11-04", "Europe/Paris")
	if len(r.Entries) != 1 {
		t.Fatalf("entries = %+v", r.Entries)
	}
	e := r.Entries[0]
	if e.Type != "tv" || e.ID != 4 || e.Plays != 0 ||
		!reflect.DeepEqual(e.Episodes, []DayEpisode{{1, 1}}) {
		t.Errorf("entry = %+v", e)
	}
	// Refinished 1x1 on Nov 6 and finishing the show (Nov 5) aren't shown,
	// same as in the activity history.
	for _, d := range []string{"2026-11-05", "2026-11-06"} {
		if r := getDay(t, f, d, "Europe/Paris"); len(r.Entries) != 0 {
			t.Errorf("%s entries = %+v, want none", d, r.Entries)
		}
	}
}

func TestDayImportedUsesWatchDate(t *testing.T) {
	f := setup(t)
	if r := getDay(t, f, "2026-11-15", "Europe/Paris"); len(r.Entries) != 0 {
		t.Errorf("import day entries = %+v, want none", r.Entries)
	}
	r := getDay(t, f, "2025-12-24", "Europe/Paris")
	if len(r.Entries) != 1 || r.Entries[0].Title != "Imp" {
		t.Errorf("watch date entries = %+v", r.Entries)
	}
}

func TestDayGroupsAndOrders(t *testing.T) {
	f := setup(t)
	poster := entity.Content{TmdbID: 7, Title: "P", Type: entity.MOVIE, PosterPath: "/p.jpg"}
	mustCreate(t, f.db, &poster)
	wP := entity.Watched{UserID: f.uid, ContentID: &poster.ID, Status: entity.FINISHED}
	mustCreate(t, f.db, &wP)
	var game entity.Game
	if err := f.db.Where("igdb_id = ?", 10).First(&game).Error; err != nil {
		t.Fatal(err)
	}
	var wG entity.Watched
	if err := f.db.Where("game_id = ?", game.ID).First(&wG).Error; err != nil {
		t.Fatal(err)
	}
	for _, a := range []entity.Activity{
		{UserID: f.uid, WatchedID: wP.ID, Type: entity.ADDED_WATCHED, CountAsPlay: true,
			CustomDate: utc("2026-11-12T21:00:00Z")},
		{UserID: f.uid, WatchedID: wG.ID, Type: entity.STATUS_CHANGED, CountAsPlay: true,
			Data: "FINISHED", CustomDate: utc("2026-11-12T09:00:00Z")},
		{UserID: f.uid, WatchedID: wP.ID, Type: entity.STATUS_CHANGED, CountAsPlay: true,
			Data: "FINISHED", CustomDate: utc("2026-11-12T10:00:00Z")},
	} {
		mustCreate(t, f.db, &a)
	}

	r := getDay(t, f, "2026-11-12", "")
	want := []DayEntry{
		{WatchedID: wG.ID, Type: "game", ID: 10, Title: "G", Plays: 1},
		{WatchedID: wP.ID, Type: "movie", ID: 7, Title: "P", PosterPath: "img/p.jpg", Plays: 2},
	}
	if !reflect.DeepEqual(r.Entries, want) {
		t.Errorf("entries = %+v\nwant %+v", r.Entries, want)
	}
}

func TestDayInvalidDate(t *testing.T) {
	f := setup(t)
	for _, d := range []string{"", "2026-13-01", "20261101", "yesterday"} {
		_, err := f.svc.GetDay(f.uid, DayRequest{Date: d})
		if !errors.Is(err, ErrInvalidDate) {
			t.Errorf("GetDay(%q) err = %v, want ErrInvalidDate", d, err)
		}
	}
}
