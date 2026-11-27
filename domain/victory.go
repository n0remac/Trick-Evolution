package domain

import (
	"fmt"
	"strings"
)

func targetForRules(r RuleResolution) int {
	e, _ := r.Effect(MechanicVictory)
	switch e.Value {
	case ValueFirstTo5:
		return 5
	case ValueFirstTo21:
		return 21
	}
	return 0
}

func cloneSeatScores(scores map[SeatID]int) map[SeatID]int {
	if scores == nil {
		return nil
	}
	out := make(map[SeatID]int, len(scores))
	for id, score := range scores {
		out[id] = score
	}
	return out
}

func cloneTargetResult(result *TargetGameResult) *TargetGameResult {
	if result == nil {
		return nil
	}
	out := *result
	out.Scores = cloneSeatScores(result.Scores)
	out.TeamScores = make(map[int]int, len(result.TeamScores))
	for team, score := range result.TeamScores {
		out.TeamScores[team] = score
	}
	out.Winners = append([]SeatID(nil), result.Winners...)
	return &out
}

// A game interval starts at a deal boundary. Historical personal and run
// ledgers are never rewritten when ownership or victory rules change.
func syncGameContext(s *MatchState, r RuleResolution) {
	owner, _ := r.Effect(MechanicScoreOwnership)
	victory, _ := r.Effect(MechanicVictory)
	_, bidOnly := r.Effect(MechanicWinOnBid)
	ids := make([]string, len(s.Seats))
	for i, seat := range s.Seats {
		ids[i] = string(seat.ID)
	}
	key := fmt.Sprintf("%s/%s/%t/%s", owner.Value, victory.Value, bidOnly == nil, strings.Join(ids, ","))
	if s.GameRuleKey != key {
		wasTarget := strings.Contains(s.GameRuleKey, string(ValueFirstTo5)) || strings.Contains(s.GameRuleKey, string(ValueFirstTo21))
		if s.GameRuleKey != "" {
			s.GameNumber++
		}
		s.GameRuleKey = key
		s.GameScores = make(map[SeatID]int, len(s.Seats))
		for _, seat := range s.Seats {
			s.GameScores[seat.ID] = 0
		}
		s.GameTeamScores = nil
		if owner.Value == ValuePartnership {
			s.GameTeamScores = map[int]int{0: 0, 1: 0}
		}
		if s.Config.Mode == ModeEndless {
			if targetForRules(r) != 0 {
				s.ShortGameResult = nil
				s.ShortGameRound, s.ShortGameScores = 0, nil
			} else if wasTarget {
				s.ShortGameNumber++
				s.ShortGameRound = 1
				s.ShortGameScores = make(map[SeatID]int, len(s.Seats))
				s.ShortGameResult = nil
			}
		}
	} else if s.GameResult != nil && s.GameResult.Number == s.GameNumber {
		s.GameNumber++
		for id := range s.GameScores {
			s.GameScores[id] = 0
		}
		for team := range s.GameTeamScores {
			s.GameTeamScores[team] = 0
		}
	}
	known := RecognizeKnownGame(*s, r)
	s.RecognitionNotice = ""
	if known != "" && known != s.KnownGame {
		s.RecognitionNotice = "This field plays as " + known + "."
	}
	s.KnownGame = known
}

// settleTargetGame counts the final net hand awards, including a bidder's
// replacement penalty, once. A target win is checked only after settlement.
func settleTargetGame(s *MatchState, r RuleResolution) *TargetGameResult {
	target := targetForRules(r)
	if target == 0 {
		return nil
	}
	if s.GameScores == nil {
		s.GameScores = make(map[SeatID]int, len(s.Seats))
	}
	if len(s.HandTeams) == 4 && s.GameTeamScores == nil {
		s.GameTeamScores = map[int]int{0: 0, 1: 0}
	}
	for _, a := range s.HandAwards {
		s.GameScores[a.Seat] += a.Points
		if team, ok := s.HandTeams[a.Seat]; ok {
			s.GameTeamScores[team] += a.Points
		}
	}
	best := target - 1
	var leaders []SeatID
	for _, seat := range s.Seats {
		score := s.GameScores[seat.ID]
		if len(s.HandTeams) == 4 {
			score = s.GameTeamScores[s.HandTeams[seat.ID]]
		}
		if score > best {
			best, leaders = score, nil
		}
		if score == best && score >= target {
			leaders = append(leaders, seat.ID)
		}
	}
	if len(leaders) == 0 {
		return nil
	}
	if len(s.HandTeams) == 4 && len(leaders) > 2 {
		// Both teams tied. All four seats share a win unless bid restricted.
	}
	if _, err := r.Effect(MechanicWinOnBid); err == nil {
		if s.Auction == nil || s.Auction.Contract == nil || !s.Auction.Contract.Made {
			return nil
		}
		contract := s.Auction.Contract
		eligible := false
		for _, id := range leaders {
			if id == contract.Bidder || contract.Team >= 0 && s.HandTeams[id] == contract.Team {
				eligible = true
			}
		}
		if !eligible {
			return nil
		}
		leaders = nil
		for _, seat := range s.Seats {
			if seat.ID == contract.Bidder || contract.Team >= 0 && s.HandTeams[seat.ID] == contract.Team {
				leaders = append(leaders, seat.ID)
			}
		}
	}
	result := &TargetGameResult{Number: s.GameNumber, Target: target, Hand: s.HandNumber,
		Scores: make(map[SeatID]int, len(s.GameScores)), TeamScores: make(map[int]int, len(s.GameTeamScores)), Winners: leaders}
	for id, score := range s.GameScores {
		result.Scores[id] = score
	}
	for team, score := range s.GameTeamScores {
		result.TeamScores[team] = score
	}
	s.GameResult = result
	return result
}
