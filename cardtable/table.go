// Package cardtable adapts private Trick Evolution views to generic 3D scenes.
package cardtable

import (
	"fmt"

	"github.com/n0remac/trick-evolution/animation"
	"github.com/n0remac/trick-evolution/domain"
	"github.com/n0remac/trick-evolution/scene"
)

type Selection struct {
	PickID    string                `json:"pickId"`
	Card      domain.CardID         `json:"card"`
	Legal     bool                  `json:"legal"`
	Animation animation.Description `json:"animation"`
}

type Payload struct {
	Version    int               `json:"version"`
	Scene      scene.Description `json:"scene"`
	Selections []Selection       `json:"selections"`
}

func Build(view domain.PlayerView, catalog domain.Catalog) (Payload, error) {
	var legal []domain.CardID
	if view.Public.Phase == domain.PhasePlayingTrick && view.Seat == view.Public.Turn {
		var err error
		legal, err = domain.LegalCards(view, catalog)
		if err != nil {
			return Payload{}, err
		}
	}
	allowed := make(map[domain.CardID]bool, len(legal))
	for _, card := range legal {
		allowed[card] = true
	}
	nodes := []scene.Node{{ID: "table", Kind: "group", Transform: scene.Identity()}}
	selections := make([]Selection, 0, len(view.Hand))
	for i, cardID := range view.Hand {
		card, ok := domain.CardDetails(cardID)
		if !ok {
			return Payload{}, fmt.Errorf("unknown card %q", cardID)
		}
		id := fmt.Sprintf("hand_%s_%02d", card.Suit, card.Rank)
		x := (float64(i) - float64(len(view.Hand)-1)/2) * 1.23
		base := scene.Identity()
		base.Position = scene.Vec3{x, -0.78, 0}
		color, ink := "#fffdf4", "#18251c"
		if card.Suit == domain.Hearts || card.Suit == domain.Diamonds {
			ink = "#af3030"
		}
		nodes = append(nodes, scene.Node{ID: id, Parent: "table", Kind: "box", Size: scene.Vec3{1.02, 1.42, 0.045},
			Material: scene.Material{Color: color, Text: cardLabel(card), Ink: ink}, Transform: base, PickID: id})
		lift := base
		lift.Position[1] += 0.42
		lift.Position[2] += 0.24
		move := lift
		move.Position[1] += 0.35
		move.Position[2] += 0.12
		motion := animation.New(id,
			animation.Step{To: lift, DurationMS: 180, Easing: "easeOutCubic"},
			animation.Step{To: move, DurationMS: 220, Easing: "easeInOutCubic"},
			animation.Step{To: lift, DurationMS: 240, Easing: "easeInOutCubic"})
		if err := motion.Validate(); err != nil {
			return Payload{}, err
		}
		selections = append(selections, Selection{PickID: id, Card: cardID, Legal: allowed[cardID], Animation: motion})
	}
	description := scene.New(view.Public.Revision, "#184332", scene.Camera{Position: scene.Vec3{0, 0, 6.5}, Target: scene.Vec3{0, 0, 0}, FOV: 45}, nodes...)
	if err := description.Validate(); err != nil {
		return Payload{}, err
	}
	return Payload{Version: 1, Scene: description, Selections: selections}, nil
}

func cardLabel(card domain.StandardCard) string {
	rank := fmt.Sprint(card.Rank)
	switch card.Rank {
	case 11:
		rank = "J"
	case 12:
		rank = "Q"
	case 13:
		rank = "K"
	case 14:
		rank = "A"
	}
	suit := map[domain.Suit]string{domain.Hearts: "♥", domain.Diamonds: "♦", domain.Clubs: "♣", domain.Spades: "♠"}[card.Suit]
	return rank + suit
}
