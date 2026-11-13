package gameui

import (
	_ "embed"
	"net/http"
)

//go:embed game.js
var gameJS []byte

//go:embed style.css
var gameCSS []byte

//go:embed vendor/htmx-2.0.11.min.js
var htmxJS []byte

//go:embed vendor/htmx-ext-ws-2.0.4.min.js
var htmxWSJS []byte

func serveGameJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write(gameJS)
}

func serveGameCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write(gameCSS)
}

func serveHTMX(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(htmxJS)
}

func serveHTMXWS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(htmxWSJS)
}
