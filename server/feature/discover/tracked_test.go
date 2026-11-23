package discover

import (
	"testing"

	"github.com/sbondCo/Watcharr/database/entity"
	"github.com/sbondCo/Watcharr/domain"
)

func TestRemoveTracked(t *testing.T) {
	results := []domain.Media{
		{Name: "untracked"},
		{Name: "finished", Watched: domain.WatchedDto{ID: 1, Status: entity.FINISHED}},
		{Name: "planned", Watched: domain.WatchedDto{ID: 2, Status: entity.PLANNED}},
		{Name: "untracked 2"},
		{Name: "no status", Watched: domain.WatchedDto{ID: 3}},
	}
	got := removeTracked(results)
	if len(got) != 2 || got[0].Name != "untracked" || got[1].Name != "untracked 2" {
		t.Errorf("unexpected results: %+v", got)
	}

	if got := removeTracked(nil); got == nil || len(got) != 0 {
		t.Errorf("nil input: expected an empty (non nil) slice, got %#v", got)
	}
}
