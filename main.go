// nextendo-dashboard is the UNIFIED live-monitoring site for the private Nextendo
// NEX secure servers (Mario Kart 8 Deluxe, Splatoon 2, Super Smash Bros Ultimate,
// Animal Crossing: New Horizons). Each game server already exposes a per-game
// /api/stats JSON on its own dashboard port; this aggregator polls them server-side, tags each by game,
// rolls them up into one payload (+ a short history for the realtime charts) and
// serves a single rich UI with game tabs and a global overview.
//
// Standard library only (net/http, encoding/json) — no NEX/Postgres here; the heavy
// per-game data extraction stays in each game server's dashboard.
package main

import (
	"crypto/subtle"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed worldmap.jpg
var worldMapJPG []byte


// ---- game sources ----------------------------------------------------------

type gameSrc struct {
	Key   string // "mk8" | "s2" | "ssbu" | "acnh"
	Label string
	Color string // accent hex used by the UI
	URL   string // base URL of that game's dashboard (…/api/stats appended)
	Token string // DASH_TOKEN of that game (?key=)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func sources() []gameSrc {
	tok := os.Getenv("DASH_TOKEN")
	return []gameSrc{
		{Key: "mk8", Label: "Mario Kart 8 DX", Color: "#ff3b4e",
			URL: envOr("DASH_MK8_URL", "http://localhost:8082"), Token: envOr("DASH_MK8_TOKEN", tok)},
		{Key: "s2", Label: "Splatoon 2", Color: "#2ee06a",
			URL: envOr("DASH_S2_URL", "http://localhost:8083"), Token: envOr("DASH_S2_TOKEN", tok)},
		// Splatoon 3 is NPLN, not NEX: the npln-s3 server publishes the same /api/stats shape
		// (gRPC calls counted as "RMC", game sessions as gatherings). Its container has no
		// published port — we reach it over the shared docker network.
		{Key: "s3", Label: "Splatoon 3", Color: "#e8ff2a",
			URL: envOr("DASH_S3_URL", "http://localhost:8089"), Token: envOr("DASH_S3_TOKEN", tok)},
		{Key: "ssbu", Label: "Smash Bros Ultimate", Color: "#ff913b",
			URL: envOr("DASH_SSBU_URL", "http://localhost:8084"), Token: envOr("DASH_SSBU_TOKEN", tok)},
		// the service-nexgo is the stopped container of the previous stack; the live ACNH
		// server is the service (the in-house core), so poll that one.
		{Key: "acnh", Label: "Animal Crossing: NH", Color: "#4aa8e8",
			URL: envOr("DASH_ACNH_URL", "http://localhost:8086"), Token: envOr("DASH_ACNH_TOKEN", tok)},
		// Minecraft n'utilise PAS le jeton commun : son serveur a le sien. Il doit donc
		// arriver par DASH_MC_TOKEN dans l'environnement du conteneur. L'écrire en dur
		// ici mettrait un secret vivant dans le dépôt, et sa réponse contient les
		// adresses IP des joueurs.
		{Key: "mc", Label: "Minecraft", Color: "#5d8c3f",
			URL: envOr("DASH_MC_URL", "http://localhost:8087"), Token: envOr("DASH_MC_TOKEN", tok)},
		{Key: "lm3", Label: "Luigi's Mansion 3", Color: "#9b6ef3",
			URL: envOr("DASH_LM3_URL", "http://localhost:8089"), Token: envOr("DASH_LM3_TOKEN", tok)},
		// ARMS partage le jeton commun et répond sur :8091. Son /api/stats rend
		// exactement la même forme que lm3, donc aucune adaptation n'est nécessaire.
		// Attention si on teste à la main : ce serveur veut le jeton en ?key=, pas
		// en en-tête Authorization — un Bearer se fait répondre « forbidden ».
		{Key: "arms", Label: "ARMS", Color: "#2ad4e6",
			URL: envOr("DASH_ARMS_URL", "http://localhost:8091"), Token: envOr("DASH_ARMS_TOKEN", tok)},
		// Mario Tennis Aces. Le conteneur s appelle `tennis` et son tableau de bord
		// ecoute sur 8092 — surtout pas 8084, qui est celui de SSBU chez nous.
		{Key: "mta", Label: "Mario Tennis Aces", Color: "#ff4fa3",
			URL: envOr("DASH_MTA_URL", "http://localhost:8092"), Token: envOr("DASH_MTA_TOKEN", tok)},
	}
}

// ---- per-game snapshot cache ----------------------------------------------

// rollup is the small subset of each game's /api/stats we sum for the global view.
type rollup struct {
	Connected     int   `json:"connected"`
	InLobby       int   `json:"inLobby"`
	ActiveLobbies int   `json:"activeLobbies"`
	TotalRMC      int64 `json:"totalRmc"`
	PeakConnected int   `json:"peakConnected"`
}

type snapshot struct {
	Online   bool
	LastPoll time.Time
	LastErr  string
	Raw      json.RawMessage // full /api/stats passed through to the UI
	R        rollup
}

var (
	mu    sync.RWMutex
	cache = map[string]*snapshot{} // game key -> snapshot

	histMu  sync.Mutex
	history []histPoint
	start   = time.Now()
)

type histPoint struct {
	T    int64          `json:"t"`    // unix seconds
	Conn map[string]int `json:"conn"` // game -> connected
	RMC  int64          `json:"rmc"`  // total RMC across games (cumulative)
}

func poll(src gameSrc, client *http.Client) {
	url := src.URL + "/api/stats"
	if src.Token != "" {
		url += "?key=" + src.Token
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	snap := &snapshot{LastPoll: time.Now()}
	if err != nil {
		snap.Online = false
		snap.LastErr = err.Error()
		mergeSnapshot(src.Key, snap, true)
		return
	}
	defer resp.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil || resp.StatusCode != 200 {
		snap.Online = false
		snap.LastErr = fmt.Sprintf("http %d", resp.StatusCode)
		mergeSnapshot(src.Key, snap, true)
		return
	}
	snap.Online = true
	snap.Raw = raw
	_ = json.Unmarshal(raw, &snap.R)
	mergeSnapshot(src.Key, snap, false)
}

// mergeSnapshot stores a new snapshot; on a failed poll it keeps the last good Raw
// (so a brief blip doesn't blank the UI) but flips Online=false.
func mergeSnapshot(key string, snap *snapshot, failed bool) {
	mu.Lock()
	defer mu.Unlock()
	if failed {
		if old := cache[key]; old != nil {
			snap.Raw = old.Raw
			snap.R = old.R
		}
	}
	cache[key] = snap
}

func pollLoop() {
	client := &http.Client{Timeout: 4 * time.Second}
	srcs := sources()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		var wg sync.WaitGroup
		for _, s := range srcs {
			wg.Add(1)
			go func(src gameSrc) { defer wg.Done(); poll(src, client) }(s)
		}
		wg.Wait()
		recordHistory()
		resolveNames(client)
		<-tick.C
	}
}

func recordHistory() {
	mu.RLock()
	conn := map[string]int{}
	var rmc int64
	for k, s := range cache {
		conn[k] = s.R.Connected
		rmc += s.R.TotalRMC
	}
	mu.RUnlock()
	histMu.Lock()
	history = append(history, histPoint{T: time.Now().Unix(), Conn: conn, RMC: rmc})
	if len(history) > 180 { // ~6 min at 2s
		history = history[len(history)-180:]
	}
	histMu.Unlock()
}

// ---- PID -> pseudo resolution (via the account server) ---------------------
// Games like S2/SSBU don't upload a name the dashboard can read, so players show as
// "Joueur-<pid>". But the NEX PID == the Nextendo account PID, so we resolve the real
// pseudo (+ avatar) from nextendo-account /api/names and let the UI override the label.

type nameEntry struct {
	Name  string `json:"name"`
	Image string `json:"image,omitempty"` // URL d'avatar (pp réelle) servie par nextendo-account /api/avatar
}

// cachedName is a resolved (or negatively-resolved) pseudo plus the time it was
// fetched, so entries can EXPIRE and be re-resolved. Without expiry a PID that had
// no account when first seen — a disposable counter PID, or any PID resolved during
// a brief account-server restart window — was negative-cached FOREVER and never
// picked up its real pseudo. That is exactly why a real console kept showing
// "Joueur-6" even after its NEX PID correctly became the account PID (the account PID).
type cachedName struct {
	name  string
	image string // URL d'avatar (vide si le compte n'a pas de pp)
	at    int64  // unix seconds when cached
}

var (
	nameMu    sync.RWMutex
	nameCache = map[uint64]cachedName{}
)

// Re-resolution TTLs: negative (no-account) entries retry quickly so a PID that
// gains an account self-heals within seconds; positive entries refresh slower to
// pick up pseudo changes without hammering the account server.
const (
	negNameTTL = 45 * time.Second
	posNameTTL = 300 * time.Second
)

func nameNeedsResolve(c cachedName, now int64) bool {
	age := time.Duration(now-c.at) * time.Second
	if c.name == "" {
		return age >= negNameTTL
	}
	return age >= posNameTTL
}

func accountURL() string { return envOr("DASH_ACCOUNT_URL", "http://localhost:8080") }

type pidScrape struct {
	Players []struct {
		PID uint64 `json:"pid"`
	} `json:"players"`
	Gatherings []struct {
		Players []struct {
			PID uint64 `json:"pid"`
		} `json:"players"`
	} `json:"gatherings"`
}

func resolveNames(client *http.Client) {
	mu.RLock()
	pids := map[uint64]bool{}
	for _, s := range cache {
		if s == nil || len(s.Raw) == 0 {
			continue
		}
		var ps pidScrape
		if json.Unmarshal(s.Raw, &ps) == nil {
			for _, p := range ps.Players {
				if p.PID != 0 {
					pids[p.PID] = true
				}
			}
			for _, g := range ps.Gatherings {
				for _, p := range g.Players {
					if p.PID != 0 {
						pids[p.PID] = true
					}
				}
			}
		}
	}
	mu.RUnlock()

	nameMu.RLock()
	var want []uint64
	now := time.Now().Unix()
	for pid := range pids {
		c, ok := nameCache[pid]
		if !ok || nameNeedsResolve(c, now) {
			want = append(want, pid)
		}
	}
	nameMu.RUnlock()
	if len(want) == 0 {
		return
	}

	parts := make([]string, 0, len(want))
	for _, p := range want {
		parts = append(parts, strconv.FormatUint(p, 10))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, accountURL()+"/api/names?pids="+strings.Join(parts, ","), nil)
	resp, err := client.Do(req)
	if err != nil {
		return // transient: retry next cycle (no negative cache on failure)
	}
	defer resp.Body.Close()
	var out struct {
		Names map[string]nameEntry `json:"names"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		return
	}
	nameMu.Lock()
	stamp := time.Now().Unix()
	for ps, ne := range out.Names {
		if pid, err := strconv.ParseUint(ps, 10, 64); err == nil {
			nameCache[pid] = cachedName{name: ne.Name, image: ne.Image, at: stamp}
		}
	}
	for _, p := range want { // negative-cache PIDs still without an account (retried after negNameTTL)
		if c, ok := nameCache[p]; !ok || c.name == "" {
			nameCache[p] = cachedName{name: "", at: stamp}
		}
	}
	nameMu.Unlock()
}

func namesPayload() map[string]nameEntry {
	nameMu.RLock()
	defer nameMu.RUnlock()
	out := map[string]nameEntry{}
	for pid, c := range nameCache {
		if c.name != "" {
			out[strconv.FormatUint(pid, 10)] = nameEntry{Name: c.name, Image: c.image}
		}
	}
	return out
}

// ---- unified payload -------------------------------------------------------

type gameOut struct {
	Key          string          `json:"key"`
	Label        string          `json:"label"`
	Color        string          `json:"color"`
	Online       bool            `json:"online"`
	AgeSecs      int             `json:"ageSeconds"`
	Stats        json.RawMessage `json:"stats"`
	Controllable bool            `json:"controllable"` // a start/stop button is offered (allowlisted container)
	Running      bool            `json:"running"`      // real Docker container state (for the button); falls back to Online
}

type globalOut struct {
	Connected     int            `json:"connected"`
	InLobby       int            `json:"inLobby"`
	ActiveLobbies int            `json:"activeLobbies"`
	TotalRMC      int64          `json:"totalRmc"`
	Peak          int            `json:"peak"`
	GamesOnline   int            `json:"gamesOnline"`
	PerGame       map[string]int `json:"perGame"`
}

type unified struct {
	ServerTime    string               `json:"serverTime"`
	UptimeSeconds int                  `json:"uptimeSeconds"`
	Games         []gameOut            `json:"games"`
	Global        globalOut            `json:"global"`
	History       []histPoint          `json:"history"`
	Names         map[string]nameEntry `json:"names"`
}

func buildUnified() unified {
	srcs := sources()
	mu.RLock()
	games := make([]gameOut, 0, len(srcs))
	g := globalOut{PerGame: map[string]int{}}
	maxGamePeak := 0 // highest single-game peak (each game persists its own peakConnected)
	for _, s := range srcs {
		snap := cache[s.Key]
		go0 := gameOut{Key: s.Key, Label: s.Label, Color: s.Color}
		if snap != nil {
			go0.Online = snap.Online
			go0.Stats = snap.Raw
			go0.AgeSecs = int(time.Since(snap.LastPoll).Seconds())
			// Un jeu HORS-LIGNE (poll en échec) ne compte PAS dans les totaux « live ». Sinon
			// mergeSnapshot conserve son dernier rollup connu et il reste affiché « en ligne »
			// indéfiniment : relevé en prod, acnh éteint depuis 5 jours comptait toujours 1
			// joueur dans le total global. Les compteurs live ne reflètent que les serveurs
			// réellement joignables ; l'onglet du jeu garde son dernier Raw, marqué hors-ligne.
			if snap.Online {
				g.Connected += snap.R.Connected
				g.InLobby += snap.R.InLobby
				g.ActiveLobbies += snap.R.ActiveLobbies
				g.PerGame[s.Key] = snap.R.Connected
				g.GamesOnline++
			} else {
				g.PerGame[s.Key] = 0
			}
			g.TotalRMC += snap.R.TotalRMC
			if snap.R.PeakConnected > maxGamePeak {
				maxGamePeak = snap.R.PeakConnected
			}
		}
		games = append(games, go0)
	}
	mu.RUnlock()

	// Power state for the start/stop buttons — queried from Docker OUTSIDE the cache lock
	// (a Docker call must never be made while holding mu). Only allowlisted containers.
	for i := range games {
		if name, ctrl := powerContainers[games[i].Key]; ctrl {
			games[i].Controllable = true
			if running, ok := containerRunning(name); ok {
				games[i].Running = running
			} else {
				games[i].Running = games[i].Online // Docker unreachable -> fall back to poll state
			}
		}
	}
	if g.Connected > peakConn {
		peakConn = g.Connected
	}
	// The global peak is the peak of the *simultaneous sum*, which resets when this
	// aggregator restarts and can miss a brief peak between 2s polls. Never report it
	// below the highest single-game peak (each game server persists its own peakConnected
	// for far longer), so "Pic connectés" can't undercount a single game (e.g. MK8=9).
	if maxGamePeak > peakConn {
		peakConn = maxGamePeak
	}
	g.Peak = peakConn
	histMu.Lock()
	h := make([]histPoint, len(history))
	copy(h, history)
	histMu.Unlock()
	return unified{
		ServerTime:    time.Now().Format("15:04:05"),
		UptimeSeconds: int(time.Since(start).Seconds()),
		Games:         games,
		Global:        g,
		History:       h,
		Names:         namesPayload(),
	}
}

var peakConn int

// ---- HTTP ------------------------------------------------------------------

func main() {
	port := envOr("DASH_PORT", envOr("PORT", "8090")) // DASH_PORT (deploy) > PORT (local) > 8090
	token := os.Getenv("DASH_VIEW_TOKEN")

	// ⚠️ ÉCHEC FERMÉ. Un jeton absent rendait authed() vrai pour tout le monde, ce qui
	// ouvrait /api/kick, /api/ban, /api/power et /api/flag à n'importe qui. Refuser de
	// démarrer se remarque tout de suite ; servir ouvert ne se remarque jamais.
	//
	// Le contrôle est ici ET PAS SEULEMENT dans authed() : sans lui, la comparaison à temps
	// constant ci-dessous rendrait vrai pour une clé vide face à un jeton vide — le même
	// trou, sous une forme moins visible.
	if token == "" {
		fmt.Println("[nextendo-dashboard] DASH_VIEW_TOKEN absent : refus de démarrer.")
		os.Exit(1)
	}

	if os.Getenv("NEXTENDO_DASH_MOCK") == "1" {
		go mockLoop()
	} else {
		go pollLoop()
	}

	authed := func(w http.ResponseWriter, r *http.Request) bool {
		// Comparaison a temps constant : le jeton voyage dans l'URL, autant ne pas y ajouter
		// une fuite par le temps de reponse.
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("key")), []byte(token)) == 1 {
			return true
		}
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		// Compression : la charge utile fait ~180 Ko et le navigateur la redemande
		// toutes les 2 s, soit plus de 5 Mo par minute et par spectateur. Du JSON
		// se compresse d'environ neuf fois : autant de données en moins sur le
		// réseau, donc autant d'occasions en moins qu'une requête échoue et que le
		// tableau de bord s'affiche « injoignable ».
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Vary", "Accept-Encoding")
			gz, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
			defer gz.Close()
			_ = json.NewEncoder(gz).Encode(buildUnified())
			return
		}
		_ = json.NewEncoder(w).Encode(buildUnified())
	})
	mux.HandleFunc("/api/power", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) { // same view-token gate as the rest of the dashboard
			return
		}
		powerHandler(w, r)
	})
	// Moderation. Meme porte que le reste du site pour le kick, qui est reversible.
	// Le ban, lui, exige EN PLUS la cle d administration en en-tete : le jeton de
	// consultation voyage dans l URL, donc dans les historiques et les captures.
	mux.HandleFunc("/api/kick", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		kickHandler(w, r)
	})
	mux.HandleFunc("/api/ban", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		banHandler(w, r)
	})
	mux.HandleFunc("/api/flag", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		flagHandler(w, r)
	})
	// Tout ce qu'on sait du festival Splatoon 3 en cours (voir splatfest.go).
	mux.HandleFunc("/api/splatfest", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		splatfestHandler(w, r)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ok") })
	mux.HandleFunc("/worldmap.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(worldMapJPG)
	})
	// Icônes de jeu (embarquées) — chargées par <img> sans token, comme la worldmap.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store") // always serve fresh HTML (no stale UI)
		_, _ = w.Write([]byte(strings.Replace(dashboardHTML, "/*FEST_UI*/", festUIJS, 1)))
	})

	fmt.Printf("[nextendo-dashboard] unified monitoring on :%s (mock=%s)\n", port, envOr("NEXTENDO_DASH_MOCK", "0"))
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Printf("[nextendo-dashboard] HTTP error: %v\n", err)
	}
}
