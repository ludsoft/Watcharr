package imprt

// Fork (ludsoft): Netflix viewing history import.
//
// The client parses `NetflixViewingHistory.csv` (Title,Date) and sends one
// ImportRequest per movie/show with `Netflix` set. Matching (search by name,
// IMPORT_MULTI when the user must choose) is the generic importer's, except
// that among results with exactly the same title, one is picked automatically
// when it clearly stands out (see netflixPickSameTitle). Once a match is
// found, `netflixSuccessfulImport` adds the item, or merges with the existing
// watched entry:
//   - Everything is imported as FINISHED.
//   - Movie: one play per date. Show: one play, dated on the last episode,
//     and each matched episode is set as watched on its date.
//   - Existing entry: a play less than a year from an imported play date is
//     moved to that date, otherwise a new play is added.

import (
	"errors"
	"log/slog"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/sbondCo/Watcharr/activity"
	"github.com/sbondCo/Watcharr/database/entity"
	"github.com/sbondCo/Watcharr/domain"
	"github.com/sbondCo/Watcharr/media/tmdb"
	"github.com/sbondCo/Watcharr/util"
	"gorm.io/gorm"
)

const netflixCreatedBy = entity.ActivityCreatedByNetflixImport

// An existing play closer than this to an imported play date is considered
// the same viewing, so its date is updated instead of adding a play.
const netflixSamePlayWindow = 365 * 24 * time.Hour

// Among results with the same title, the most voted one is picked when it
// has at least this many times the votes of every other one.
const netflixVotesFactor = 10

// Entry point for Netflix items without a TMDB ID yet.
func (s *Service) netflixImport(
	userId uint,
	ar *domain.ImportRequest,
) (domain.ImportResponse, error) {
	if ar.Netflix.Group {
		return s.netflixImportGroup(userId, ar)
	}
	if ar.Netflix.FallbackShow != nil {
		return s.netflixImportAmbiguous(userId, ar)
	}
	return s.netflixImportWithName(userId, ar)
}

// The generic search by name, then when it ends with multiple results, try
// to pick one among those with exactly the same title.
func (s *Service) netflixImportWithName(
	userId uint,
	ar *domain.ImportRequest,
) (domain.ImportResponse, error) {
	resp, err := s.importWithName(userId, ar)
	if err != nil || resp.Type != domain.IMPORT_MULTI {
		return resp, err
	}
	pick, ok := netflixPickSameTitle(ar.Name, resp.Results, netflixFirstWatched(ar.Netflix))
	if !ok {
		return resp, nil
	}
	slog.Debug("netflixImportWithName: picked among same titles",
		"name", ar.Name, "tmdb_id", pick.IDs.TMDB, "release", pick.ReleaseDate)
	props, err := domain.NewSuccessfulImportPropsFromMedia(&pick)
	if err != nil {
		return resp, nil
	}
	return s.SuccessfulImport(userId, ar, props), nil
}

// Earliest date in the Netflix data (zero if none).
func netflixFirstWatched(n *domain.NetflixImportData) time.Time {
	var first time.Time
	consider := func(d time.Time) {
		if !d.IsZero() && (first.IsZero() || d.Before(first)) {
			first = d
		}
	}
	for _, d := range n.Dates {
		consider(d)
	}
	for _, e := range n.Episodes {
		consider(e.Date)
	}
	if n.FallbackShow != nil {
		for _, e := range n.FallbackShow.Episodes {
			consider(e.Date)
		}
	}
	return first
}

// Rows like "Name: Something" can be a movie (with a subtitle) or an episode
// of a show without seasons. Import as a movie when a movie has exactly that
// name, else as the show when a show has exactly the prefix name, else go
// the usual movie way (single result, multiple results, not found).
func (s *Service) netflixImportAmbiguous(
	userId uint,
	ar *domain.ImportRequest,
) (domain.ImportResponse, error) {
	fb := ar.Netflix.FallbackShow
	if s.netflixHasExactMatch(userId, ar.Name, domain.SearchTypeMovie) {
		slog.Debug("netflixImportAmbiguous: exact movie match", "name", ar.Name)
		return s.netflixImportWithName(userId, ar)
	}
	if s.netflixHasExactMatch(userId, fb.Name, domain.SearchTypeShow) {
		slog.Debug("netflixImportAmbiguous: exact show match",
			"name", ar.Name, "show", fb.Name)
		showReq := *ar
		showReq.Name = fb.Name
		showReq.Type = domain.ImportContentTypeShow
		showReq.Netflix = &domain.NetflixImportData{Episodes: fb.Episodes}
		return s.netflixImportWithName(userId, &showReq)
	}
	return s.netflixImportWithName(userId, ar)
}

// How many of a group's rows are checked for exact movie matches.
const netflixGroupMovieChecks = 4

// Grouped rows without a season label: movies of a franchise
// ("Mission: Impossible - Fallout") or episodes of a show ("Gotham: Arkham").
func (s *Service) netflixImportGroup(
	userId uint,
	ar *domain.ImportRequest,
) (domain.ImportResponse, error) {
	checked, movies := 0, 0
	for _, t := range ar.Netflix.RowTitles {
		if checked == netflixGroupMovieChecks {
			break
		}
		checked++
		if s.netflixHasExactMatch(userId, t, domain.SearchTypeMovie) {
			movies++
		}
	}
	if checked > 0 && movies*2 >= checked {
		slog.Debug("netflixImportGroup: rows are movies, asking client to split",
			"name", ar.Name, "movies", movies, "checked", checked)
		return domain.ImportResponse{Type: domain.IMPORT_NOTFOUND, NetflixSplit: true}, nil
	}
	showReq := *ar
	showReq.Type = domain.ImportContentTypeShow
	for _, name := range ar.Netflix.ShowNames {
		if s.netflixHasExactMatch(userId, name, domain.SearchTypeShow) {
			showReq.Name = name
			return s.netflixImportWithName(userId, &showReq)
		}
	}
	if n := len(ar.Netflix.ShowNames); n > 0 {
		showReq.Name = ar.Netflix.ShowNames[n-1]
	}
	return s.netflixImportWithName(userId, &showReq)
}

func (s *Service) netflixHasExactMatch(userId uint, name string, t domain.SearchType) bool {
	resp, err := s.searchProvider.Search(
		domain.SearchRequest{Query: name, Type: t},
		util.PaginationParams{Page: 1},
		userId,
	)
	if err != nil {
		slog.Error("netflixHasExactMatch: Search failed", "error", err)
		return false
	}
	for _, r := range resp.Results {
		if strings.EqualFold(r.Name, name) {
			return true
		}
	}
	return false
}

func (s *Service) netflixSuccessfulImport(
	userId uint,
	ar *domain.ImportRequest,
	props domain.SuccessfulImportProps,
) domain.ImportResponse {
	var contentType entity.ContentType
	switch props.ContentType {
	case util.SupportedMediaMovie:
		contentType = entity.MOVIE
	case util.SupportedMediaShow:
		contentType = entity.SHOW
	default:
		slog.Error("netflixSuccessfulImport: Unsupported content type",
			"content_type", props.ContentType)
		return domain.ImportResponse{Type: domain.IMPORT_FAILED}
	}

	// Dates of the rows, and the dates that count as plays.
	var dates []time.Time
	var episodes []domain.NetflixEpisode
	if contentType == entity.MOVIE {
		dates = append(dates, ar.Netflix.Dates...)
	} else {
		episodes = ar.Netflix.Episodes
		// The user picked a show for an ambiguous row (IMPORT_MULTI).
		if len(episodes) == 0 && ar.Netflix.FallbackShow != nil {
			episodes = ar.Netflix.FallbackShow.Episodes
		}
		for _, e := range episodes {
			dates = append(dates, e.Date)
		}
	}
	if len(dates) == 0 {
		dates = append(dates, time.Now())
	}
	slices.SortFunc(dates, func(a, b time.Time) int { return a.Compare(b) })
	playDates := dates
	if contentType == entity.SHOW {
		playDates = []time.Time{dates[len(dates)-1]}
	}

	w, err := s.wp.GetWatchedItemByTmdbId(userId, uint(props.TmdbID), contentType)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Error("netflixSuccessfulImport: Failed to look for an existing entry",
			"error", err)
		return domain.ImportResponse{Type: domain.IMPORT_FAILED}
	}
	if err == nil {
		s.netflixMergeExisting(userId, w, playDates)
	} else {
		w, err = s.netflixAddNew(userId, props, dates[0], playDates)
		if err != nil {
			if errors.Is(err, domain.ErrWatchedExists) {
				return domain.ImportResponse{Type: domain.IMPORT_EXISTS}
			}
			return domain.ImportResponse{Type: domain.IMPORT_FAILED}
		}
	}

	resp := domain.ImportResponse{Type: domain.IMPORT_SUCCESS}
	if contentType == entity.SHOW && len(episodes) > 0 {
		resp.UnmatchedEpisodes = s.netflixSetEpisodes(userId, w.ID, props.TmdbID, episodes)
	}
	// Return the up to date entry (activity, episodes).
	if fresh, err := s.wp.GetWatchedItemByTmdbId(userId, uint(props.TmdbID), contentType); err == nil {
		w = fresh
	}
	resp.WatchedEntry = w
	return resp
}

// Add a new FINISHED watched entry. The add activity is the first play.
func (s *Service) netflixAddNew(
	userId uint,
	props domain.SuccessfulImportProps,
	firstDate time.Time,
	playDates []time.Time,
) (entity.Watched, error) {
	firstPlay := playDates[0]
	w, err := s.wp.AddWatched(
		userId,
		domain.WatchedAddRequest{
			ContentType:  props.ContentType,
			TMDBID:       props.TmdbID,
			Status:       entity.FINISHED,
			WatchedDate:  firstDate,
			ActivityDate: &firstPlay,
		},
		domain.WatchedAddExtraProps{ActivityCreatedBy: netflixCreatedBy},
	)
	if err != nil {
		slog.Error("netflixAddNew: Failed to add watched entry", "error", err)
		return w, err
	}
	for _, d := range playDates[1:] {
		s.netflixAddPlay(userId, w.ID, d)
	}
	return w, nil
}

// Rule for items already on the list: set them FINISHED, then move existing
// plays less than a year away to the imported dates, or add new plays.
func (s *Service) netflixMergeExisting(userId uint, w entity.Watched, playDates []time.Time) {
	if w.Status != entity.FINISHED {
		err := s.db.Model(&entity.Watched{}).
			Where("id = ? AND user_id = ?", w.ID, userId).
			Update("status", entity.FINISHED).Error
		if err != nil {
			slog.Error("netflixMergeExisting: Failed to set status", "error", err)
		} else {
			last := playDates[len(playDates)-1]
			_, err = activity.
				NewCreator(s.db, userId, w.ID, entity.STATUS_CHANGED, false, netflixCreatedBy).
				SetData(string(entity.FINISHED)).
				SetCustomDate(&last).
				Create()
			if err != nil {
				slog.Error("netflixMergeExisting: Failed to add status activity", "error", err)
			}
		}
	}

	var plays []netflixPlay
	for _, a := range w.Activity {
		if !a.CountAsPlay {
			continue
		}
		d := a.CreatedAt
		if a.CustomDate != nil && !a.CustomDate.IsZero() {
			d = *a.CustomDate
		}
		plays = append(plays, netflixPlay{ID: a.ID, Date: d})
	}
	updates, newDates := netflixMergePlayDates(plays, playDates)
	for _, u := range updates {
		err := s.db.Model(&entity.Activity{}).
			Where("id = ? AND user_id = ?", u.ID, userId).
			Update("custom_date", u.Date).Error
		if err != nil {
			slog.Error("netflixMergeExisting: Failed to update play date",
				"activity_id", u.ID, "error", err)
		}
	}
	for _, d := range newDates {
		s.netflixAddPlay(userId, w.ID, d)
	}
}

func (s *Service) netflixAddPlay(userId uint, watchedId uint, d time.Time) {
	_, err := activity.
		NewCreator(s.db, userId, watchedId, entity.IMPORTED_ADDED_WATCHED, true, netflixCreatedBy).
		SetCustomDate(&d).
		Create()
	if err != nil {
		slog.Error("netflixAddPlay: Failed to add play", "date", d, "error", err)
	}
}

// Set matched episodes as watched (each on its earliest date). Returns the
// episodes that couldn't be matched.
func (s *Service) netflixSetEpisodes(
	userId uint,
	watchedId uint,
	tmdbId int,
	episodes []domain.NetflixEpisode,
) []domain.NetflixEpisode {
	candidates, err := s.netflixShowEpisodes(tmdbId)
	if err != nil {
		slog.Error("netflixSetEpisodes: Failed to get show episodes", "error", err)
		return episodes
	}
	type key struct{ season, episode int }
	earliest := map[key]time.Time{}
	var order []key
	var unmatched []domain.NetflixEpisode
	for _, e := range episodes {
		c, ok := netflixMatchEpisode(candidates, e)
		if _, isSeason := netflixParseSeasonLabel(e.SeasonLabel); !ok && e.SeasonLabel != "" && !isSeason {
			// A middle part that isn't a season label may be part of the
			// episode title ("Hyperdrive: Knockout 1: Head to Head").
			c, ok = netflixMatchEpisode(candidates, domain.NetflixEpisode{
				Title: e.SeasonLabel + ": " + e.Title,
			})
		}
		if !ok {
			unmatched = append(unmatched, e)
			continue
		}
		k := key{c.Season, c.Number}
		if d, seen := earliest[k]; !seen {
			order = append(order, k)
			earliest[k] = e.Date
		} else if e.Date.Before(d) {
			earliest[k] = e.Date
		}
	}
	for _, k := range order {
		_, err := s.wep.SetWatchedEpisode(userId, domain.WatchedEpisodeSetRequest{
			WatchedID:         watchedId,
			SeasonNumber:      k.season,
			EpisodeNumber:     k.episode,
			Status:            entity.FINISHED,
			AddActivityDate:   earliest[k],
			ActivityCreatedBy: netflixCreatedBy,
			SkipHooks:         true,
		})
		if err != nil {
			slog.Error("netflixSetEpisodes: Failed to set episode",
				"season", k.season, "episode", k.episode, "error", err)
		}
	}
	if len(unmatched) > 0 {
		slog.Info("netflixSetEpisodes: Some episodes couldn't be matched",
			"tmdb_id", tmdbId, "unmatched", len(unmatched), "total", len(episodes))
	}
	return unmatched
}

// All episodes of a show, from TMDB (cached).
func (s *Service) netflixShowEpisodes(tmdbId int) ([]netflixCandidate, error) {
	id := strconv.Itoa(tmdbId)
	show, err := s.tmdb.ShowDetails(tmdb.ShowDetailsOptions{ID: id, DontRunDBCache: true})
	if err != nil {
		return nil, err
	}
	var candidates []netflixCandidate
	for _, season := range show.Seasons {
		sd, err := s.tmdb.SeasonDetails(id, strconv.Itoa(season.SeasonNumber))
		if err != nil {
			slog.Error("netflixShowEpisodes: Failed to get season, skipping it",
				"tmdb_id", tmdbId, "season", season.SeasonNumber, "error", err)
			continue
		}
		for _, e := range sd.Episodes {
			candidates = append(candidates, netflixCandidate{
				Season: e.SeasonNumber,
				Number: e.EpisodeNumber,
				Name:   netflixNormalizeTitle(e.Name),
			})
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("show has no episodes")
	}
	return candidates, nil
}

// ---- Pure helpers (tested in import_netflix_test.go) ----

type netflixCandidate struct {
	Season int
	Number int
	// Normalized episode name.
	Name string
}

type netflixPlay struct {
	ID   uint
	Date time.Time
}

var (
	netflixSeasonLabelRe = regexp.MustCompile(
		`^(?:season|series|part|volume|vol\.?|chapter|book|collection)\s+(\d+|[ivx]+)$`)
	netflixNumberRe         = regexp.MustCompile(`^(\d+)$`)
	netflixGenericEpisodeRe = regexp.MustCompile(`^(?:episode|chapter|ep)\s*(\d+)$`)
)

// Season number from the season part of a Netflix title.
func netflixParseSeasonLabel(label string) (int, bool) {
	l := strings.ToLower(strings.TrimSpace(label))
	if l == "" {
		return 0, false
	}
	if l == "limited series" || l == "miniseries" || l == "mini series" {
		return 1, true
	}
	if m := netflixSeasonLabelRe.FindStringSubmatch(l); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, true
		}
		if n := netflixRoman(m[1]); n > 0 {
			return n, true
		}
	}
	if m := netflixNumberRe.FindStringSubmatch(l); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, true
	}
	// "<Show name> 2" (eg: "Stranger Things 2").
	if i := strings.LastIndex(l, " "); i > 0 {
		if n, err := strconv.Atoi(l[i+1:]); err == nil {
			return n, true
		}
	}
	return 0, false
}

func netflixRoman(s string) int {
	vals := map[rune]int{'i': 1, 'v': 5, 'x': 10}
	total, prev := 0, 0
	for i := len(s) - 1; i >= 0; i-- {
		v := vals[rune(s[i])]
		if v < prev {
			total -= v
		} else {
			total += v
			prev = v
		}
	}
	return total
}

// Lowercase, unify quotes/dashes, drop punctuation, squash spaces. The words
// "part"/"pt" are dropped too: Netflix and TMDB write "Part 2", ", Part 2",
// "(2)" or "Pt. 2" for the same episode.
func netflixNormalizeTitle(t string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(t) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		case r == '&':
			if !lastSpace {
				b.WriteRune(' ')
			}
			b.WriteString("and ")
			lastSpace = true
		case r == '\'' || r == '’' || r == '‘':
			// Drop apostrophes so "don't" == "dont".
		default:
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
		}
	}
	words := strings.Fields(b.String())
	words = slices.DeleteFunc(words, func(w string) bool { return w == "part" || w == "pt" })
	return strings.Join(words, " ")
}

// Find the TMDB episode for a Netflix episode row.
func netflixMatchEpisode(candidates []netflixCandidate, e domain.NetflixEpisode) (netflixCandidate, bool) {
	// "Pups Save a Clown / Pups Save a Mayor": two segments in one episode,
	// TMDB has the first one.
	if first, _, found := strings.Cut(e.Title, " / "); found {
		e.Title = first
	}
	title := netflixNormalizeTitle(e.Title)
	season, hasSeason := netflixParseSeasonLabel(e.SeasonLabel)

	pick := func(list []netflixCandidate) (netflixCandidate, bool) {
		if len(list) == 1 {
			return list[0], true
		}
		if len(list) > 1 && hasSeason {
			var inSeason []netflixCandidate
			for _, c := range list {
				if c.Season == season {
					inSeason = append(inSeason, c)
				}
			}
			if len(inSeason) == 1 {
				return inSeason[0], true
			}
		}
		return netflixCandidate{}, false
	}

	if title != "" {
		// 1. Same title.
		var exact []netflixCandidate
		for _, c := range candidates {
			if c.Name == title {
				exact = append(exact, c)
			}
		}
		if c, ok := pick(exact); ok {
			return c, true
		}
		if len(exact) > 1 {
			// Same title in several seasons and no season to choose: use
			// the first regular season.
			return netflixFirstRegular(exact), true
		}
	}

	// 2. "Episode 3" / "Chapter 3" titles: match by number.
	if m := netflixGenericEpisodeRe.FindStringSubmatch(title); m != nil {
		n, _ := strconv.Atoi(m[1])
		var byNumber []netflixCandidate
		for _, c := range candidates {
			if c.Number == n && c.Season > 0 {
				byNumber = append(byNumber, c)
			}
		}
		if c, ok := pick(byNumber); ok {
			return c, true
		}
	}

	// 3. One title contains the other (eg: a shortened title).
	if len([]rune(title)) >= 4 {
		var partial []netflixCandidate
		for _, c := range candidates {
			if len([]rune(c.Name)) >= 4 &&
				(strings.Contains(c.Name, title) || strings.Contains(title, c.Name)) {
				partial = append(partial, c)
			}
		}
		if c, ok := pick(partial); ok {
			return c, true
		}
	}
	return netflixCandidate{}, false
}

func netflixFirstRegular(list []netflixCandidate) netflixCandidate {
	best := list[0]
	for _, c := range list[1:] {
		bestSpecial, cSpecial := best.Season == 0, c.Season == 0
		if (bestSpecial && !cSpecial) ||
			(bestSpecial == cSpecial && (c.Season < best.Season ||
				(c.Season == best.Season && c.Number < best.Number))) {
			best = c
		}
	}
	return best
}

// Merge imported play dates with existing plays: each imported date moves the
// closest unused existing play less than a year away, or becomes a new play.
// Updates moving a play to the date it already has are left out.
func netflixMergePlayDates(existing []netflixPlay, dates []time.Time) (updates []netflixPlay, newDates []time.Time) {
	sorted := slices.Clone(dates)
	slices.SortFunc(sorted, func(a, b time.Time) int { return a.Compare(b) })
	used := make([]bool, len(existing))
	for _, d := range sorted {
		best := -1
		bestDist := time.Duration(math.MaxInt64)
		for i, p := range existing {
			if used[i] {
				continue
			}
			dist := p.Date.Sub(d)
			if dist < 0 {
				dist = -dist
			}
			if dist < netflixSamePlayWindow && dist < bestDist {
				best, bestDist = i, dist
			}
		}
		if best == -1 {
			newDates = append(newDates, d)
			continue
		}
		used[best] = true
		if !existing[best].Date.Equal(d) {
			updates = append(updates, netflixPlay{ID: existing[best].ID, Date: d})
		}
	}
	return updates, newDates
}

// Pick a result among those with the given title (ignoring case/punctuation):
// results released after the first watch date are ignored (they can't be
// what was watched). If one is left, it's the one. Otherwise the most voted
// is picked only if it has at least netflixVotesFactor times the votes of
// every other one, else the user has to choose.
func netflixPickSameTitle(name string, results []domain.Media, firstWatched time.Time) (domain.Media, bool) {
	var same []domain.Media
	norm := netflixNormalizeTitle(name)
	for _, r := range results {
		// Punctuation may differ ("Ford v. Ferrari" / "Ford v Ferrari").
		if netflixNormalizeTitle(r.Name) != norm {
			continue
		}
		if !firstWatched.IsZero() && !r.ReleaseDate.IsZero() && r.ReleaseDate.After(firstWatched) {
			continue
		}
		same = append(same, r)
	}
	if len(same) == 0 {
		return domain.Media{}, false
	}
	if len(same) == 1 {
		return same[0], true
	}
	slices.SortStableFunc(same, func(a, b domain.Media) int {
		return int(b.RatingCount) - int(a.RatingCount)
	})
	top, second := same[0].RatingCount, same[1].RatingCount
	if top >= netflixVotesFactor && top >= netflixVotesFactor*second {
		return same[0], true
	}
	return domain.Media{}, false
}
