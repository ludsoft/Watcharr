package releasedate

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/sbondCo/Watcharr/database/entity"
	"github.com/sbondCo/Watcharr/domain"
	"github.com/sbondCo/Watcharr/feature/watched"
	"github.com/sbondCo/Watcharr/feature/watched/episode"
	"github.com/sbondCo/Watcharr/feature/watched/season"
	"github.com/sbondCo/Watcharr/internal/testutil"
	"github.com/sbondCo/Watcharr/media/tmdb"
	"gorm.io/gorm"
)

func day(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &t
}

// Content cache backed by the test db, with configurable release dates.
type fakeContent struct {
	db    *gorm.DB
	dates map[int]*time.Time
}

func (f *fakeContent) GetOrCacheContent(ct entity.ContentType, tmdbId int) (entity.Content, error) {
	c := entity.Content{}
	err := f.db.Where(entity.Content{TmdbID: tmdbId, Type: ct}).
		Attrs(entity.Content{Title: "c", ReleaseDate: f.dates[tmdbId]}).
		FirstOrCreate(&c).Error
	return c, err
}

type fakeGames struct {
	db    *gorm.DB
	dates map[int]*time.Time
}

func (f *fakeGames) GetOrCache(igdbId int) (entity.Game, error) {
	g := entity.Game{}
	err := f.db.Where(entity.Game{IgdbID: igdbId}).
		Attrs(entity.Game{Name: "g", ReleaseDate: f.dates[igdbId]}).
		FirstOrCreate(&g).Error
	return g, err
}

// Automations off, so hooks (which need the real tmdb) don't run.
type fakeUsers struct{}

func (fakeUsers) UserGetSettings(uint) (entity.UserSettings, error) {
	off := false
	return entity.UserSettings{AutomateShowStatuses: &off}, nil
}

// Show with: specials (season 0), season 1 (2 aired episodes) and season 2
// (1 aired, 1 future episode).
type fakeTMDB struct{}

func (fakeTMDB) ShowDetails(tmdb.ShowDetailsOptions) (tmdb.ShowDetails, error) {
	d := tmdb.ShowDetails{}
	err := json.Unmarshal([]byte(`{"seasons":[
		{"season_number":0},{"season_number":1},{"season_number":2}]}`), &d)
	return d, err
}

func (fakeTMDB) SeasonDetails(_ string, seasonNumber string) (tmdb.SeasonDetails, error) {
	eps := map[string]string{
		"0": `[{"episode_number":1,"air_date":"2001-01-01"}]`,
		"1": `[{"episode_number":2,"air_date":"2010-01-08"},{"episode_number":1,"air_date":"2010-01-01"}]`,
		"2": `[{"episode_number":1,"air_date":"2011-03-01"},{"episode_number":2,"air_date":"2027-01-01"}]`,
	}[seasonNumber]
	d := tmdb.SeasonDetails{}
	err := json.Unmarshal([]byte(`{"episodes":`+eps+`}`), &d)
	return d, err
}

type fixture struct {
	db  *gorm.DB
	svc *Service
	ws  *watched.Service
	uid uint
}

const (
	movieOld    = 1
	movieFuture = 2
	movieNoDate = 3
	showId      = 10
	gameId      = 20
)

func setup(t *testing.T) fixture {
	t.Helper()
	testutil.SetupLogging()
	db := testutil.SetupDB(t)
	u := entity.User{Username: "me", Password: "x"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	cp := &fakeContent{db, map[int]*time.Time{
		movieOld:    day("1994-05-11"),
		movieFuture: day("2027-06-01"),
		showId:      day("2010-01-01"),
	}}
	gp := &fakeGames{db, map[int]*time.Time{gameId: day("2015-05-19")}}
	ws := watched.NewService(db, cp, gp, fakeUsers{})
	ss := season.NewService(db, ws, fakeUsers{}, nil)
	es := episode.NewService(db, ws, ss, nil, fakeUsers{})
	svc := NewService(ws, ss, es, cp, gp, fakeTMDB{})
	svc.now = func() time.Time { return time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC) }
	return fixture{db, svc, ws, u.ID}
}

// Activities of a watched entry, by type.
func activities(t *testing.T, f fixture, wid uint, typ entity.ActivityType) []entity.Activity {
	t.Helper()
	a := []entity.Activity{}
	if err := f.db.Where("watched_id = ? AND type = ?", wid, typ).Find(&a).Error; err != nil {
		t.Fatal(err)
	}
	return a
}

func plays(t *testing.T, f fixture, wid uint) []entity.Activity {
	t.Helper()
	a := []entity.Activity{}
	if err := f.db.Where("watched_id = ? AND count_as_play = 1", wid).Find(&a).Error; err != nil {
		t.Fatal(err)
	}
	return a
}

func dateOf(a entity.Activity) string {
	if a.CustomDate == nil {
		return "(none)"
	}
	return a.CustomDate.UTC().Format(time.RFC3339)
}

func TestMovieNew(t *testing.T) {
	f := setup(t)
	w, err := f.svc.FinishContent(f.uid, ContentRequest{ContentType: "movie", TMDBID: movieOld})
	if err != nil {
		t.Fatal(err)
	}
	if w.Status != entity.FINISHED || w.Plays != 1 {
		t.Errorf("status %s, plays %d", w.Status, w.Plays)
	}
	p := plays(t, f, w.ID)
	if len(p) != 1 || dateOf(p[0]) != "1994-05-11T12:00:00Z" {
		t.Errorf("plays = %d, date %s", len(p), dateOf(p[0]))
	}
}

func TestMovieExistingAndAlreadyFinished(t *testing.T) {
	f := setup(t)
	added, err := f.ws.AddWatched(f.uid, domain.WatchedAddRequest{
		ContentType: "movie", TMDBID: movieOld, Status: entity.PLANNED,
	}, domain.WatchedAddExtraProps{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.FinishContent(f.uid, ContentRequest{ContentType: "movie", TMDBID: movieOld}); err != nil {
		t.Fatal(err)
	}
	p := plays(t, f, added.ID)
	if len(p) != 1 || p[0].Type != entity.STATUS_CHANGED || dateOf(p[0]) != "1994-05-11T12:00:00Z" {
		t.Fatalf("plays = %+v", p)
	}
	// Again: already finished, no extra play.
	w, err := f.svc.FinishContent(f.uid, ContentRequest{ContentType: "movie", TMDBID: movieOld})
	if err != nil {
		t.Fatal(err)
	}
	if w.Plays != 1 {
		t.Errorf("plays after second call = %d, want 1", w.Plays)
	}
}

func TestMovieRejections(t *testing.T) {
	f := setup(t)
	for id, want := range map[int]error{movieFuture: errNotReleased, movieNoDate: errNoReleaseDate} {
		_, err := f.svc.FinishContent(f.uid, ContentRequest{ContentType: "movie", TMDBID: id})
		if !errors.Is(err, want) {
			t.Errorf("movie %d: err = %v, want %v", id, err, want)
		}
	}
	var n int64
	f.db.Model(&entity.Watched{}).Count(&n)
	if n != 0 {
		t.Errorf("%d watched entries created for rejected requests", n)
	}
}

func TestGame(t *testing.T) {
	f := setup(t)
	w, err := f.svc.FinishContent(f.uid, ContentRequest{ContentType: "game", IGDBID: gameId})
	if err != nil {
		t.Fatal(err)
	}
	p := plays(t, f, w.ID)
	if len(p) != 1 || dateOf(p[0]) != "2015-05-19T12:00:00Z" {
		t.Errorf("game plays = %+v", p)
	}
}

func TestShow(t *testing.T) {
	f := setup(t)
	w, err := f.svc.FinishContent(f.uid, ContentRequest{ContentType: "tv", TMDBID: showId})
	if err != nil {
		t.Fatal(err)
	}
	if w.Status != entity.FINISHED {
		t.Errorf("show status = %s", w.Status)
	}
	// One play, at the last released episode (S2E1), not the future S2E2.
	p := plays(t, f, w.ID)
	if len(p) != 1 || dateOf(p[0]) != "2011-03-01T12:00:00Z" {
		t.Errorf("show plays = %+v", p)
	}
	// Released episodes finished (no specials, no future episode).
	got := map[[2]int]entity.WatchedStatus{}
	for _, e := range w.WatchedEpisodes {
		got[[2]int{e.SeasonNumber, e.EpisodeNumber}] = e.Status
	}
	want := map[[2]int]entity.WatchedStatus{{1, 1}: entity.FINISHED, {1, 2}: entity.FINISHED, {2, 1}: entity.FINISHED}
	if len(got) != len(want) {
		t.Errorf("episodes = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("episode %v = %q, want %q", k, got[k], v)
		}
	}
	// Each episode dated at its air date.
	epDates := map[string]bool{}
	for _, a := range activities(t, f, w.ID, entity.EPISODE_ADDED) {
		epDates[dateOf(a)] = true
	}
	for _, d := range []string{"2010-01-01T12:00:00Z", "2010-01-08T12:00:00Z", "2011-03-01T12:00:00Z"} {
		if !epDates[d] {
			t.Errorf("no episode activity dated %s (have %v)", d, epDates)
		}
	}
	// Seasons finished at their last episode.
	seasonDates := map[string]bool{}
	for _, a := range activities(t, f, w.ID, entity.SEASON_ADDED) {
		seasonDates[dateOf(a)] = true
	}
	if !seasonDates["2010-01-08T12:00:00Z"] || !seasonDates["2011-03-01T12:00:00Z"] || len(seasonDates) != 2 {
		t.Errorf("season dates = %v", seasonDates)
	}
}

func TestShowExistingEpisodeInProgress(t *testing.T) {
	f := setup(t)
	added, err := f.ws.AddWatched(f.uid, domain.WatchedAddRequest{
		ContentType: "tv", TMDBID: showId, Status: entity.WATCHING,
	}, domain.WatchedAddExtraProps{})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&entity.WatchedEpisode{
		UserID: f.uid, WatchedID: added.ID, SeasonNumber: 1, EpisodeNumber: 1, Status: entity.WATCHING,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.FinishContent(f.uid, ContentRequest{ContentType: "tv", TMDBID: showId}); err != nil {
		t.Fatal(err)
	}
	changed := activities(t, f, added.ID, entity.EPISODE_STATUS_CHANGED)
	if len(changed) != 1 || dateOf(changed[0]) != "2010-01-01T12:00:00Z" {
		t.Errorf("S1E1 status change activities = %+v", changed)
	}
}

func TestSeasonAndEpisode(t *testing.T) {
	f := setup(t)
	added, err := f.ws.AddWatched(f.uid, domain.WatchedAddRequest{
		ContentType: "tv", TMDBID: showId, Status: entity.WATCHING,
	}, domain.WatchedAddExtraProps{})
	if err != nil {
		t.Fatal(err)
	}

	w, err := f.svc.FinishEpisode(f.uid, EpisodeRequest{WatchedID: added.ID, SeasonNumber: 2, EpisodeNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.WatchedEpisodes) != 1 {
		t.Fatalf("episodes = %+v", w.WatchedEpisodes)
	}
	if _, err := f.svc.FinishEpisode(f.uid, EpisodeRequest{WatchedID: added.ID, SeasonNumber: 2, EpisodeNumber: 2}); !errors.Is(err, errNotReleased) {
		t.Errorf("future episode err = %v", err)
	}

	w, err = f.svc.FinishSeason(f.uid, SeasonRequest{WatchedID: added.ID, SeasonNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.WatchedEpisodes) != 3 || len(w.WatchedSeasons) != 1 || w.WatchedSeasons[0].Status != entity.FINISHED {
		t.Errorf("after season: eps %d, seasons %+v", len(w.WatchedEpisodes), w.WatchedSeasons)
	}
	// Show itself isn't finished by a single season (no play added).
	if len(plays(t, f, added.ID)) != 0 {
		t.Errorf("season finish added a show play")
	}
}

func TestOtherUsersEntry(t *testing.T) {
	f := setup(t)
	added, err := f.ws.AddWatched(f.uid, domain.WatchedAddRequest{
		ContentType: "tv", TMDBID: showId, Status: entity.WATCHING,
	}, domain.WatchedAddExtraProps{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.FinishSeason(f.uid+1, SeasonRequest{WatchedID: added.ID, SeasonNumber: 1})
	var ue UserError
	if !errors.As(err, &ue) {
		t.Errorf("other user err = %v, want UserError", err)
	}
}
