// Package cardtable adapts private Trick Evolution views to generic 3D scenes.
package cardtable

import (
	"fmt"
	"math"
	"sort"

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
type Seat struct {
	ID            domain.SeatID `json:"id"`
	Name          string        `json:"name"`
	Count         int           `json:"count"`
	Score         int           `json:"score"`
	Tricks        int           `json:"tricks"`
	LabelPosition scene.Vec3    `json:"labelPosition"`
	Turn          bool          `json:"turn"`
	Self          bool          `json:"self"`
}
type Presentation struct {
	ID       string                `json:"id"`
	Kind     domain.EventKind      `json:"kind"`
	Revision uint64                `json:"revision"`
	Seat     domain.SeatID         `json:"seat"`
	Card     domain.CardID         `json:"card,omitempty"`
	Plays    []domain.Play         `json:"plays,omitempty"`
	Motion   animation.Description `json:"motion"`
}
type Payload struct {
	Version           int                `json:"version"`
	Scene             scene.Description  `json:"scene"`
	Selections        []Selection        `json:"selections"`
	Seats             []Seat             `json:"seats"`
	Presentations     []Presentation     `json:"presentations,omitempty"`
	PresentationScene *scene.Description `json:"presentationScene,omitempty"`
}

func cardID(id domain.CardID) string {
	card, _ := domain.CardDetails(id)
	return fmt.Sprintf("card_%s_%02d", card.Suit, card.Rank)
}
func seatPosition(index, total int) scene.Vec3 {
	// Seat zero is always the local near edge. Remaining seats run clockwise.
	switch total {
	case 2:
		if index == 0 {
			return scene.Vec3{0, 0, 3.85}
		}
		return scene.Vec3{0, 0, -3.55}
	case 3:
		return []scene.Vec3{{0, 0, 3.85}, {-1.8, 0, -2.9}, {1.8, 0, -2.9}}[index]
	default:
		return []scene.Vec3{{0, 0, 3.85}, {-1.85, 0, -.6}, {0, 0, -3.55}, {1.85, 0, -.6}}[index]
	}
}
func playPosition(index, total int) scene.Vec3 {
	p := seatPosition(index, total)
	return scene.Vec3{p[0] * .58, .045, p[2] * .38}
}
func cardNode(id string, card domain.StandardCard, position scene.Vec3, rotation scene.Vec3, pick string) scene.Node {
	ink := "#18251c"
	if card.Suit == domain.Hearts || card.Suit == domain.Diamonds {
		ink = "#af3030"
	}
	t := scene.Identity()
	t.Position = position
	t.Rotation = rotation
	return scene.Node{ID: id, Parent: "table", Kind: "box", Radius: .055, Size: scene.Vec3{.86, .035, 1.22}, Material: scene.Material{Color: "#fffdf4", Text: cardLabel(card), Ink: ink}, Transform: t, PickID: pick}
}
func landingMotion(id string, target scene.Transform) animation.Description {
	above := target
	above.Position[1] = 1.45
	above.Position[2] -= .3
	above.Rotation[0] = -.3
	bounce := target
	bounce.Position[1] = .13
	return animation.New(id,
		animation.Step{To: above, DurationMS: 170, Easing: "easeOutCubic"},
		animation.Step{To: target, DurationMS: 390, Easing: "easeInOutCubic"},
		animation.Step{To: bounce, DurationMS: 90, Easing: "easeOutCubic"},
		animation.Step{To: target, DurationMS: 130, Easing: "easeInOutCubic"})
}
func Build(view domain.PlayerView, catalog domain.Catalog, events ...domain.Event) (Payload, error) {
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
	surface := scene.Identity()
	surface.Position[1] = -.09
	nodes = append(nodes, scene.Node{ID: "surface", Parent: "table", Kind: "box", Radius: .46, Size: scene.Vec3{4.5, .16, 9.5}, Material: scene.Material{Color: "#225a43"}, Transform: surface})
	// The center inlay makes the valid release region legible even without hover.
	zone := scene.Identity()
	zone.Position[1] = .003
	nodes = append(nodes, scene.Node{ID: "dropzone", Parent: "table", Kind: "box", Radius: .24, Size: scene.Vec3{3.3, .008, 2.65}, Material: scene.Material{Color: "#326d50"}, Transform: zone})
	seats := make([]Seat, 0, len(view.Public.Seats))
	ownIndex := 0
	for i, s := range view.Public.Seats {
		if s.ID == view.Seat {
			ownIndex = i
			break
		}
	}
	for offset := 0; offset < len(view.Public.Seats); offset++ {
		s := view.Public.Seats[(ownIndex+offset)%len(view.Public.Seats)]
		pos := seatPosition(offset, len(view.Public.Seats))
		label := scene.Vec3{pos[0], .08, pos[2] - .75}
		if offset == 0 {
			label[2] = 4.95
		}
		seats = append(seats, Seat{ID: s.ID, Count: s.HandCount, Score: s.Score, Tricks: s.TricksWon,
			LabelPosition: label, Turn: view.Public.Phase == domain.PhasePlayingTrick && s.ID == view.Public.Turn, Self: s.ID == view.Seat})
		if offset == 0 {
			continue
		}
		marker := scene.Identity()
		marker.Position = scene.Vec3{pos[0], .025, pos[2]}
		color := "#456b5a"
		if s.ID == view.Public.Turn {
			color = "#eec976"
		}
		nodes = append(nodes, scene.Node{ID: fmt.Sprintf("seat_%d", offset), Parent: "table", Kind: "box", Radius: .09, Size: scene.Vec3{1.05, .04, .42}, Material: scene.Material{Color: color}, Transform: marker})
		for j := 0; j < s.HandCount && j < 13; j++ {
			back := scene.Identity()
			back.Position = scene.Vec3{pos[0] + (float64(j)-float64(s.HandCount-1)/2)*.22, .055, pos[2] - .15}
			nodes = append(nodes, scene.Node{ID: fmt.Sprintf("back_%d_%d", offset, j), Parent: "table", Kind: "box", Radius: .045, Size: scene.Vec3{.6, .025, .82}, Material: scene.Material{Color: "#e2c783"}, Transform: back})
		}
	}
	hand := make([]domain.StandardCard, 0, len(view.Hand))
	for _, id := range view.Hand {
		card, ok := domain.CardDetails(id)
		if !ok {
			return Payload{}, fmt.Errorf("unknown card %q", id)
		}
		hand = append(hand, card)
	}
	suitOrder := map[domain.Suit]int{domain.Hearts: 0, domain.Diamonds: 1, domain.Clubs: 2, domain.Spades: 3}
	sort.Slice(hand, func(i, j int) bool {
		if hand[i].Suit != hand[j].Suit {
			return suitOrder[hand[i].Suit] < suitOrder[hand[j].Suit]
		}
		return hand[i].Rank < hand[j].Rank
	})

	const pitch = .61
	const layerGap = .041 // Greater than card thickness so neighboring cards cannot intersect.
	normalY, normalZ := math.Cos(pitch), math.Sin(pitch)
	spread := .43
	if len(hand) > 5 {
		spread = 1.72 / float64(len(hand)-1)
	}
	selections := make([]Selection, 0, len(hand))
	for i, card := range hand {
		id := "hand_" + cardIDString(card)
		offset := float64(i) - float64(len(hand)-1)/2
		layer := float64(i) * layerGap
		// Parallel faces overlap from left to right. The slight downward step exposes
		// the upper-left corner even when the hand is tightly packed.
		node := cardNode(id, card, scene.Vec3{
			offset * spread,
			.49 + layer*normalY - offset*.025*normalZ,
			3.42 + layer*normalZ + offset*.025*normalY,
		}, scene.Vec3{pitch, 0, 0}, id)
		nodes = append(nodes, node)
		lift := node.Transform
		liftDistance := float64(len(hand)-i)*layerGap + .14
		lift.Position[1] += liftDistance*normalY + .12*normalZ
		lift.Position[2] += liftDistance*normalZ - .12*normalY
		motion := animation.New(id, animation.Step{To: lift, DurationMS: 160, Easing: "easeOutCubic"})
		if err := motion.Validate(); err != nil {
			return Payload{}, err
		}
		selections = append(selections, Selection{PickID: id, Card: card.ID, Legal: allowed[card.ID], Animation: motion})
	}
	for _, play := range view.Public.CurrentTrick {
		card, ok := domain.CardDetails(play.Card)
		if !ok {
			return Payload{}, fmt.Errorf("unknown played card %q", play.Card)
		}
		index := 0
		for i, s := range seats {
			if s.ID == play.Seat {
				index = i
				break
			}
		}
		nodes = append(nodes, cardNode(cardID(play.Card), card, playPosition(index, len(seats)), scene.Vec3{}, ""))
	}
	description := scene.New(view.Public.Revision, "#122b23", scene.Camera{Position: scene.Vec3{0, 6.4, 7.1}, Target: scene.Vec3{0, 0, 0}, FOV: 44}, nodes...)
	if err := description.Validate(); err != nil {
		return Payload{}, err
	}
	payload := Payload{Version: 1, Scene: description, Selections: selections, Seats: seats}
	for _, event := range events {
		if event.Kind != domain.EventCardPlayed && event.Kind != domain.EventTrickWon {
			continue
		}
		item := Presentation{ID: fmt.Sprintf("event_%d_%s", event.Revision, event.Kind), Kind: event.Kind, Revision: event.Revision, Seat: event.Seat, Card: event.Card, Plays: event.Plays}
		if event.Kind == domain.EventCardPlayed {
			index := 0
			for i, s := range seats {
				if s.ID == event.Seat {
					index = i
					break
				}
			}
			target := scene.Identity()
			target.Position = playPosition(index, len(seats))
			item.Motion = landingMotion(cardID(event.Card), target)
		} else if len(event.Plays) > 0 {
			staged := description
			staged.Nodes = append([]scene.Node(nil), description.Nodes...)
			for _, play := range event.Plays {
				card, ok := domain.CardDetails(play.Card)
				if !ok {
					return Payload{}, fmt.Errorf("unknown collected card %q", play.Card)
				}
				index := 0
				for i, s := range seats {
					if s.ID == play.Seat {
						index = i
						break
					}
				}
				staged.Nodes = append(staged.Nodes, cardNode(cardID(play.Card), card, playPosition(index, len(seats)), scene.Vec3{}, ""))
			}
			if err := staged.Validate(); err != nil {
				return Payload{}, err
			}
			payload.PresentationScene = &staged
			target := scene.Identity()
			target.Position = scene.Vec3{0, .9, 0}
			item.Motion = animation.New(cardID(event.Plays[0].Card), animation.Step{To: target, DurationMS: 300, Easing: "easeInOutCubic"})
		}
		if len(item.Motion.Steps) > 0 {
			if err := item.Motion.Validate(); err != nil {
				return Payload{}, err
			}
		}
		payload.Presentations = append(payload.Presentations, item)
	}
	return payload, nil
}
func cardIDString(card domain.StandardCard) string {
	return fmt.Sprintf("%s_%02d", card.Suit, card.Rank)
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
