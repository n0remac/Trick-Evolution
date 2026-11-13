package gameui_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/domain"
	"github.com/n0remac/trick-evolution/gameui"
)

type matchSession struct {
	Room  string        `json:"room"`
	Seat  domain.SeatID `json:"seat"`
	Token string        `json:"token"`
}

type matchMessage struct {
	Type     string             `json:"type"`
	ID       string             `json:"id"`
	Code     string             `json:"code"`
	Revision uint64             `json:"revision"`
	View     *domain.PlayerView `json:"view"`
	Events   []domain.Event     `json:"events"`
	Raw      []byte             `json:"-"`
}

type matchPeer struct {
	session matchSession
	conn    *websocket.Conn
	view    domain.PlayerView
}

type matchFixture struct {
	t        *testing.T
	base     string
	peers    []matchPeer
	revision uint64
	serial   int
	played   map[domain.CardID]bool
	counts   map[domain.EventKind]int
}

var matchHTTPClient = &http.Client{Timeout: 5 * time.Second}

func newMatchFixture(t *testing.T, players int) *matchFixture {
	t.Helper()
	var deal atomic.Int64
	server, err := gameui.NewServerWithConfig(domain.DefaultConfig(), func() domain.ShuffleSource {
		return domain.SeededShuffle(5100 + deal.Add(1))
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(server.Close)
	t.Cleanup(httpServer.Close)
	f := &matchFixture{t: t, base: httpServer.URL, peers: make([]matchPeer, players), played: map[domain.CardID]bool{}, counts: map[domain.EventKind]int{}}
	for i := range f.peers {
		f.peers[i].session = f.join("complete", fmt.Sprintf("P%d", i), "")
		f.peers[i].conn = f.dial(f.peers[i].session)
		_ = f.read(i, func(m matchMessage) bool { return m.Type == "lobby" })
	}
	return f
}

func (f *matchFixture) join(room, name, token string) matchSession {
	f.t.Helper()
	data, _ := json.Marshal(map[string]string{"room": room, "name": name, "token": token})
	request, err := http.NewRequest(http.MethodPost, f.base+"/game/join", bytes.NewReader(data))
	if err != nil {
		f.t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := matchHTTPClient.Do(request)
	if err != nil {
		f.t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		f.t.Fatalf("join %s: HTTP %d", room, response.StatusCode)
	}
	var session matchSession
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		f.t.Fatal(err)
	}
	if session.Seat == "" || session.Token == "" {
		f.t.Fatal("join omitted seat or token")
	}
	return session
}

func (f *matchFixture) dial(session matchSession) *websocket.Conn {
	f.t.Helper()
	u, _ := url.Parse(f.base)
	address := "ws://" + u.Host + "/ws/game?room=" + url.QueryEscape(session.Room) + "&token=" + url.QueryEscape(session.Token)
	conn, response, err := websocket.DefaultDialer.Dial(address, http.Header{"Origin": []string{f.base}})
	if err != nil {
		f.t.Fatalf("websocket dial: %v (%v)", err, response)
	}
	f.t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func (f *matchFixture) read(peer int, wanted func(matchMessage) bool) matchMessage {
	f.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for attempt := 0; attempt < 24; attempt++ {
		_ = f.peers[peer].conn.SetReadDeadline(deadline)
		_, raw, err := f.peers[peer].conn.ReadMessage()
		if err != nil {
			f.t.Fatalf("seat %d read: %v", peer, err)
		}
		var message matchMessage
		if err := json.Unmarshal(raw, &message); err != nil {
			f.t.Fatalf("seat %d malformed server JSON: %v", peer, err)
		}
		message.Raw = raw
		if wanted(message) {
			return message
		}
	}
	f.t.Fatalf("seat %d did not receive requested message", peer)
	return matchMessage{}
}

func (f *matchFixture) write(peer int, value any) {
	f.t.Helper()
	if err := f.peers[peer].conn.WriteJSON(value); err != nil {
		f.t.Fatalf("seat %d write: %v", peer, err)
	}
}

func (f *matchFixture) apply(peer int, kind string, card domain.CardID) (string, []domain.Event) {
	f.t.Helper()
	f.serial++
	id := fmt.Sprintf("action-%03d", f.serial)
	command := map[string]any{"type": kind, "id": id}
	if card != "" {
		command["card"] = card
	}
	f.write(peer, command)
	ack := f.read(peer, func(m matchMessage) bool { return m.Type == "ack" && m.ID == id })
	if ack.Revision != f.revision+1 {
		f.t.Fatalf("%s: revision %d, expected %d", id, ack.Revision, f.revision+1)
	}
	f.revision = ack.Revision
	var snapshots []matchMessage
	for i := range f.peers {
		snapshot := f.read(i, func(m matchMessage) bool { return m.Type == "state" && m.Revision == f.revision })
		if snapshot.View == nil {
			f.t.Fatalf("seat %d missing view", i)
		}
		f.peers[i].view = *snapshot.View
		snapshots = append(snapshots, snapshot)
	}
	f.assertSnapshots(snapshots)
	events := snapshots[0].Events
	for _, event := range events {
		if event.Revision != f.revision {
			f.t.Fatalf("event %s has revision %d, expected %d", event.Kind, event.Revision, f.revision)
		}
		f.counts[event.Kind]++
	}
	if kind == "play" {
		if len(events) == 0 || events[0].Kind != domain.EventCardPlayed || events[0].Card != card || events[0].Seat != f.peers[peer].session.Seat {
			f.t.Fatalf("%s: wrong public play event: %+v", id, events)
		}
		if f.played[card] {
			f.t.Fatalf("card %s played twice in one hand", card)
		}
		f.played[card] = true
		f.assertConservation()
	}
	return id, events
}

func (f *matchFixture) assertSnapshots(messages []matchMessage) {
	f.t.Helper()
	if len(messages) != len(f.peers) {
		f.t.Fatal("missing seat snapshot")
	}
	public := messages[0].View.Public
	for i, message := range messages {
		if message.View.Seat != f.peers[i].session.Seat || !reflect.DeepEqual(message.View.Public, public) || !reflect.DeepEqual(message.Events, messages[0].Events) {
			f.t.Fatalf("seat %d received wrong public or private projection", i)
		}
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(message.Raw, &envelope); err != nil {
			f.t.Fatal(err)
		}
		var rawView map[string]json.RawMessage
		if err := json.Unmarshal(envelope["view"], &rawView); err != nil {
			f.t.Fatal(err)
		}
		publicJSON := rawView["Public"]
		if len(publicJSON) == 0 {
			f.t.Fatal("raw public view is missing")
		}
		for j, other := range messages {
			for _, card := range other.View.Hand {
				quoted := []byte(`"` + string(card) + `"`)
				if bytes.Contains(publicJSON, quoted) {
					f.t.Fatalf("unplayed card %s leaked in public state", card)
				}
				if i != j && bytes.Contains(message.Raw, quoted) {
					f.t.Fatalf("seat %d saw seat %d's unplayed card %s", i, j, card)
				}
			}
		}
	}
}

func (f *matchFixture) assertConservation() {
	f.t.Helper()
	public := f.peers[0].view.Public
	seen := make(map[domain.CardID]bool)
	for card := range f.played {
		seen[card] = true
	}
	handCount, capturedCount := 0, 0
	for _, peer := range f.peers {
		handCount += len(peer.view.Hand)
		for _, card := range peer.view.Hand {
			if seen[card] {
				f.t.Fatalf("card %s appears twice in hand or plays", card)
			}
			seen[card] = true
		}
	}
	for _, seat := range public.Seats {
		capturedCount += seat.CapturedCount
	}
	dealt := 5 * len(f.peers)
	if len(seen) != dealt || len(f.played)+handCount != dealt || capturedCount+len(public.CurrentTrick)+handCount != dealt {
		f.t.Fatalf("hand conservation: seen=%d played=%d hand=%d captured=%d trick=%d dealt=%d", len(seen), len(f.played), handCount, capturedCount, len(public.CurrentTrick), dealt)
	}
}

func (f *matchFixture) legalCard() (int, domain.CardID) {
	f.t.Helper()
	public := f.peers[0].view.Public
	peer := -1
	for i, candidate := range f.peers {
		if candidate.session.Seat == public.Turn {
			peer = i
			break
		}
	}
	if peer < 0 {
		f.t.Fatalf("unknown turn seat %s", public.Turn)
	}
	hand := f.peers[peer].view.Hand
	if len(hand) == 0 {
		f.t.Fatalf("turn seat %d has no cards", peer)
	}
	follow := false
	for _, rule := range public.Rules {
		if rule.Active && rule.Instance.DefinitionID == domain.RuleFollowSuit {
			follow = true
		}
	}
	if follow && len(public.CurrentTrick) != 0 {
		lead := strings.Split(string(public.CurrentTrick[0].Card), ":")[1]
		for _, card := range hand {
			if strings.Split(string(card), ":")[1] == lead {
				return peer, card
			}
		}
	}
	return peer, hand[0]
}

func (f *matchFixture) reconnect(peer int) {
	f.t.Helper()
	before := make([]domain.CardID, len(f.peers[peer].view.Hand))
	copy(before, f.peers[peer].view.Hand)
	_ = f.peers[peer].conn.Close()
	session := f.join("complete", fmt.Sprintf("Changed%d", peer), f.peers[peer].session.Token)
	if session.Seat != f.peers[peer].session.Seat {
		f.t.Fatal("reconnect changed seat")
	}
	f.peers[peer].conn = f.dial(session)
	snapshot := f.read(peer, func(m matchMessage) bool { return m.Type == "state" && m.Revision == f.revision })
	if snapshot.View == nil || !reflect.DeepEqual(snapshot.View.Hand, before) || !reflect.DeepEqual(snapshot.View.Public, f.peers[0].view.Public) {
		f.t.Fatalf("seat %d reconnect did not restore state", peer)
	}
	f.peers[peer].view = *snapshot.View
}

func TestCompleteGameBlackBox(t *testing.T) {
	for _, players := range []int{2, 4} {
		t.Run(fmt.Sprintf("%d_players", players), func(t *testing.T) {
			f := newMatchFixture(t, players)
			f.apply(0, "start", "")
			if f.peers[0].view.Public.HandNumber != 1 {
				t.Fatal("first hand did not start")
			}
			if players == 2 {
				foreign := f.join("foreign", "Foreign", "")
				u, _ := url.Parse(f.base)
				_, response, err := websocket.DefaultDialer.Dial("ws://"+u.Host+"/ws/game?room=complete&token="+foreign.Token, nil)
				if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
					t.Fatalf("foreign token entered room: %v, %v", err, response)
				}
				f.write(1, map[string]any{"type": "play", "id": "forged-room", "card": f.peers[1].view.Hand[0], "room": "foreign", "playerId": f.peers[0].session.Seat})
				bad := f.read(1, func(m matchMessage) bool { return m.Type == "error" && m.ID == "" })
				if bad.Code != "bad_command" {
					t.Fatalf("forged command accepted: %+v", bad)
				}
			}
			var revealHands []int
			var finalEvents []domain.Event
			for hand := 1; hand <= 10; hand++ {
				f.played = map[domain.CardID]bool{}
				for play := 0; play < 5*players; play++ {
					peer, card := f.legalCard()
					id, events := f.apply(peer, "play", card)
					if hand == 10 && play == 5*players-1 {
						finalEvents = events
					}
					if hand == 1 && play == 0 {
						f.write(peer, map[string]any{"type": "play", "id": id, "card": card})
						duplicate := f.read(peer, func(m matchMessage) bool { return m.Type == "ack" && m.ID == id })
						if duplicate.Revision != f.revision {
							t.Fatalf("duplicate action advanced revision to %d", duplicate.Revision)
						}
					}
					for i, event := range events {
						if event.Kind == domain.EventRuleRevealed {
							revealHands = append(revealHands, event.Hand)
							if i == 0 || events[i-1].Kind != domain.EventHandCompleted {
								t.Fatalf("rule reveal before hand completion: %+v", events)
							}
						}
					}
				}
				public := f.peers[0].view.Public
				wantPhase := domain.PhaseHandComplete
				if hand == 10 {
					wantPhase = domain.PhaseGameComplete
				}
				if public.Phase != wantPhase || public.CompletedHands != hand || len(public.Rules) != 1+hand/3 {
					t.Fatalf("hand %d boundary: phase=%s completed=%d rules=%d", hand, public.Phase, public.CompletedHands, len(public.Rules))
				}
				score, tricks, captured := 0, 0, 0
				for _, seat := range public.Seats {
					score += seat.Score
					tricks += seat.TricksWon
					captured += seat.CapturedCount
				}
				if score != 5*hand || tricks != 5*hand || captured != 5*players {
					t.Fatalf("hand %d totals: score=%d tricks=%d captured=%d", hand, score, tricks, captured)
				}
				if hand < 10 {
					f.apply(0, "next", "")
					f.played = map[domain.CardID]bool{}
					if hand == 4 && players == 2 {
						f.reconnect(1)
					}
				}
			}
			if !reflect.DeepEqual(revealHands, []int{3, 6, 9}) {
				t.Fatalf("reveal hands: %v", revealHands)
			}
			wantRevision := uint64(1 + 10*5*players + 9)
			if f.revision != wantRevision {
				t.Fatalf("final revision %d, want %d", f.revision, wantRevision)
			}
			for kind, count := range map[domain.EventKind]int{
				domain.EventHandStarted: 10, domain.EventCardPlayed: 50 * players, domain.EventTrickWon: 50,
				domain.EventHandCompleted: 10, domain.EventRuleRevealed: 3, domain.EventGameCompleted: 1,
			} {
				if f.counts[kind] != count {
					t.Fatalf("event %s count %d, want %d", kind, f.counts[kind], count)
				}
			}
			final := f.peers[0].view.Public
			var scores []int
			best := -1
			for _, seat := range final.Seats {
				scores = append(scores, seat.Score)
				if seat.Score > best {
					best = seat.Score
				}
			}
			var winners []domain.SeatID
			for _, seat := range final.Seats {
				if seat.Score == best {
					winners = append(winners, seat.ID)
				}
			}
			if !reflect.DeepEqual(final.Winners, winners) {
				t.Fatalf("winner projection %v, expected %v", final.Winners, winners)
			}
			wantScores := map[int][]int{2: {28, 22}, 4: {10, 13, 16, 11}}[players]
			if !reflect.DeepEqual(scores, wantScores) {
				t.Fatalf("final scores %v, expected %v", scores, wantScores)
			}
			if len(finalEvents) == 0 || finalEvents[len(finalEvents)-1].Kind != domain.EventGameCompleted || !reflect.DeepEqual(finalEvents[len(finalEvents)-1].Winners, winners) {
				t.Fatalf("final game event disagrees with winner projection: %+v", finalEvents)
			}
			if players == 2 {
				f.reconnect(1)
			}
			for _, kind := range []string{"play", "next"} {
				f.serial++
				id := fmt.Sprintf("post-%d", f.serial)
				f.write(0, map[string]any{"type": kind, "id": id, "card": "std:H:02"})
				rejected := f.read(0, func(m matchMessage) bool { return m.Type == "error" && m.ID == id })
				if rejected.Revision != f.revision || rejected.Code == "" {
					t.Fatalf("post-game %s was accepted: %+v", kind, rejected)
				}
			}
		})
	}
}
