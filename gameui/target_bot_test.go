package gameui

import (
	"testing"

	"github.com/n0remac/trick-evolution/domain"
)

func TestBotUsesTargetTeamIntervalAndLegalAuctionBounds(t *testing.T) {
	view := domain.PlayerView{Seat: "bot", Hand: []domain.CardID{"std:H:14", "std:H:13"}, Public: domain.PublicView{
		GameTarget: 21, HandTeams: map[domain.SeatID]int{"bot": 0, "partner": 0, "other": 1, "opponent": 1},
		TeamScores: map[int]int{0: -30, 1: 90}, GameTeamScores: map[int]int{0: 20, 1: 18},
		Auction: &domain.AuctionState{Min: 2, Max: 4, Metric: domain.ValueBidPoints},
	}}
	if bid := chooseBotBid(view); bid != 2 {
		t.Fatalf("target-team bot bid %d, want legal Pitch minimum", bid)
	}
	state := domain.MatchState{HandNumber: 7, GameTeamScores: map[int]int{0: 21, 1: 19}, TeamScores: map[int]int{0: -29, 1: 91}}
	if score := rolloutScore(state, view, ObjectiveMatchMargin); score != 2 {
		t.Fatalf("rollout used historical ledger instead of target interval: %d", score)
	}
	state.GameResult = &domain.TargetGameResult{Hand: 7, Winners: []domain.SeatID{"bot", "partner"}}
	if score := rolloutScore(state, view, ObjectiveMatchMargin); score != 1002 {
		t.Fatalf("rollout missed qualifying target win: %d", score)
	}
	state.GameResult.Winners = []domain.SeatID{"other", "opponent"}
	if score := rolloutScore(state, view, ObjectiveMatchMargin); score != -998 {
		t.Fatalf("rollout missed opponent target win: %d", score)
	}
}
