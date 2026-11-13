package gameui

import (
	_ "embed"
	"net/http"
)

//go:embed index.html
var gamePage []byte

//go:embed game.js
var gameJS []byte

//go:embed style.css
var gameCSS []byte

func serveGamePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(gamePage)
}

func serveGameJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write(gameJS)
}

func serveGameCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write(gameCSS)
}
