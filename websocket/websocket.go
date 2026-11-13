// Package websocket retains the optional browser-log socket helper.
// Game connections are owned by gameui and never use a process-global hub.
package websocket

import (
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
)

// WithWS registers a same-origin WebSocket endpoint.
func WithWS(path string, mux *http.ServeMux, handler func(*websocket.Conn)) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		return u.Host == r.Host && u.Scheme == scheme
	}}
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WS upgrade %s: %v", path, err)
			return
		}
		handler(conn)
	})
}
