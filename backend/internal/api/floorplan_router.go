package api

import (
	"net/http"
	"strings"
)

// floorPlanRouter despacha as rotas sob /api/floorplans/{id}.
//
// O net/http do Go casa "/api/floorplans/" por prefixo e não extrai variáveis
// de caminho, então o sufixo é resolvido aqui em vez de espalhar o parsing
// pelos handlers.
func floorPlanRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/floorplans/")

	switch {
	case strings.HasSuffix(path, "/image"):
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		floorPlanImageHandler(w, r)

	case strings.HasSuffix(path, "/pins"):
		if r.Method != http.MethodPut {
			methodNotAllowed(w, http.MethodPut)
			return
		}
		floorPlanPinsHandler(w, r)

	default:
		if r.Method != http.MethodGet && r.Method != http.MethodDelete {
			methodNotAllowed(w, http.MethodGet, http.MethodDelete)
			return
		}
		floorPlanHandler(w, r)
	}
}

func methodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
