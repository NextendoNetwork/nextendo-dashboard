package main

// Mock data generator: set NEXTENDO_DASH_MOCK=1 to drive the unified UI with
// synthetic-but-realistic per-game stats that drift over time, so the front-end can
// be developed without the three live servers. Shape MUST match each game server's
// /api/stats (see the previous stack/dashboard.go apiStats).

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

type mPlayer struct {
	PID         uint64 `json:"pid"`
	Name        string `json:"name"`
	IP          string `json:"ip"`
	State       string `json:"state"`
	Gathering   uint32 `json:"gathering"`
	CommonData  int    `json:"commonDataBytes"`
	OnlineSecs  int    `json:"onlineSeconds"`
	Calls       int64  `json:"calls"`
	LastAction  string `json:"lastAction"`
	IdleSecs    int    `json:"idleSeconds"`
	Country     string `json:"country"`
	CC          string `json:"cc"`
	City        string `json:"city"`
	ISP         string `json:"isp"`
	VR          uint32 `json:"vr"`
	Mode        string `json:"mode"`
	IsHost      bool   `json:"isHost"`
	MiiColor    string `json:"miiColor"`
	MiiUrl      string `json:"miiUrl"`
}

type mLobbyP struct {
	PID  uint64 `json:"pid"`
	Name string `json:"name"`
	VR   uint32 `json:"vr"`
	Host bool   `json:"host"`
}

type mGathering struct {
	ID       uint32    `json:"id"`
	HostPID  uint64    `json:"hostPid"`
	HostName string    `json:"hostName"`
	Type     string    `json:"type"`
	Mode     uint32    `json:"mode"`
	VR       uint32    `json:"vr"`
	Players  []mLobbyP `json:"players"`
	Count    int       `json:"count"`
	Max      uint32    `json:"max"`
	State    string    `json:"state"`
}

type mStats struct {
	ServerTime     string                   `json:"serverTime"`
	UptimeSeconds  int                      `json:"uptimeSeconds"`
	Connected      int                      `json:"connected"`
	InLobby        int                      `json:"inLobby"`
	ActiveLobbies  int                      `json:"activeLobbies"`
	TotalSessions  int64                    `json:"totalSessions"`
	TotalRMC       int64                    `json:"totalRmc"`
	GatheringsMade int64                    `json:"gatheringsMade"`
	PeakConnected  int                      `json:"peakConnected"`
	Server         map[string]interface{}   `json:"server"`
	Players        []mPlayer                `json:"players"`
	Gatherings     []mGathering             `json:"gatherings"`
	Events         []map[string]interface{} `json:"events"`
	Methods        []map[string]interface{} `json:"methods"`
}

var mockNames = []string{"Player", "Marin", "Léo", "Yuki", "Sora", "Nina", "Tom", "Aya", "Max", "Rin", "Eli", "Zoé", "Kai", "Mia", "Théo", "Luna"}
var mockGeo = []struct{ c, cc, city, isp string }{
	{"France", "FR", "Paris", "Orange"}, {"France", "FR", "Lyon", "Free"},
	{"Belgique", "BE", "Bruxelles", "Proximus"}, {"Canada", "CA", "Montréal", "Bell"},
	{"Japon", "JP", "Tokyo", "NTT"}, {"États-Unis", "US", "New York", "Comcast"},
	{"Allemagne", "DE", "Berlin", "Telekom"}, {"Espagne", "ES", "Madrid", "Movistar"},
}

type gameFlavor struct {
	modes   []string
	method  []string
	rating  func(r *rand.Rand) uint32
	rname   string
}

var flavors = map[string]gameFlavor{
	"mk8": {modes: []string{"Course", "Course (équipe)", "Bataille", "Bataille (équipe)"},
		method: []string{"MatchmakeExt::AutoMatchmake", "Ranking::UploadCommonData", "MatchMaking::GetSessionURLs", "NATTraversal::ReportNATProperties", "Utility::AcquireNexUniqueID"},
		rating: func(r *rand.Rand) uint32 { return uint32(1000 + r.Intn(28000)) }, rname: "VR"},
	"s2": {modes: []string{"Pluie de mèches", "Match classé · Zones", "Match classé · Tour", "Ligue", "Festival · Famille"},
		method: []string{"MatchmakeExt::AutoMatchmake", "DataStore::GetMetaByPID", "Ranking::GetFestivalResult", "Utility::UpdateCurrentUser", "NATTraversal::ReportNATProperties"},
		rating: func(r *rand.Rand) uint32 { return uint32(r.Intn(3000)) }, rname: "Puissance"},
	"ssbu": {modes: []string{"Quickplay · 3 vies", "Quickplay · Temps", "Arène · Invité", "Background Matchmaking"},
		method: []string{"MatchmakeExt::AutoMatchmake", "MatchMaking::GetSessionURLs", "Ranking::GetCommonData", "Utility::AcquireNexUniqueID", "NATTraversal::ReportNATProperties"},
		rating: func(r *rand.Rand) uint32 { return uint32(2000000 + r.Intn(8000000)) }, rname: "PSP"},
	"acnh": {modes: []string{"Visite d'île · ami", "Visite d'île · Dodo Code", "Île ouverte", "Aéroport"},
		method: []string{"MatchmakeExt::FindMatchmakeSessionByParticipant", "MatchmakeExt::BrowseMatchmakeSessionNoHolderNoResultRange", "MatchmakeExt::JoinMatchmakeSessionWithParam", "MatchmakeExt::UpdateMatchmakeSessionPart", "NATTraversal::ReportNATProperties"},
		rating: func(r *rand.Rand) uint32 { return uint32(r.Intn(60000)) }, rname: "Miles"},
}

func mockGame(key string, t int64) (json.RawMessage, rollup) {
	r := rand.New(rand.NewSource(t/8 + int64(len(key)))) // changes every ~16s
	fl := flavors[key]
	// connection count drifts per game
	base := map[string]int{"mk8": 5, "s2": 7, "ssbu": 3, "acnh": 4}[key]
	n := base + r.Intn(4)
	players := make([]mPlayer, 0, n)
	rmcTotal := int64(2000 + (t-startUnix)/2*int64(n) + int64(r.Intn(500)))
	// build a lobby of the first few
	lobN := 2 + r.Intn(3)
	if lobN > n {
		lobN = n
	}
	var lobPlayers []mLobbyP
	gid := uint32(8000 + r.Intn(900))
	mode := uint32(1 + r.Intn(len(fl.modes)))
	hostPID := uint64(1800000000 + r.Intn(99999))
	for i := 0; i < n; i++ {
		g := mockGeo[r.Intn(len(mockGeo))]
		pid := uint64(1800000000 + r.Intn(99999))
		name := mockNames[(i+int(t/8))%len(mockNames)]
		inLobby := i < lobN
		st := "en ligne"
		var gth uint32
		host := false
		if inLobby {
			st = "dans un lobby"
			gth = gid
			pid2 := pid
			if i == 0 {
				host = true
				pid2 = hostPID
				pid = hostPID
			}
			lobPlayers = append(lobPlayers, mLobbyP{PID: pid2, Name: name, VR: fl.rating(r), Host: host})
		}
		players = append(players, mPlayer{
			PID: pid, Name: name, IP: fmt.Sprintf("90.%d.%d.%d", r.Intn(255), r.Intn(255), 1+r.Intn(254)),
			State: st, Gathering: gth, CommonData: 200 + r.Intn(400),
			OnlineSecs: 30 + r.Intn(3000), Calls: int64(20 + r.Intn(400)),
			LastAction: fl.method[r.Intn(len(fl.method))], IdleSecs: r.Intn(40),
			Country: g.c, CC: g.cc, City: g.city, ISP: g.isp,
			VR: fl.rating(r), Mode: fl.modes[r.Intn(len(fl.modes))], IsHost: host,
		})
	}
	gatherings := []mGathering{}
	if len(lobPlayers) > 0 {
		state := "en recherche"
		if len(lobPlayers) >= 2 {
			state = "apparié"
		}
		gatherings = append(gatherings, mGathering{
			ID: gid, HostPID: hostPID, HostName: lobPlayers[0].Name,
			Type: fl.modes[mode%uint32(len(fl.modes))], Mode: mode, VR: fl.rating(r),
			Players: lobPlayers, Count: len(lobPlayers), Max: 8, State: state,
		})
	}
	events := []map[string]interface{}{}
	for i := 0; i < 14; i++ {
		events = append(events, map[string]interface{}{
			"agoSeconds": i*3 + r.Intn(3), "pid": players[r.Intn(len(players))].PID,
			"action": fl.method[r.Intn(len(fl.method))],
		})
	}
	methods := []map[string]interface{}{}
	for i, m := range fl.method {
		methods = append(methods, map[string]interface{}{"name": m, "count": int64(120 - i*18 + r.Intn(30))})
	}
	st := mStats{
		ServerTime: time.Now().Format("15:04:05"), UptimeSeconds: int(t - startUnix),
		Connected: n, InLobby: len(lobPlayers), ActiveLobbies: len(gatherings),
		TotalSessions: int64(40 + r.Intn(30)), TotalRMC: rmcTotal,
		GatheringsMade: int64(10 + r.Intn(20)), PeakConnected: n + r.Intn(5),
		Server: map[string]interface{}{
			"accessKey": map[string]string{"mk8": "25dbf96a", "s2": "f6c2bcb3", "ssbu": "2c4f5e21", "acnh": "7d9e4b02"}[key],
			"nexVersion": map[string]string{"mk8": "4.3.3", "s2": "3.10.0", "ssbu": "4.6.3", "acnh": "4.0.0"}[key],
			"authPort": "443 (the reverse proxy SNI)", "securePort": map[string]int{"mk8": 60003, "s2": 60004, "ssbu": 60005, "acnh": 60007}[key],
			"sniHost": "nintendo.net", "sessionKeyLen": 32,
		},
		Players: players, Gatherings: gatherings, Events: events, Methods: methods,
	}
	raw, _ := json.Marshal(st)
	return raw, rollup{Connected: n, InLobby: len(lobPlayers), ActiveLobbies: len(gatherings), TotalRMC: rmcTotal, PeakConnected: st.PeakConnected}
}

var startUnix = time.Now().Unix()

func mockLoop() {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		now := time.Now().Unix()
		for _, key := range []string{"mk8", "s2", "ssbu", "acnh"} {
			raw, rl := mockGame(key, now)
			// ssbu occasionally "offline" to exercise that state
			online := !(key == "ssbu" && (now/30)%4 == 0)
			mergeSnapshot(key, &snapshot{Online: online, LastPoll: time.Now(), Raw: raw, R: rl}, false)
		}
		recordHistory()
		<-tick.C
	}
}
