package discover

import "github.com/sbondCo/Watcharr/domain"

// Remove the results that are on the user's watched list (any status).
// Must run after the watched data has been added to the results.
// The pagination of the response is left as is (it follows the provider's
// pages), so a page can have fewer results, or none.
func removeTracked(results []domain.Media) []domain.Media {
	kept := make([]domain.Media, 0, len(results))
	for _, m := range results {
		if m.Watched.ID == 0 {
			kept = append(kept, m)
		}
	}
	return kept
}
