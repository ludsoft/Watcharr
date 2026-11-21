package imprt

import (
	"testing"
	"time"

	"github.com/sbondCo/Watcharr/domain"
)

func TestNetflixParseSeasonLabel(t *testing.T) {
	tests := []struct {
		label string
		want  int
		ok    bool
	}{
		{"Season 1", 1, true},
		{"season 12", 12, true},
		{"Series 3", 3, true},
		{"Part 2", 2, true},
		{"Volume 1", 1, true},
		{"Vol. 4", 4, true},
		{"Chapter 5", 5, true},
		{"Book 3", 3, true},
		{"Part I", 1, true},
		{"Part II", 2, true},
		{"Part IV", 4, true},
		{"Limited Series", 1, true},
		{"Miniseries", 1, true},
		{"2", 2, true},
		{"Stranger Things 2", 2, true},
		{"Blood and Sand", 0, false},
		{"Discovery", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := netflixParseSeasonLabel(tt.label)
		if got != tt.want || ok != tt.ok {
			t.Errorf("netflixParseSeasonLabel(%q) = %d, %v; want %d, %v",
				tt.label, got, ok, tt.want, tt.ok)
		}
	}
}

func TestNetflixNormalizeTitle(t *testing.T) {
	tests := map[string]string{
		"Chapter One: The Vanishing of Will Byers": "chapter one the vanishing of will byers",
		"Don’t Look Back":                          "dont look back",
		"Don't  Look   Back!":                      "dont look back",
		"Rock & Roll":                              "rock and roll",
		"Épisode — Final":                          "épisode final",
		"  ...  ":                                  "",
		"That Hope Is You, Part 2":                 "that hope is you 2",
		"Mayday (2)":                               "mayday 2",
	}
	for in, want := range tests {
		if got := netflixNormalizeTitle(in); got != want {
			t.Errorf("netflixNormalizeTitle(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestNetflixMatchEpisode(t *testing.T) {
	c := func(s, n int, name string) netflixCandidate {
		return netflixCandidate{Season: s, Number: n, Name: netflixNormalizeTitle(name)}
	}
	candidates := []netflixCandidate{
		c(0, 1, "Pilot"),
		c(1, 1, "Chapter One: The Vanishing of Will Byers"),
		c(1, 2, "Chapter Two: The Weirdo on Maple Street"),
		c(1, 3, "Pilot"),
		c(2, 1, "Chapter One: MADMAX"),
		c(2, 2, "Pilot"),
		c(2, 3, "Episode 3"),
		c(3, 1, "The Long Goodbye"),
	}
	tests := []struct {
		name        string
		label       string
		title       string
		wantSeason  int
		wantEpisode int
		wantOk      bool
	}{
		{"exact title", "Season 1", "Chapter One: The Vanishing of Will Byers", 1, 1, true},
		{"exact title, season label not needed", "Part 7", "Chapter Two: The Weirdo on Maple Street", 1, 2, true},
		{"exact title, punctuation differs", "Season 2", "Chapter One - MADMAX", 2, 1, true},
		{"same title in several seasons, season decides", "Season 2", "Pilot", 2, 2, true},
		{"same title, no season: first regular season", "", "Pilot", 1, 3, true},
		{"generic episode title by number", "Season 2", "Episode 3", 2, 3, true},
		{"shortened title", "Season 3", "Long Goodbye", 3, 1, true},
		{"no match", "Season 1", "Something Else Entirely", 0, 0, false},
		{"too short for partial match", "Season 1", "Ch", 0, 0, false},
		{"two segments: first one", "Season 3", "The Long Goodbye / Another One", 3, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := netflixMatchEpisode(candidates, domain.NetflixEpisode{
				SeasonLabel: tt.label,
				Title:       tt.title,
			})
			if ok != tt.wantOk || got.Season != tt.wantSeason || got.Number != tt.wantEpisode {
				t.Errorf("got S%dE%d ok=%v; want S%dE%d ok=%v",
					got.Season, got.Number, ok, tt.wantSeason, tt.wantEpisode, tt.wantOk)
			}
		})
	}
}

func TestNetflixMergePlayDates(t *testing.T) {
	day := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}

	t.Run("no existing plays: all new", func(t *testing.T) {
		updates, newDates := netflixMergePlayDates(nil, []time.Time{day("2024-01-02"), day("2023-05-01")})
		if len(updates) != 0 || len(newDates) != 2 {
			t.Fatalf("updates=%v newDates=%v", updates, newDates)
		}
		if !newDates[0].Equal(day("2023-05-01")) {
			t.Errorf("new dates should be sorted, got %v", newDates)
		}
	})

	t.Run("less than a year: play is moved", func(t *testing.T) {
		updates, newDates := netflixMergePlayDates(
			[]netflixPlay{{ID: 7, Date: day("2024-03-01")}},
			[]time.Time{day("2024-01-02")})
		if len(newDates) != 0 || len(updates) != 1 ||
			updates[0].ID != 7 || !updates[0].Date.Equal(day("2024-01-02")) {
			t.Fatalf("updates=%v newDates=%v", updates, newDates)
		}
	})

	t.Run("more than a year: new play", func(t *testing.T) {
		updates, newDates := netflixMergePlayDates(
			[]netflixPlay{{ID: 7, Date: day("2020-03-01")}},
			[]time.Time{day("2024-01-02")})
		if len(updates) != 0 || len(newDates) != 1 {
			t.Fatalf("updates=%v newDates=%v", updates, newDates)
		}
	})

	t.Run("same date: nothing to do (re-import)", func(t *testing.T) {
		updates, newDates := netflixMergePlayDates(
			[]netflixPlay{{ID: 7, Date: day("2024-01-02")}},
			[]time.Time{day("2024-01-02")})
		if len(updates) != 0 || len(newDates) != 0 {
			t.Fatalf("updates=%v newDates=%v", updates, newDates)
		}
	})

	t.Run("each existing play is used once, closest first", func(t *testing.T) {
		updates, newDates := netflixMergePlayDates(
			[]netflixPlay{{ID: 1, Date: day("2024-01-10")}, {ID: 2, Date: day("2024-06-01")}},
			[]time.Time{day("2024-01-01"), day("2024-02-01"), day("2024-05-20")})
		// 01-01 takes play 1 (closest), 02-01 finds play 2 (play 1 used),
		// 05-20 has nothing left: new play.
		if len(updates) != 2 || len(newDates) != 1 {
			t.Fatalf("updates=%v newDates=%v", updates, newDates)
		}
		if updates[0].ID != 1 || updates[1].ID != 2 || !newDates[0].Equal(day("2024-05-20")) {
			t.Errorf("updates=%v newDates=%v", updates, newDates)
		}
	})
}

func TestNetflixPickSameTitle(t *testing.T) {
	day := func(s string) time.Time {
		d, _ := time.Parse("2006-01-02", s)
		return d
	}
	m := func(id int, name, release string, votes uint) domain.Media {
		r := domain.Media{Name: name, RatingCount: votes}
		r.IDs.TMDB = id
		if release != "" {
			r.ReleaseDate = day(release)
		}
		return r
	}
	watched := day("2024-06-01")
	tests := []struct {
		name    string
		results []domain.Media
		wantID  int
		wantOk  bool
	}{
		{"10x more votes: picked", []domain.Media{
			m(1, "Rush Hour", "2018-01-01", 300),
			m(2, "Rush Hour", "1998-09-18", 5000),
			m(3, "Rush Hour 2", "2001-08-03", 4000),
		}, 2, true},
		{"less than 10x: ask", []domain.Media{
			m(1, "Casino Royale", "2006-11-14", 9000),
			m(2, "Casino Royale", "1967-04-13", 1000),
		}, 0, false},
		{"exactly 10x: picked", []domain.Media{
			m(1, "Atlas", "2024-05-24", 1000),
			m(2, "atlas", "2018-01-01", 100),
		}, 1, true},
		{"released after first watch ignored, one left: picked", []domain.Media{
			m(1, "The Rip", "2026-01-16", 900),
			m(2, "The Rip", "2011-01-01", 800),
		}, 2, true},
		{"too few votes overall: ask", []domain.Media{
			m(1, "Obscure", "2010-01-01", 5),
			m(2, "Obscure", "2012-01-01", 0),
		}, 0, false},
		{"punctuation differs: same title", []domain.Media{
			m(1, "Ford v Ferrari", "2019-11-13", 8000),
			m(2, "Ford v Ferrari: Bringing the Rivalry to Life", "2020-02-11", 10),
		}, 1, true},
		{"no exact title: ask", []domain.Media{
			m(1, "The Suicide Squad", "2021-07-28", 9000),
			m(2, "The Monster Squad", "1987-08-14", 500),
		}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := tt.results[0].Name
			switch tt.name {
			case "no exact title: ask":
				name = "The Squad"
			case "punctuation differs: same title":
				name = "Ford v. Ferrari"
			}
			got, ok := netflixPickSameTitle(name, tt.results, watched)
			if ok != tt.wantOk || (ok && got.IDs.TMDB != tt.wantID) {
				t.Errorf("got id=%d ok=%v; want id=%d ok=%v", got.IDs.TMDB, ok, tt.wantID, tt.wantOk)
			}
		})
	}
}
