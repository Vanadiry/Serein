package server

import (
	"net/http"

	"github.com/vanadiry/serein/core/events"
)

func handleEvents(w http.ResponseWriter, r *http.Request) {
	ch := events.Subscribe()
	defer events.Unsubscribe(ch)
	serveSSE(w, r, ch)
}
