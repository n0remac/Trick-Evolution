package browser3d

import (
	_ "embed"
	"net/http"
)

//go:embed runtime.js
var runtime []byte

//go:embed vendor/three-0.160.1.module.js
var three []byte

func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /game/browser3d/runtime.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(runtime)
	})
	mux.HandleFunc("GET /game/vendor/three-0.160.1.module.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(three)
	})
}
