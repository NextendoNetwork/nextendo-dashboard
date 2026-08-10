package main

// Manual start/stop of game servers from the dashboard.
//
// SECURITY: this talks to the local Docker Engine API over /var/run/docker.sock (mounted into
// this container). The socket is powerful, so every action here is constrained to a STRICT
// ALLOWLIST (powerContainers) and to two verbs (start/stop) — never an arbitrary container or
// command. The HTTP handler is behind the same DASH_VIEW_TOKEN gate as the rest of the site and
// is POST-only. MK8 is deliberately NOT in the list: it runs as a systemd service on another
// host (production) and is not controlled from here.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// powerContainers maps a dashboard game key -> the Docker container that runs it. ONLY these
// can be started/stopped from the dashboard.
var powerContainers = map[string]string{
	"s2":   "localhost",
	"ssbu": "localhost",
	"acnh": "localhost",
	"mc":   "minecraft",
}

// dockerHTTP speaks the Docker Engine API over the unix socket. host in the URL is ignored.
var dockerHTTP = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/docker.sock")
		},
	},
}

// containerRunning reports the container's running state. ok=false when Docker is unreachable
// (socket not mounted) so callers can fall back to the poll-based online flag.
func containerRunning(name string) (running, ok bool) {
	resp, err := dockerHTTP.Get("http://docker/containers/" + name + "/json")
	if err != nil {
		return false, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, false
	}
	var body struct {
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return false, false
	}
	return body.State.Running, true
}

// containerPower starts or stops the named container.
func containerPower(name, action string) error {
	resp, err := dockerHTTP.Post("http://docker/containers/"+name+"/"+action, "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 204 = changed, 304 = already in that state — both fine.
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
		return fmt.Errorf("docker %s %s: HTTP %d", action, name, resp.StatusCode)
	}
	return nil
}

// powerHandler starts/stops an allowlisted game container. POST only; the view-token gate is
// applied by the caller. Params (query or form): game=<key>, action=start|stop.
func powerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	game := r.FormValue("game")
	action := r.FormValue("action")
	name, ok := powerContainers[game]
	if !ok {
		http.Error(w, "unknown or forbidden server", http.StatusForbidden)
		return
	}
	if action != "start" && action != "stop" {
		http.Error(w, "action must be start or stop", http.StatusBadRequest)
		return
	}
	if err := containerPower(name, action); err != nil {
		fmt.Printf("[nextendo-dashboard] power %s %s FAILED: %v\n", action, name, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	fmt.Printf("[nextendo-dashboard] power %s %s OK\n", action, name)
	running, _ := containerRunning(name)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "game": game, "running": running})
}
