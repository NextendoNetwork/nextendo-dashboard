package main

// Le panneau Splatfest de la section Splatoon 3.
//
// CE QU'IL MONTRE. Tout ce que le serveur de jeu sait de la fête en cours : le calendrier de ses
// cinq phases, les trois camps avec leur part en direct, les points calculés et le DÉTAIL de ce
// calcul, la liste nominative des votants, et le barème de référence. La page n'invente rien et ne
// recalcule rien : les parts, les rangs et les points arrivent déjà faits de npln-s3, où ils
// sortent de la fonction qui alimente le verdict envoyé à la console. Si un jour l'écran de
// résultats du jeu et cette page ne disaient pas la même chose, ce serait un défaut du serveur, pas
// un écart d'affichage.
//
// POURQUOI LES PSEUDOS SONT RÉSOLUS ICI. La résolution habituelle ne connaît que les joueurs vus
// dans les instantanés, c'est-à-dire ceux qui sont en ligne. Or un votant passe l'essentiel de la
// fête hors ligne : sa ligne n'aurait montré qu'un identifiant NPLN. On complète donc la réponse
// avec le pseudo et l'avatar de chaque votant, en passant par le même serveur de comptes et le même
// cache que le reste du tableau de bord.

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// splatfestHandler relaie /api/splatfest du serveur Splatoon 3, en y ajoutant les pseudos.
func splatfestHandler(w http.ResponseWriter, r *http.Request) {
	var src *gameSrc
	for _, s := range sources() {
		if s.Key == "s3" {
			s := s
			src = &s

			break
		}
	}
	if src == nil {
		http.Error(w, `{"actif":false,"erreur":"aucune source Splatoon 3"}`, http.StatusServiceUnavailable)

		return
	}

	url := src.URL + "/api/splatfest"
	if src.Token != "" {
		url += "?key=" + src.Token
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, `{"actif":false,"erreur":"serveur Splatoon 3 injoignable"}`, http.StatusBadGateway)

		return
	}
	defer resp.Body.Close()

	// On décode en carte générique : tout champ que npln-s3 ajoutera plus tard traversera ce
	// relais sans qu'on ait à y toucher.
	var doc map[string]interface{}
	if json.NewDecoder(resp.Body).Decode(&doc) != nil || resp.StatusCode != http.StatusOK {
		http.Error(w, `{"actif":false,"erreur":"réponse illisible"}`, http.StatusBadGateway)

		return
	}

	nommerLesVotants(doc)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(doc)
}

// nommerLesVotants ajoute « pseudo » et « image » à chaque votant qui a un compte Nextendo.
func nommerLesVotants(doc map[string]interface{}) {
	liste, _ := doc["votants"].([]interface{})
	if len(liste) == 0 {
		return
	}

	pids := map[uint64]bool{}
	for _, v := range liste {
		m, _ := v.(map[string]interface{})
		f, _ := m["pid"].(float64)
		if f <= 0 {
			// Inscription posee avant que npln-s3 retienne le PID du votant : sa ligne serait
			// restee anonyme pour toute la duree de la fete. L'uid NPLN etant une derivation
			// pure du PID, la table inverse le retrouve.
			u, _ := m["uid"].(string)
			if pid := pidParUidNPLN(u); pid > 0 {
				f = float64(pid)
				m["pid"] = f
			}
		}
		if f > 0 {
			pids[uint64(f)] = true
		}
	}

	resoudrePids(pids)

	nameMu.RLock()
	defer nameMu.RUnlock()
	for _, v := range liste {
		m, _ := v.(map[string]interface{})
		f, ok := m["pid"].(float64)
		if !ok || f <= 0 {
			continue
		}
		if c, ok := nameCache[uint64(f)]; ok && c.name != "" {
			m["pseudo"] = c.name
			m["image"] = c.image
		}
	}
}

// resoudrePids interroge le serveur de comptes pour les PID que le cache ne connaît pas encore, et
// alimente ce même cache. C'est le pendant de resolveNames pour des joueurs qui ne sont pas dans
// les instantanés — donc typiquement hors ligne.
func resoudrePids(pids map[uint64]bool) {
	nameMu.RLock()
	var want []string
	now := time.Now().Unix()
	for pid := range pids {
		c, ok := nameCache[pid]
		if !ok || nameNeedsResolve(c, now) {
			want = append(want, strconv.FormatUint(pid, 10))
		}
	}
	nameMu.RUnlock()
	if len(want) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		accountURL()+"/api/names?pids="+strings.Join(want, ","), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return // passager : on réessaiera au prochain affichage, sans cache négatif
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
	for _, ps := range want { // cache négatif : un PID sans compte ne sera redemandé qu'après negNameTTL
		if pid, err := strconv.ParseUint(ps, 10, 64); err == nil {
			if c, ok := nameCache[pid]; !ok || c.name == "" {
				nameCache[pid] = cachedName{name: "", at: stamp}
			}
		}
	}
	nameMu.Unlock()
}

// cheminTableUid : la table inverse uid NPLN -> PID Nextendo, fournie par le deploiement par
// le generateur de la table et refaite toutes les dix minutes.
var cheminTableUid = envOr("DASH_UID_TABLE", "npln-uid-pid.json")

var (
	tblUidMu  sync.RWMutex
	tblUid    map[string]uint64
	tblUidMod time.Time
)

// pidParUidNPLN rend le PID Nextendo dont derive un uid NPLN, ou 0 s'il est inconnu.
//
// POURQUOI UNE TABLE. Un uid NPLN n'est pas tire au sort : c'est un HMAC du PID sous le secret
// partage. La table inverse est donc entierement reconstructible, et c'est ce que fait le
// generateur pose a cote du binaire. Le tableau de bord ne detient pas le secret et n'a pas a le
// detenir : il ne lit que le resultat.
//
// A QUOI ELLE SERT. npln-s3 n'a commence a retenir le PID d'un votant que le 24 aout 2026. Les
// inscriptions anterieures n'ont pas un PID nul, elles n'ont pas de champ PID du tout : sans
// cette table, ces lignes resteraient anonymes pendant toute la fete, et une fete dure une
// semaine.
//
// Le fichier est relu des que sa date de modification change, donc un compte cree apres coup est
// pris en compte au plus tard dix minutes plus tard, sans redemarrage.
func pidParUidNPLN(uid string) uint64 {
	if uid == "" {
		return 0
	}

	st, err := os.Stat(cheminTableUid)
	if err != nil {
		return 0
	}

	tblUidMu.RLock()
	if tblUid != nil && st.ModTime().Equal(tblUidMod) {
		pid := tblUid[uid]
		tblUidMu.RUnlock()

		return pid
	}
	tblUidMu.RUnlock()

	tblUidMu.Lock()
	defer tblUidMu.Unlock()

	if tblUid == nil || !st.ModTime().Equal(tblUidMod) {
		b, err := os.ReadFile(cheminTableUid)
		if err != nil {
			return 0
		}

		var t map[string]uint64
		if json.Unmarshal(b, &t) != nil {
			return 0
		}

		tblUid, tblUidMod = t, st.ModTime()
		log.Printf("[splatfest] table uid->pid rechargee : %d comptes", len(t))
	}

	return tblUid[uid]
}

// festUIJS est injecté dans la page à la place du repère /*FEST_UI*/.
//
// Chaîne brute Go : AUCUN accent grave ici, et le JS n'utilise que de la concaténation.
const festUIJS = `
  /* ---- Panneau Splatfest (Splatoon 3) ------------------------------------- */
  var FEST=null, FEST_OUVERT=false, FEST_EN_VOL=false;

  (function(){
    var st=document.createElement('style');
    st.textContent=''
     +'.fbtn{display:inline-flex;align-items:center;gap:7px;font-size:12px;font-weight:800;padding:7px 14px;border-radius:10px;border:1px solid var(--line2);background:var(--card2);color:var(--txt);cursor:pointer;transition:.12s;margin:14px 0 0}'
     +'.fbtn:hover{border-color:#e8ff2a;color:#e8ff2a}'
     +'.fbtn.on{border-color:#e8ff2a;color:#e8ff2a;background:rgba(232,255,42,.10)}'
     +'.fpan{margin-top:14px;border:1px solid var(--line2);border-radius:14px;background:var(--card);overflow:hidden}'
     +'.fhead{display:flex;flex-wrap:wrap;align-items:center;gap:9px 16px;padding:13px 16px;border-bottom:1px solid var(--line2);background:var(--card2);font-size:12px}'
     +'.fhead b{color:var(--txt)}'
     +'.fph{font-weight:800;padding:3px 10px;border-radius:999px;font-size:11px;letter-spacing:.3px}'
     +'.fsec{padding:15px 16px;border-bottom:1px solid var(--line2)}'
     +'.fsec:last-child{border-bottom:0}'
     +'.ftit{font-size:11px;font-weight:800;letter-spacing:.6px;text-transform:uppercase;color:var(--faint);margin-bottom:11px;display:flex;align-items:center;gap:7px}'
     +'.ffrise{display:flex;flex-wrap:wrap;gap:8px}'
     +'.fetape{flex:1 1 150px;min-width:150px;border:1px solid var(--line2);border-radius:10px;padding:9px 11px;background:var(--card2)}'
     +'.fetape.passe{opacity:.45}'
     +'.fetape.ici{border-color:#e8ff2a;box-shadow:0 0 0 1px rgba(232,255,42,.35) inset}'
     +'.fetape .n{font-size:10px;font-weight:800;letter-spacing:.5px;text-transform:uppercase;color:var(--faint)}'
     +'.fetape .d{font-size:13px;font-weight:700;margin-top:3px}'
     +'.fetape .r{font-size:11px;color:#e8ff2a;margin-top:2px;font-weight:700}'
     +'.fcamps{display:flex;flex-wrap:wrap;gap:10px}'
     +'.fcamp{flex:1 1 210px;min-width:210px;border:1px solid var(--line2);border-radius:12px;padding:12px 13px;background:var(--card2);position:relative}'
     +'.fcamp.gagne{border-color:#ffce3a;box-shadow:0 0 0 1px rgba(255,206,58,.30) inset}'
     +'.fcamp .nom{font-weight:800;font-size:13px;display:flex;align-items:center;gap:6px}'
     +'.fcamp .cle{font-size:10px;color:var(--faint);font-weight:700;letter-spacing:.5px}'
     +'.fcamp .pct{font-size:26px;font-weight:900;line-height:1.1;margin-top:6px}'
     +'.fcamp .sub{font-size:11px;color:var(--faint);margin-top:2px}'
     +'.fcamp .pts{font-size:13px;font-weight:800;margin-top:8px}'
     +'.fbar{height:6px;border-radius:999px;background:var(--line2);margin-top:8px;overflow:hidden}'
     +'.fbar i{display:block;height:100%;border-radius:999px}'
     +'table.ftab{width:100%;border-collapse:collapse;font-size:12px}'
     +'table.ftab th,table.ftab td{padding:7px 9px;border-bottom:1px solid var(--line2);text-align:left;white-space:nowrap}'
     +'table.ftab th{font-size:10px;letter-spacing:.5px;text-transform:uppercase;color:var(--faint);font-weight:800}'
     +'table.ftab td.num{text-align:right;font-variant-numeric:tabular-nums;font-weight:700}'
     +'table.ftab tr.tot td{border-top:2px solid var(--line2);border-bottom:0;font-weight:900;font-size:13px}'
     +'.fwarn{font-size:11px;color:#ffce3a;margin-top:9px;display:flex;gap:7px;align-items:flex-start;line-height:1.45}'
     +'.fnote{font-size:11px;color:var(--faint);margin-top:9px;line-height:1.5}'
     +'.fmes{display:inline-block;font-size:9px;font-weight:800;padding:1px 6px;border-radius:999px;margin-left:6px;vertical-align:1px}'
     +'.fmes.oui{background:rgba(46,224,106,.16);color:#5fe08c}'
     +'.fmes.non{background:rgba(255,206,58,.14);color:#ffce3a}'
     +'.fdot{width:7px;height:7px;border-radius:50%;display:inline-block;margin-right:6px}'
     +'.fscroll{overflow-x:auto}'
     +'.fproj{display:flex;gap:9px;align-items:flex-start;font-size:11.5px;line-height:1.5;color:#ffce3a;background:rgba(255,206,58,.08);border:1px solid rgba(255,206,58,.30);border-radius:10px;padding:10px 12px;margin-bottom:11px}'
     +'.fcamp .pts.vide{color:var(--faint);font-weight:700;font-size:12px}'
     +'.fvide{padding:22px;text-align:center;color:var(--faint);font-size:12px}';
    document.head.appendChild(st);
  })();

  function festBouton(){
    return '<button class="fbtn '+(FEST_OUVERT?'on':'')+'" onclick="festBascule()">'
      +ic('flag')+(FEST_OUVERT?'Masquer le Splatfest':'Splatfest')+'</button>'
      +'<div id="fpanwrap">'+(FEST_OUVERT?festPanneau():'')+'</div>';
  }

  function festBascule(){
    FEST_OUVERT=!FEST_OUVERT;
    if(FEST_OUVERT && !FEST) festCharge(true);
    if(LAST) draw(LAST);
  }

  /* Le panneau se rafraichit tout seul pendant qu'il est ouvert, un peu moins souvent que
     l'instantane general : ces chiffres bougent au rythme des votes, pas des trames reseau. */
  setInterval(function(){ if(FEST_OUVERT) festCharge(); }, 4000);

  function festCharge(force){
    if(FEST_EN_VOL) return;
    if(!FEST_OUVERT && !force) return;
    FEST_EN_VOL=true;
    fetch('/api/splatfest'+KEY,{cache:'no-store'})
      .then(function(r){ return r.json(); })
      .then(function(j){ FEST=j; if(FEST_OUVERT){ var w=document.getElementById('fpanwrap'); if(w) w.innerHTML=festPanneau(); } })
      .catch(function(){})
      .then(function(){ FEST_EN_VOL=false; });
  }

  function fdate(ts){
    if(!ts) return '—';
    var d=new Date(ts*1000);
    return d.toLocaleString('fr-FR',{timeZone:'Europe/Paris',weekday:'short',day:'2-digit',month:'short',hour:'2-digit',minute:'2-digit'});
  }
  function frest(ts,now){
    var s=ts-now; if(s<=0) return '';
    var j=Math.floor(s/86400), h=Math.floor((s%86400)/3600), m=Math.floor((s%3600)/60);
    if(j>0) return 'dans '+j+' j '+h+' h';
    if(h>0) return 'dans '+h+' h '+m+' min';
    return 'dans '+m+' min';
  }
  function fpct(x){ return ((x||0)*100).toFixed(3)+' %'; }

  var FCOUL={Alpha:'#ff5aa8',Bravo:'#26d7c8',Charlie:'#ffce3a'};
  function fcoul(c){ return FCOUL[c]||'#9cc1ff'; }

  var FPHASE={
    avant:    {t:'Pas encore annoncée', c:'#8b97c4', b:'rgba(139,151,196,.16)'},
    annonce:  {t:'Annoncée — choix du camp', c:'#9cc1ff', b:'rgba(106,166,255,.16)'},
    premiere: {t:'1re mi-temps', c:'#5fe08c', b:'rgba(46,224,106,.18)'},
    seconde:  {t:'2de mi-temps', c:'#5fe08c', b:'rgba(46,224,106,.18)'},
    attente:  {t:'Terminée — verdict à venir', c:'#ffce3a', b:'rgba(255,206,58,.16)'},
    resultats:{t:'Résultats publiés', c:'#ff5aa8', b:'rgba(255,90,168,.16)'}
  };

  /* Le nom affiche d'un camp, tel que le joueur le voit dans le jeu. Le serveur ne connait les
     camps que par Alpha/Bravo/Charlie ; c'est le drapeau festcamps qui leur donne un nom, et il
     arrive deja resolu dans f.camps. On le retrouve ici plutot que de repeter la cle. */
  function nomDeCamp(f, cle){
    var c=(f&&f.camps)||[];
    for(var i=0;i<c.length;i++) if(c[i].cle===cle) return c[i].nom||cle;
    return cle;
  }

  function festPanneau(){
    var f=FEST;
    if(!f) return '<div class="fpan"><div class="fvide">chargement…</div></div>';
    if(!f.actif) return '<div class="fpan"><div class="fvide">'+ic('flag')+' Aucun festival servi en ce moment.'
      +(f.erreur?'<br><span style="color:#ff6b6b">'+esc(f.erreur)+'</span>':'')+'</div></div>';

    var now=f.maintenant||Math.floor(Date.now()/1000);
    var ph=FPHASE[f.phase]||{t:f.phaseLibelle||f.phase,c:'#9cc1ff',b:'rgba(106,166,255,.16)'};
    var h='<div class="fpan'+(f.projection?' proj':'')+'">';

    h+='<div class="fhead">'
      +'<span class="fph" style="background:'+ph.b+';color:'+ph.c+'">'+esc(ph.t)+'</span>'
      +'<span>'+ic('hash')+'Festival <b>'+esc(f.festId)+'</b></span>'
      +'<span>'+ic('layers')+'Paquet <b>'+esc(f.ressource)+'</b></span>'
      +'<span>'+ic('key')+'Révision <b>'+esc(String(f.revision||'').slice(0,12))+'…</b></span>'
      +'<span>'+ic('globe')+'Régions <b>'+esc((f.regions||[]).join(' '))+'</b></span>'
      +'<span>'+ic('flag')+'Logo écran-titre <b>'+(f.logo?esc(f.logo):'aucun')+'</b></span>'
      +'</div>';

    /* --- frise des cinq phases --- */
    var bornes=[['annonce','Annonce'],['debut','Début'],['intermede','Intermède'],['fin','Fin'],['resultats','Résultats']];
    h+='<div class="fsec"><div class="ftit">'+ic('timer')+'Calendrier (heure de Paris)</div><div class="ffrise">';
    for(var i=0;i<bornes.length;i++){
      var ts=(f.horaires||{})[bornes[i][0]]||0;
      var passe=ts<=now, prochaine=!passe;
      /* l etape « ici » est la derniere franchie */
      var suiv=(f.horaires||{})[(bornes[i+1]||[])[0]]||0;
      var ici=passe && (!suiv || suiv>now);
      h+='<div class="fetape '+(passe?'passe ':'')+(ici?'ici':'')+'">'
        +'<div class="n">'+bornes[i][1]+'</div>'
        +'<div class="d">'+fdate(ts)+'</div>'
        +(prochaine?'<div class="r">'+frest(ts,now)+'</div>':'')
        +'</div>';
    }
    h+='</div></div>';

    /* --- les camps --- */
    var camps=f.camps||[];
    var totalVotes=0; for(var a=0;a<camps.length;a++) totalVotes+=camps[a].votes||0;
    h+='<div class="fsec"><div class="ftit">'+ic('users')+'Camps — part en direct</div>';
    if(f.projection) h+='<div class="fproj">'+ic('zap')+'<span><b>Aucun vote pour le moment, donc aucun point.</b> '
      +'Les pourcentages ci-dessous sont les parts de DEPART : trois valeurs proches du tiers que le serveur derive de '
      +'l&#39;identifiant de la fete, parce que le jeu n&#39;affiche jamais un tiers pile avant le premier vote. '
      +'Le classement et les points n&#39;ont donc rien a mesurer : ils apparaitront des la premiere inscription.</span></div>';
    h+='<div class="fcamps">';
    for(var b=0;b<camps.length;b++){
      var c=camps[b], col=fcoul(c.cle);
      h+='<div class="fcamp '+(c.vainqueur?'gagne':'')+'">'
        +'<div class="nom" style="color:'+col+'">'+(c.vainqueur?'&#9733; ':'')+esc(c.nom)+'</div>'
        +'<div class="cle">'+esc(c.cle)+(f.projection?'':' &middot; rang '+c.rang)+'</div>'
        +'<div class="pct" style="color:'+col+'">'+fpct(c.part)+'</div>'
        +'<div class="sub">'+(c.votes||0)+' vote'+((c.votes||0)>1?'s':'')+' sur '+totalVotes+'</div>'
        +'<div class="fbar"><i style="width:'+Math.max(2,Math.min(100,(c.part||0)*100))+'%;background:'+col+'"></i></div>'
        +(f.projection?'<div class="pts vide">— aucun point calcule</div>'
                      :'<div class="pts">'+c.points+' point'+(c.points>1?'s':'')+'</div>')
        +'</div>';
    }
    h+='</div>';
    h+='</div>';

    /* --- detail du calcul --- */
    h+='<div class="fsec"><div class="ftit">'+ic('layers')+'Détail du calcul des points</div><div class="fscroll"><table class="ftab">';
    h+='<tr><th>Critère</th>';
    for(var d0=0;d0<camps.length;d0++) h+='<th style="color:'+fcoul(camps[d0].cle)+';text-align:right">'+esc(camps[d0].nom)+'</th>';
    h+='</tr>';
    var lignes=(camps[0]&&camps[0].detail)||[];
    for(var e=0;e<lignes.length;e++){
      h+='<tr><td>'+esc(lignes[e].libelle)
        +'<span class="fmes '+(lignes[e].reel?'oui':'non')+'">'+(lignes[e].reel?'mesuré':'recopié du vote')+'</span></td>';
      for(var g=0;g<camps.length;g++){
        var dt=camps[g].detail[e]||{};
        h+='<td class="num" style="color:'+(!f.projection&&dt.points>0?fcoul(camps[g].cle):'var(--faint)')+'">'
          +(f.projection?'—':dt.points+'<span style="color:var(--faint);font-weight:600"> ('+dt.place+'<sup>e</sup>)</span>')+'</td>';
      }
      h+='</tr>';
    }
    h+='<tr class="tot"><td>Total</td>';
    for(var k=0;k<camps.length;k++) h+='<td class="num" style="color:'+(f.projection?'var(--faint)':fcoul(camps[k].cle))+'">'+(f.projection?'—':camps[k].points)+'</td>';
    h+='</tr></table></div>';
    if(!f.projection) h+='<div class="fwarn">'+ic('zap')+'<span>Un seul des cinq critères est réellement mesuré aujourd\'hui : le vote préliminaire. '
      +'Les quatre autres recopient l\'ordre du vote, faute de batailles comptées — c\'est pourquoi un camp peut afficher '
      +'180 points de match tricolore sans qu\'aucun match tricolore ait eu lieu.</span></div>';
    h+='</div>';

    /* --- les votants --- */
    var v=f.votants||[];
    h+='<div class="fsec"><div class="ftit">'+ic('usercheck')+'Votants ('+v.length+')</div>';
    if(!v.length){
      h+='<div class="fnote">Aucune inscription pour ce festival.</div>';
    }else{
      h+='<div class="fscroll"><table class="ftab">'
        +'<tr><th>Joueur</th><th>PID</th><th>Camp</th><th>Région</th><th>Choix effectué</th><th>Identifiant NPLN</th></tr>';
      for(var m=0;m<v.length;m++){
        var x=v[m], col2=fcoul(x.camp);
        /* Deux absences differentes, deux phrases differentes : un PID nul veut dire que le vote
           precede le suivi des identites, pas que le joueur n'a pas de compte. */
        var nom=x.pseudo?esc(x.pseudo)
          :(x.pid?'<span style="color:var(--faint)">sans compte Nextendo</span>'
                 :'<span style="color:var(--faint)">identite non retenue (vote anterieur au suivi)</span>');
        h+='<tr>'
          +'<td><span class="fdot" style="background:'+(x.enLigne?'#2ee06a':'#4b5680')+'" title="'+(x.enLigne?'en ligne':'hors ligne')+'"></span>'
          +nom+'</td>'
          +'<td class="num">'+(x.pid||'—')+'</td>'
          +'<td style="color:'+col2+';font-weight:800">'+esc(nomDeCamp(f, x.camp))+'</td>'
          +'<td>'+esc(x.region||'—')+'</td>'
          +'<td>'+(x.vote?fdate(x.vote):'<span style="color:var(--faint)">avant le suivi</span>')+'</td>'
          +'<td style="color:var(--faint);font-size:11px">'+esc(x.uid)+'</td>'
          +'</tr>';
      }
      h+='</table></div>';
    }
    h+='</div>';

    /* --- baremes et cles --- */
    h+='<div class="fsec"><div class="ftit">'+ic('key')+'Barème et clés de déchiffrement</div><div class="fscroll"><table class="ftab">'
      +'<tr><th>Critère</th><th style="text-align:right">1<sup>er</sup></th><th style="text-align:right">2<sup>e</sup></th><th style="text-align:right">3<sup>e</sup></th></tr>';
    var bar=f.bareme||{};
    var crits=[['Yobisai','Vote préliminaire'],['PlayerCount','Popularité'],['Regular','Matchs ouverts'],['Challenge','Matchs défi'],['Tricolor','Match tricolore']];
    for(var n=0;n<crits.length;n++){
      h+='<tr><td>'+crits[n][1]+'</td>'
        +'<td class="num">'+(bar[crits[n][0]+'WinPoint']||0)+'</td>'
        +'<td class="num">'+(bar[crits[n][0]+'SecondPoint']||0)+'</td>'
        +'<td class="num">'+(bar[crits[n][0]+'ThirdPoint']||0)+'</td></tr>';
    }
    h+='</table></div>';
    h+='<div class="fnote">Clés servies à la console : <b>'+f.clesServies+'</b> sur 3. '
      +(f.vainqueur
        ? 'La troisième ouvre le paquet de victoire de <b style="color:'+fcoul(f.vainqueur)+'">'+esc(f.vainqueur)+'</b>.'
        : 'La troisième — celle du camp vainqueur — ne sera servie qu\'à l\'ouverture des résultats, comme chez Nintendo.')
      +'</div></div>';

    return h+'</div>';
  }
`
