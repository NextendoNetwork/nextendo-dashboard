package main

// Kick a player from a game server, from the dashboard.
//
// Every game server already exposes /api/kick; what was missing was any way to use it. Until
// now it meant a hand-written curl, with the right game's token, against the right internal
// URL — which in practice meant moderation did not happen. This aggregator already knows both,
// so it relays.
//
// SECURITY: the PID is the only key accepted. The displayed name comes from the CLIENT and can
// be someone else's — players have been observed presenting themselves under another player's
// name, blank, or as "Player". Kicking on the strength of a name would hit the victim of an
// impersonation rather than its author. The PID is authenticated; the name is not.
//
// Like /api/power this is POST-only and sits behind the same view-token gate as the rest of the
// site. POST matters here: a GET would be prefetchable and forgeable from a foreign page.

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// kickTimeout bounds the call to the game server. A moderator is watching a button, so a slow
// server must produce an answer rather than a spinner that never resolves.
const kickTimeout = 6 * time.Second

func kickHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	pid, err := strconv.ParseUint(r.URL.Query().Get("pid"), 10, 64)
	if err != nil || pid == 0 {
		http.Error(w, `{"error":"pid manquant ou invalide"}`, http.StatusBadRequest)
		return
	}

	// The game key is matched against the configured sources — never used to build a URL
	// directly, so no caller can point this at a host of their choosing.
	gameKey := r.URL.Query().Get("game")
	var src *gameSrc
	all := sources()
	for i := range all {
		if all[i].Key == gameKey {
			src = &all[i]
			break
		}
	}
	if src == nil {
		http.Error(w, `{"error":"jeu inconnu"}`, http.StatusBadRequest)
		return
	}

	u := fmt.Sprintf("%s/api/kick?pid=%d&key=%s", src.URL, pid, url.QueryEscape(src.Token))
	ctx, cancel := context.WithTimeout(r.Context(), kickTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Say that it failed, and why. A button that answers nothing lets a moderator believe
		// the player is gone while they are still racing.
		log.Printf("[kick] %s pid=%d -> %v", src.Key, pid, err)
		http.Error(w, `{"error":"serveur de jeu injoignable"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	// Kicks are logged server-side: a moderation action nobody can retrace is not moderation.
	log.Printf("[kick] %s pid=%d -> %d %s", src.Key, pid, resp.StatusCode, strings.TrimSpace(string(body)))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}
