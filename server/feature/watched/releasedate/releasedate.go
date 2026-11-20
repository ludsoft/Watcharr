// releasedate implements "Finished at release date": marking content as
// finished, with the watch dated at the content's release date instead of now.
//
//   - Movie / game: one play dated at its release date.
//   - Show: every released episode is finished at its own air date, every
//     season at its last released episode and the show (the play) at the last
//     released episode of the whole show. Specials (season 0) are skipped.
//   - Season: its released episodes at their air dates, the season at its
//     last released episode (normal automations then run for the show).
//   - Episode: finished at its air date (normal automations run).
//
// Dates are stored at 12:00 UTC so they land on the same calendar day in
// every timezone the analytics page may use.
package releasedate

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/sbondCo/Watcharr/database/entity"
	"github.com/sbondCo/Watcharr/domain"
	"github.com/sbondCo/Watcharr/media/tmdb"
	"github.com/sbondCo/Watcharr/util"
	"gorm.io/gorm"
)

type WatchedProvider interface {
	AddWatched(userId uint, ar domain.WatchedAddRequest, extraProps domain.WatchedAddExtraProps) (entity.Watched, error)
	UpdateWatched(userId uint, id uint, ar domain.WatchedUpdateRequest) (domain.WatchedUpdateResponse, error)
	GetWatchedItemById(userId uint, id uint) (entity.Watched, error)
	GetWatchedItemByTmdbId(userId uint, tmdbId uint, contentType entity.ContentType) (entity.Watched, error)
	GetWatchedItemByIgdbId(userId uint, igdbId uint) (entity.Watched, error)
}

type SeasonSetter interface {
	SetWatchedSeason(userId uint, ar domain.WatchedSeasonSetRequest) (domain.WatchedSeasonSetResponse, error)
}

type EpisodeSetter interface {
	SetWatchedEpisode(userId uint, ar domain.WatchedEpisodeSetRequest) (domain.WatchedEpisodeSetResponse, error)
}

type ContentProvider interface {
	GetOrCacheContent(contentType entity.ContentType, tmdbId int) (entity.Content, error)
}

type GameProvider interface {
	GetOrCache(igdbID int) (entity.Game, error)
}

type TMDBProvider interface {
	ShowDetails(o tmdb.ShowDetailsOptions) (tmdb.ShowDetails, error)
	SeasonDetails(showId string, seasonNumber string) (tmdb.SeasonDetails, error)
}

type ContentRequest struct {
	ContentType util.SupportedMedia `json:"contentType" binding:"required,oneof=movie tv game"`
	TMDBID      int                 `json:"tmdbId"`
	IGDBID      int                 `json:"igdbId"`
}

type SeasonRequest struct {
	WatchedID    uint `json:"watchedId" binding:"required"`
	SeasonNumber int  `json:"seasonNumber"`
}

type EpisodeRequest struct {
	WatchedID     uint `json:"watchedId" binding:"required"`
	SeasonNumber  int  `json:"seasonNumber"`
	EpisodeNumber int  `json:"episodeNumber" binding:"required"`
}

// An error caused by the request (eg: nothing released yet), safe to show.
type UserError struct{ msg string }

func (e UserError) Error() string { return e.msg }

var (
	errNoReleaseDate = UserError{"release date unknown"}
	errNotReleased   = UserError{"not released yet"}
	errNoEpisodes    = UserError{"no released episodes"}
)

type Service struct {
	wp     WatchedProvider
	season SeasonSetter
	ep     EpisodeSetter
	cp     ContentProvider
	gp     GameProvider
	tmdb   TMDBProvider
	// Overridable for tests.
	now func() time.Time
}

func NewService(
	wp WatchedProvider,
	season SeasonSetter,
	ep EpisodeSetter,
	cp ContentProvider,
	gp GameProvider,
	tmdb TMDBProvider,
) *Service {
	return &Service{wp, season, ep, cp, gp, tmdb, time.Now}
}

// Normalize a date to 12:00 UTC of its (UTC) calendar day.
func atNoon(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.UTC)
}

// Parse a tmdb `YYYY-MM-DD` date.
func parseDay(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return atNoon(t), true
}

// Has the day `t` started (today counts as released)?
func (s *Service) released(t time.Time) bool {
	return t.Format("2006-01-02") <= s.now().UTC().Format("2006-01-02")
}

func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// Finish a movie, show or game at its release date.
func (s *Service) FinishContent(userId uint, req ContentRequest) (domain.WatchedDto, error) {
	switch req.ContentType {
	case util.SupportedMediaMovie:
		return s.finishMovie(userId, req.TMDBID)
	case util.SupportedMediaShow:
		return s.finishShow(userId, req.TMDBID)
	case util.SupportedMediaGame:
		return s.finishGame(userId, req.IGDBID)
	}
	return domain.WatchedDto{}, UserError{"invalid content type"}
}

func (s *Service) finishMovie(userId uint, tmdbId int) (domain.WatchedDto, error) {
	if tmdbId == 0 {
		return domain.WatchedDto{}, UserError{"missing tmdb id"}
	}
	content, err := s.cp.GetOrCacheContent(entity.MOVIE, tmdbId)
	if err != nil {
		return domain.WatchedDto{}, err
	}
	if content.ReleaseDate == nil || content.ReleaseDate.IsZero() {
		return domain.WatchedDto{}, errNoReleaseDate
	}
	date := atNoon(*content.ReleaseDate)
	if !s.released(date) {
		return domain.WatchedDto{}, errNotReleased
	}
	existing, err := s.wp.GetWatchedItemByTmdbId(userId, uint(tmdbId), entity.MOVIE)
	if err != nil && !isNotFound(err) {
		return domain.WatchedDto{}, err
	}
	if _, err := s.setFinished(userId, existing, domain.WatchedAddRequest{
		ContentType: util.SupportedMediaMovie,
		TMDBID:      tmdbId,
	}, date); err != nil {
		return domain.WatchedDto{}, err
	}
	return s.reloadByTmdb(userId, tmdbId, entity.MOVIE)
}

func (s *Service) finishGame(userId uint, igdbId int) (domain.WatchedDto, error) {
	if igdbId == 0 {
		return domain.WatchedDto{}, UserError{"missing igdb id"}
	}
	game, err := s.gp.GetOrCache(igdbId)
	if err != nil {
		return domain.WatchedDto{}, err
	}
	if game.ReleaseDate == nil || game.ReleaseDate.IsZero() {
		return domain.WatchedDto{}, errNoReleaseDate
	}
	date := atNoon(*game.ReleaseDate)
	if !s.released(date) {
		return domain.WatchedDto{}, errNotReleased
	}
	existing, err := s.wp.GetWatchedItemByIgdbId(userId, uint(igdbId))
	if err != nil && !isNotFound(err) {
		return domain.WatchedDto{}, err
	}
	if _, err := s.setFinished(userId, existing, domain.WatchedAddRequest{
		ContentType: util.SupportedMediaGame,
		IGDBID:      igdbId,
	}, date); err != nil {
		return domain.WatchedDto{}, err
	}
	w, err := s.wp.GetWatchedItemByIgdbId(userId, uint(igdbId))
	if err != nil {
		return domain.WatchedDto{}, err
	}
	return domain.NewWatchedDtoForContentPage(&w), nil
}

// Add the watched entry as FINISHED, or set an existing one to FINISHED,
// with the play dated at `date`. Already finished entries are left alone
// (no extra play). Returns the watched entry id.
func (s *Service) setFinished(
	userId uint,
	existing entity.Watched,
	add domain.WatchedAddRequest,
	date time.Time,
) (uint, error) {
	if existing.ID == 0 {
		add.Status = entity.FINISHED
		add.ActivityDate = &date
		w, err := s.wp.AddWatched(userId, add, domain.WatchedAddExtraProps{
			ActivityCreatedBy: entity.ActivityCreatedByWatcharr,
		})
		if err != nil {
			return 0, err
		}
		return w.ID, nil
	}
	if existing.Status != entity.FINISHED {
		_, err := s.wp.UpdateWatched(userId, existing.ID, domain.WatchedUpdateRequest{
			Status:            entity.FINISHED,
			ActivityDate:      &date,
			ActivityCreatedBy: entity.ActivityCreatedByWatcharr,
		})
		if err != nil {
			return 0, err
		}
	}
	return existing.ID, nil
}

type releasedEpisode struct {
	season  int
	episode int
	date    time.Time
}

// Released episodes of a season, sorted by episode number.
func (s *Service) releasedEpisodes(tmdbId int, seasonNum int) ([]releasedEpisode, error) {
	sd, err := s.tmdb.SeasonDetails(strconv.Itoa(tmdbId), strconv.Itoa(seasonNum))
	if err != nil {
		return nil, err
	}
	eps := []releasedEpisode{}
	for _, e := range sd.Episodes {
		d, ok := parseDay(e.AirDate)
		if !ok || !s.released(d) {
			continue
		}
		eps = append(eps, releasedEpisode{seasonNum, e.EpisodeNumber, d})
	}
	sort.Slice(eps, func(i, j int) bool { return eps[i].episode < eps[j].episode })
	return eps, nil
}

func lastDate(eps []releasedEpisode) time.Time {
	var last time.Time
	for _, e := range eps {
		if e.date.After(last) {
			last = e.date
		}
	}
	return last
}

// Finish episodes (at their air dates) and then their season (at its last
// episode). Hooks are skipped for episodes; for the season only if
// `skipSeasonHooks`.
func (s *Service) finishEpisodesAndSeason(
	userId uint,
	watchedId uint,
	seasonNum int,
	eps []releasedEpisode,
	skipSeasonHooks bool,
) error {
	for _, e := range eps {
		_, err := s.ep.SetWatchedEpisode(userId, domain.WatchedEpisodeSetRequest{
			WatchedID:         watchedId,
			SeasonNumber:      e.season,
			EpisodeNumber:     e.episode,
			Status:            entity.FINISHED,
			AddActivityDate:   e.date,
			AddActivityReason: "Finished at release date.",
			ActivityCreatedBy: entity.ActivityCreatedByWatcharr,
			DateStatusChange:  true,
			SkipHooks:         true,
		})
		if err != nil {
			return fmt.Errorf("failed to finish S%dE%d: %w", e.season, e.episode, err)
		}
	}
	_, err := s.season.SetWatchedSeason(userId, domain.WatchedSeasonSetRequest{
		WatchedID:         watchedId,
		SeasonNumber:      seasonNum,
		Status:            entity.FINISHED,
		AddActivityDate:   lastDate(eps),
		ActivityReason:    "Finished at release date.",
		ActivityCreatedBy: entity.ActivityCreatedByWatcharr,
		DateStatusChange:  true,
		SkipHooks:         skipSeasonHooks,
	})
	if err != nil {
		return fmt.Errorf("failed to finish season %d: %w", seasonNum, err)
	}
	return nil
}

func (s *Service) finishShow(userId uint, tmdbId int) (domain.WatchedDto, error) {
	if tmdbId == 0 {
		return domain.WatchedDto{}, UserError{"missing tmdb id"}
	}
	details, err := s.tmdb.ShowDetails(tmdb.ShowDetailsOptions{ID: strconv.Itoa(tmdbId)})
	if err != nil {
		return domain.WatchedDto{}, err
	}
	seasons := map[int][]releasedEpisode{}
	seasonNums := []int{}
	var all []releasedEpisode
	for _, season := range details.Seasons {
		// Skip specials, their air dates are all over the place.
		if season.SeasonNumber <= 0 {
			continue
		}
		eps, err := s.releasedEpisodes(tmdbId, season.SeasonNumber)
		if err != nil {
			return domain.WatchedDto{}, err
		}
		if len(eps) == 0 {
			continue
		}
		seasons[season.SeasonNumber] = eps
		seasonNums = append(seasonNums, season.SeasonNumber)
		all = append(all, eps...)
	}
	if len(all) == 0 {
		return domain.WatchedDto{}, errNoEpisodes
	}
	sort.Ints(seasonNums)

	existing, err := s.wp.GetWatchedItemByTmdbId(userId, uint(tmdbId), entity.SHOW)
	if err != nil && !isNotFound(err) {
		return domain.WatchedDto{}, err
	}
	// Show first, so the episodes/seasons below don't trigger automations.
	watchedId, err := s.setFinished(userId, existing, domain.WatchedAddRequest{
		ContentType: util.SupportedMediaShow,
		TMDBID:      tmdbId,
	}, lastDate(all))
	if err != nil {
		return domain.WatchedDto{}, err
	}
	for _, n := range seasonNums {
		if err := s.finishEpisodesAndSeason(userId, watchedId, n, seasons[n], true); err != nil {
			slog.Error("finishShow: Failed", "error", err)
			return domain.WatchedDto{}, err
		}
	}
	return s.reloadByTmdb(userId, tmdbId, entity.SHOW)
}

// Get a show watched entry (must be the users and a show).
func (s *Service) getShow(userId uint, watchedId uint) (entity.Watched, error) {
	w, err := s.wp.GetWatchedItemById(userId, watchedId)
	if err != nil {
		return entity.Watched{}, err
	}
	if w.ID == 0 || w.Content == nil || w.Content.Type != entity.SHOW {
		return entity.Watched{}, UserError{"watched entry is not a show"}
	}
	return w, nil
}

// Finish a season of a show on the users list at its release dates.
func (s *Service) FinishSeason(userId uint, req SeasonRequest) (domain.WatchedDto, error) {
	w, err := s.getShow(userId, req.WatchedID)
	if err != nil {
		return domain.WatchedDto{}, err
	}
	eps, err := s.releasedEpisodes(w.Content.TmdbID, req.SeasonNumber)
	if err != nil {
		return domain.WatchedDto{}, err
	}
	if len(eps) == 0 {
		return domain.WatchedDto{}, errNoEpisodes
	}
	if err := s.finishEpisodesAndSeason(userId, w.ID, req.SeasonNumber, eps, false); err != nil {
		return domain.WatchedDto{}, err
	}
	return s.reloadByTmdb(userId, w.Content.TmdbID, entity.SHOW)
}

// Finish an episode of a show on the users list at its air date.
func (s *Service) FinishEpisode(userId uint, req EpisodeRequest) (domain.WatchedDto, error) {
	w, err := s.getShow(userId, req.WatchedID)
	if err != nil {
		return domain.WatchedDto{}, err
	}
	sd, err := s.tmdb.SeasonDetails(strconv.Itoa(w.Content.TmdbID), strconv.Itoa(req.SeasonNumber))
	if err != nil {
		return domain.WatchedDto{}, err
	}
	var date time.Time
	found := false
	for _, e := range sd.Episodes {
		if e.EpisodeNumber == req.EpisodeNumber {
			d, ok := parseDay(e.AirDate)
			if !ok {
				return domain.WatchedDto{}, errNoReleaseDate
			}
			date, found = d, true
			break
		}
	}
	if !found {
		return domain.WatchedDto{}, UserError{"episode not found"}
	}
	if !s.released(date) {
		return domain.WatchedDto{}, errNotReleased
	}
	_, err = s.ep.SetWatchedEpisode(userId, domain.WatchedEpisodeSetRequest{
		WatchedID:         w.ID,
		SeasonNumber:      req.SeasonNumber,
		EpisodeNumber:     req.EpisodeNumber,
		Status:            entity.FINISHED,
		AddActivityDate:   date,
		AddActivityReason: "Finished at release date.",
		ActivityCreatedBy: entity.ActivityCreatedByWatcharr,
		DateStatusChange:  true,
	})
	if err != nil {
		return domain.WatchedDto{}, err
	}
	return s.reloadByTmdb(userId, w.Content.TmdbID, entity.SHOW)
}

// Full watched entry (activity, seasons, episodes, plays) for the client.
func (s *Service) reloadByTmdb(userId uint, tmdbId int, ct entity.ContentType) (domain.WatchedDto, error) {
	w, err := s.wp.GetWatchedItemByTmdbId(userId, uint(tmdbId), ct)
	if err != nil {
		return domain.WatchedDto{}, err
	}
	return domain.NewWatchedDtoForContentPage(&w), nil
}
