package main

import "testing"

func TestAssertHandFollowsEvolvingField(t *testing.T) {
	r := &runner{}
	state := func(hand int, rules ...string) uiState {
		return uiState{Rules: rules, Scores: []scoreLine{{Name: "Alice", Points: hand * 5, Tricks: hand * 5}, {Name: "Bob"}}}
	}
	if err := r.assertHand(1, state(1, "Follow Suit", "High Card Wins"), state(1, "Follow Suit", "High Card Wins")); err != nil {
		t.Fatal(err)
	}
	if err := r.assertHand(3, state(3, "Follow Suit", "High Card Wins", "Spades Are Trump"), state(3, "Follow Suit", "High Card Wins", "Spades Are Trump")); err != nil {
		t.Fatal(err)
	}
	if err := r.assertHand(3, state(3, "Follow Suit", "High Card Wins"), state(3, "Follow Suit", "High Card Wins")); err == nil {
		t.Fatal("reveal boundary accepted unchanged two-card field")
	}
	if err := r.assertHand(3, state(3, "Follow Suit", "High Card Wins", "Spades Are Trump"), state(3, "High Card Wins", "Follow Suit", "Spades Are Trump")); err == nil {
		t.Fatal("seats with different field order accepted")
	}
}
