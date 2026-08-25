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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// powerContainers maps a dashboard game key -> the Docker container that runs it. ONLY these
// can be started/stopped from the dashboard.
var powerContainers = map[string]string{
	"s2":   "the service",
	"ssbu": "the service",
	"acnh": "the service",
	"mc":   "minecraft",
	"lm3":  "lm3",
	"s3":   "npln",
	"arms": "arms",
}

// ⚠️ PORTEE DU BOUTON POUR SPLATOON 3.
//
// S3 tourne en DEUX processus : le conteneur `npln` (le locataire, :443, matchmaking et comptes) et
// le service systemd `gamesync7575` (l'hote de session, :7575, les salons). Ce tableau de bord
// s'execute lui-meme dans un conteneur et ne parle qu'a la socket Docker : il ne peut donc PAS
// piloter un service systemd de l'hote.
//
// Le bouton agit donc sur le conteneur seul. En pratique c'est ce qu'on veut la plupart du temps :
// couper `npln` vide la file de matchmaking et deconnecte les joueurs, ce qui est l'usage courant.
// Mais les sessions de salon deja ouvertes survivent dans gamesync jusqu'a ce que leurs clients
// abandonnent.
//
// Pour un vrai bouton « tout eteindre », il faudra soit conteneuriser gamesync7575, soit ajouter un
// petit agent cote hote que le tableau de bord appelle. Tant que ce n'est pas fait, l'interface le
// dit explicitement plutot que de laisser croire a une extinction complete.

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

// ---- Drapeaux à chaud du serveur Splatoon 3 -----------------------------------------------
//
// POURQUOI CE SECOND MECANISME. Les boutons ci-dessus allument et éteignent un conteneur, ce qui
// déconnecte tout le monde. Certains correctifs de npln-s3 vivent au contraire derrière un DRAPEAU
// relu chaque seconde dans /data/soir.flags : les activer ou les retirer est immédiat et ne coupe
// aucun flux. C'est le retour arrière qu'on veut sous la main quand un correctif se révèle mauvais
// en pleine affluence — sans redéploiement, sans coupure, sans avoir besoin d'un accès SSH.
//
// Ce tableau de bord n'a que la socket Docker : il ne peut pas écrire dans /opt/npln de l'hôte. Il
// passe donc par un `exec` dans le conteneur npln, qui monte ce répertoire sur /data. Le fichier
// est le même que celui lu par l'hôte de session, donc les deux processus voient le changement.
//
// ⚠️ ALLOWLIST STRICTE, comme pour les conteneurs : seuls ces drapeaux-là sont pilotables d'ici.
var flagsPilotables = map[string]string{
	"rotavance": "Rotation des stages qui avance (sinon : figée sur un créneau, comportement d'avant)",
}

// execDansNpln lance une commande dans le conteneur du serveur S3 et attend sa fin. Le nom
// du conteneur vient de DASH_S3_CONTAINER : il depend du deploiement.
//
// ⚠️ La commande est un ARGV, jamais une chaine shell fournie par l'appelant. La version
// precedente prenait un « sh -c <chaine> » construit par le handler : tant que celui-ci
// validait son entree cela tenait, mais la fonction laissait une execution de commande
// arbitraire toute prete pour le prochain appelant qui l'oublierait — a travers la socket
// Docker, donc avec les droits de l'hote.
func execDansNpln(argv []string) error {
	body, _ := json.Marshal(map[string]any{
		"AttachStdout": true,
		"AttachStderr": true,
		"Cmd":          argv,
	})

	resp, err := dockerHTTP.Post("http://docker/containers/"+envOr("DASH_S3_CONTAINER", "s3")+"/exec", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var cree struct {
		ID string `json:"Id"`
	}
	if json.NewDecoder(resp.Body).Decode(&cree) != nil || cree.ID == "" {
		return fmt.Errorf("exec create: HTTP %d", resp.StatusCode)
	}

	start, err := dockerHTTP.Post("http://docker/exec/"+cree.ID+"/start", "application/json",
		bytes.NewReader([]byte(`{"Detach":false,"Tty":false}`)))
	if err != nil {
		return err
	}
	defer start.Body.Close()
	_, _ = io.Copy(io.Discard, start.Body)

	return nil
}

// flagHandler active ou retire un drapeau à chaud. POST : flag=<nom>&on=0|1.
func flagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	nom := r.FormValue("flag")
	if _, ok := flagsPilotables[nom]; !ok {
		http.Error(w, "unknown or forbidden flag", http.StatusForbidden)
		return
	}

	// Script CONSTANT : le nom du drapeau n'entre JAMAIS dans le texte de la commande, il
	// arrive en $1. Retirer d'abord dans tous les cas — c'est idempotent, et un double clic
	// ne peut donc pas écrire le drapeau deux fois.
	const script = `f=/data/soir.flags; touch "$f"; grep -vxF -- "$1" "$f" > "$f.tmp"; ` +
		`mv "$f.tmp" "$f"; if [ "$2" = 1 ]; then printf '%s\n' "$1" >> "$f"; fi; exit 0`

	if err := execDansNpln([]string{"sh", "-c", script, "sh", nom, r.FormValue("on")}); err != nil {
		fmt.Printf("[nextendo-dashboard] flag %s FAILED: %v\n", nom, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	fmt.Printf("[nextendo-dashboard] flag %s -> %s OK\n", nom, r.FormValue("on"))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "flag": nom, "on": r.FormValue("on") == "1"})
}
