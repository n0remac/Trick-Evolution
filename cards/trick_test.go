package cards

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrintableCatalogSurvivesGameIntegration(t *testing.T) {
	store.RLock()
	counts := map[CardCategory]int{}
	for _, card := range store.m {
		counts[card.Cat]++
	}
	store.RUnlock()
	if counts[CategoryStandard] != 52 || counts[CategoryRule] == 0 || counts[CategoryPhase] == 0 {
		t.Fatalf("print catalog incomplete: %+v", counts)
	}
	mux := http.NewServeMux()
	Trick(mux)
	for _, path := range []string{"/cards", "/cards/sheet"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "High Card Wins") {
			t.Fatalf("%s did not render original rule catalog: status %d", path, response.Code)
		}
	}
}
