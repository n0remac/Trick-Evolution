package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/cards"
	"github.com/n0remac/trick-evolution/gameui"
	gamews "github.com/n0remac/trick-evolution/websocket"
)

func main() {
	if debugEnabled {
		log.Println("🔍 WEBRTC_DEBUG is ON – logging endpoint active")
	} else {
		log.Println("WEBRTC_DEBUG is OFF – logging endpoint will refuse connections")
	}

	// Create a new HTTP server
	mux := http.NewServeMux()
	gameHandler := gameui.NewServer().Handler()
	mux.Handle("/game", gameHandler)
	mux.Handle("/game/", gameHandler)
	mux.Handle("/ws/game", gameHandler)
	cards.Trick(mux)

	// Redirect root to cards lobby
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/game", http.StatusFound)
	})

	gamews.WithWS("/ws/logs", mux, logSocketWS)

	address := os.Getenv("TRICK_EVOLUTION_ADDR")
	if address == "" {
		address = ":8080"
	}
	log.Println("Card game server listening on", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

// debugEnabled toggles logging of incoming browser messages
var debugEnabled = func() bool {
	v := strings.ToLower(os.Getenv("WEBRTC_DEBUG"))
	return v == "1" || v == "true" || v == "yes"
}()

// logSocketWS streams browser logs to both file and stdout
func logSocketWS(conn *websocket.Conn) {
	if !debugEnabled {
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "logging disabled"))
		conn.Close()
		return
	}
	defer conn.Close()

	// Ensure log directory
	if err := os.MkdirAll("serverlogs", 0755); err != nil {
		log.Printf("mkdir serverlogs error: %v", err)
		return
	}

	// Open append‐only daily log
	fileName := fmt.Sprintf("serverlogs/%s.webrtc.log", time.Now().Format("2006-01-02"))
	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("open log file error: %v", err)
		return
	}
	defer f.Close()

	log.Printf("📝 log‐socket connected → %s", fileName)

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Printf("log‐socket closed: %v", err)
			return
		}
		// write and mirror
		f.Write(append(raw, '\n'))
		log.Printf("[browser] %s", raw)
	}
}
