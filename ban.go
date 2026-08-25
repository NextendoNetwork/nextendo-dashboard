package main

// Ban a player's account from the dashboard.
//
// SECURITY — read before touching this. A ban is not a kick. banAccount() on the account server
// DELETES the account and leaves a tombstone: the e-mail and IP can never register again, the
// friend code is never reissued, the PID stays marked, and the cloud saves and play history are
// erased. There is no undo that brings the saves back.
//
// The dashboard is gated by DASH_VIEW_TOKEN, which travels in the URL — so it ends up in browser
// history, screenshots and shared links. That is an acceptable blast radius for looking at
// stats, and for kicking (a kick is reversible: the player reconnects). It is NOT an acceptable
// blast radius for deleting accounts.
//
// So banning takes a SECOND secret that this service never stores: the account server's admin
// key, typed by the operator at the moment of the ban and forwarded as X-Admin-Key. Leaking the
// dashboard link therefore does not hand anyone the power to erase accounts. We do not read the
// key from the environment on purpose — putting it here would defeat the whole arrangement.
//
// The PID is the only identifier accepted, for the same reason as kick.go: the displayed name
// comes from the client and can be someone else's.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	banTimeout = 8 * time.Second
	// banReasonMax bounds what we forward. The reason is stored and shown; an unbounded one
	// would be a way to vandalise the ban record itself.
	banReasonMax = 200
)

func banHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	adminKey := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
	if adminKey == "" {
		http.Error(w, `{"error":"clé d administration requise"}`, http.StatusUnauthorized)
		return
	}
	pid, err := strconv.ParseUint(r.URL.Query().Get("pid"), 10, 64)
	if err != nil || pid == 0 {
		http.Error(w, `{"error":"pid manquant ou invalide"}`, http.StatusBadRequest)
		return
	}
	// A reason is mandatory. A ban nobody can explain later is a ban nobody can defend.
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	if reason == "" {
		http.Error(w, `{"error":"motif requis"}`, http.StatusBadRequest)
		return
	}
	if len(reason) > banReasonMax {
		reason = reason[:banReasonMax]
	}

	payload, _ := json.Marshal(map[string]any{"pid": pid, "reason": reason})
	ctx, cancel := context.WithTimeout(r.Context(), banTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		accountURL()+"/api/admin/ban", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Key", adminKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[ban] pid=%d -> %v", pid, err)
		http.Error(w, `{"error":"serveur de comptes injoignable"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	// Logged with the reason, never the key. An irreversible action that leaves no trace of who
	// asked for it and why is not moderation.
	log.Printf("[ban] pid=%d reason=%q -> %d %s", pid, reason, resp.StatusCode, strings.TrimSpace(string(body)))

	// The player keeps playing until their connection drops, so close it too. Best-effort: the
	// ban is what matters and it already went through.
	if resp.StatusCode == http.StatusOK {
		go kickEverywhere(pid)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// kickEverywhere closes this PID's connections on every game server. A banned account that stays
// connected until it happens to disconnect would look, to everyone in the lobby, like nothing
// happened at all.
func kickEverywhere(pid uint64) {
	for _, src := range sources() {
		u := fmt.Sprintf("%s/api/kick?pid=%d&key=%s", src.URL, pid, url.QueryEscape(src.Token))
		ctx, cancel := context.WithTimeout(context.Background(), kickTimeout)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
		}
		cancel()
	}
}
