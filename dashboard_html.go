package main

// Unified multi-game live-monitoring UI. Single page, no external deps, polls
// /api/stats every 2s. Go raw string (backticks) -> NO backticks inside; JS uses
// string concatenation, never template literals.
const dashboardHTML = `<!DOCTYPE html>
<html lang="fr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Nextendo — Monitoring serveurs en ligne</title>
<style>
  :root{ --bg:#06080f; --bg2:#0c1020; --card:#121830; --card2:#19203c; --line:#26304f; --line2:#33406b;
         --txt:#eef2ff; --muted:#8b97c4; --faint:#586089;
         --mk8:#ff3b4e; --s2:#2ee06a; --ssbu:#ff913b; --acnh:#4aa8e8; --mc:#5d8c3f; --arms:#ff4fa3; --mta:#ffd23f; --acc:#6aa6ff;
         --red:#ff3b4e; --blue:#3b8cff; --yellow:#ffce3a; --green:#2ee06a; --purple:#b06bff; --orange:#ff913b; --cyan:#2ad4e6; }
  *{ box-sizing:border-box; }
  ::-webkit-scrollbar{ width:10px; height:10px; } ::-webkit-scrollbar-thumb{ background:var(--line2); border-radius:6px; } ::-webkit-scrollbar-track{ background:transparent; }
  body{ margin:0; font-family:"Segoe UI",system-ui,-apple-system,sans-serif; color:var(--txt);
        background:radial-gradient(1200px 600px at 80% -10%, #1a2444 0%, rgba(6,8,15,0) 55%), radial-gradient(1000px 520px at 5% 0%, #271542 0%, rgba(6,8,15,0) 52%), var(--bg); min-height:100vh; }
  .ic{ width:1em; height:1em; vertical-align:-.16em; flex:none; } .ic.lg{ width:1.25em; height:1.25em; }
  header{ display:flex; align-items:center; gap:14px; padding:13px 24px; border-bottom:1px solid var(--line);
          position:sticky; top:0; background:rgba(6,8,15,.85); backdrop-filter:blur(12px); z-index:20; flex-wrap:wrap; }
  .brand{ display:flex; align-items:center; gap:11px; }
  .brand .mark{ width:38px; height:38px; border-radius:11px; display:grid; place-items:center; color:#fff; font-weight:800; font-size:19px;
                background:linear-gradient(135deg,#6aa6ff,#b06bff); box-shadow:0 6px 20px rgba(106,166,255,.4); }
  header h1{ font-size:16px; margin:0; font-weight:800; letter-spacing:.2px; } header h1 b{ background:linear-gradient(90deg,#6aa6ff,#b06bff); -webkit-background-clip:text; background-clip:text; color:transparent; }
  header h1 .sub{ display:block; font-size:11px; color:var(--muted); font-weight:500; letter-spacing:.3px; margin-top:1px; }
  .live{ display:inline-flex; align-items:center; gap:8px; font-size:12px; color:var(--green); font-weight:800; letter-spacing:.5px;
         background:rgba(46,224,106,.1); border:1px solid rgba(46,224,106,.28); padding:6px 12px; border-radius:30px; }
  .dot{ width:8px; height:8px; border-radius:50%; background:var(--green); animation:pulse 1.6s infinite; }
  .dot.off{ background:var(--red); animation:none; }
  @keyframes pulse{ 0%{box-shadow:0 0 0 0 rgba(46,224,106,.5)} 70%{box-shadow:0 0 0 8px rgba(46,224,106,0)} 100%{box-shadow:0 0 0 0 rgba(46,224,106,0)} }
  .spacer{ flex:1; } .clock{ color:var(--muted); font-size:12px; font-variant-numeric:tabular-nums; display:flex; align-items:center; gap:6px; } .clock b{ color:var(--txt); }
  /* tabs */
  .tabs{ display:flex; gap:6px; padding:10px 24px 0; flex-wrap:wrap; position:sticky; top:64px; background:rgba(6,8,15,.7); backdrop-filter:blur(8px); z-index:15; border-bottom:1px solid var(--line); }
  .tab{ display:inline-flex; align-items:center; gap:8px; padding:9px 15px; border-radius:11px 11px 0 0; cursor:pointer; font-size:13px; font-weight:700; color:var(--muted);
        border:1px solid transparent; border-bottom:none; transition:.15s; user-select:none; }
  .tab:hover{ color:var(--txt); background:rgba(255,255,255,.03); }
  .tab.on{ color:var(--txt); background:var(--card); border-color:var(--line); }
  .tab .gd{ width:8px; height:8px; border-radius:50%; background:var(--faint); } .tab .gd.up{ box-shadow:0 0 7px currentColor; }
  .tab .cc{ font-size:11px; font-weight:800; background:var(--bg2); border:1px solid var(--line); border-radius:20px; padding:1px 8px; font-variant-numeric:tabular-nums; }
  .tab .x{ font-size:11px; color:var(--faint); font-weight:600; }
  .tab .tabico{ width:19px; height:19px; border-radius:5px; object-fit:cover; }
  .tab .tabico.off{ filter:grayscale(1); opacity:.45; }
  main{ padding:20px 32px 70px; max-width:1800px; margin:0 auto; }
  .cards{ display:grid; grid-template-columns:repeat(auto-fit,minmax(150px,1fr)); gap:12px; margin-bottom:16px; }
  .stat{ position:relative; background:linear-gradient(160deg,var(--card2),var(--card)); border:1px solid var(--line); border-radius:15px; padding:14px 16px; overflow:hidden; transition:transform .15s,border-color .15s; }
  .stat:hover{ transform:translateY(-2px); border-color:var(--line2); }
  .stat .ico{ position:absolute; right:11px; top:11px; font-size:20px; opacity:.5; }
  .stat .n{ font-size:30px; font-weight:800; line-height:1; font-variant-numeric:tabular-nums; }
  .stat .l{ color:var(--muted); font-size:10.5px; text-transform:uppercase; letter-spacing:.7px; margin-top:8px; font-weight:600; }
  .stat.red .n,.stat.red .ico{color:var(--red)} .stat.blue .n,.stat.blue .ico{color:var(--blue)} .stat.yellow .n,.stat.yellow .ico{color:var(--yellow)}
  .stat.green .n,.stat.green .ico{color:var(--green)} .stat.purple .n,.stat.purple .ico{color:var(--purple)} .stat.cyan .n,.stat.cyan .ico{color:var(--cyan)} .stat.orange .n,.stat.orange .ico{color:var(--orange)}
  h2{ font-size:12.5px; color:var(--muted); text-transform:uppercase; letter-spacing:1.1px; margin:24px 0 12px; display:flex; align-items:center; gap:8px; font-weight:800; }
  h2 .ic{ color:var(--faint); } h2 .cnt{ color:var(--faint); font-weight:700; }
  .grid2{ display:grid; grid-template-columns:1.3fr 1fr; gap:18px; } @media(max-width:1000px){ .grid2{ grid-template-columns:1fr; } }
  .panel{ background:var(--card); border:1px solid var(--line); border-radius:16px; padding:15px 17px; }
  /* per-game summary strip */
  .gstrip{ display:grid; grid-template-columns:repeat(auto-fit,minmax(240px,1fr)); gap:13px; margin-bottom:6px; }
  .gcard{ position:relative; background:linear-gradient(160deg,var(--card2),var(--card)); border:1px solid var(--line); border-radius:16px; padding:15px 17px; cursor:pointer; overflow:hidden; transition:.15s; }
  .gcard:hover{ transform:translateY(-2px); border-color:var(--line2); }
  .gcard::before{ content:""; position:absolute; left:0; top:0; bottom:0; width:4px; background:var(--gc); }
  .gcard .gh{ display:flex; align-items:center; gap:10px; margin-bottom:12px; }
  .gcard .gn{ font-weight:800; font-size:15px; } .gcard .gs{ margin-left:auto; font-size:10.5px; font-weight:800; padding:3px 9px; border-radius:20px; }
  .gcard .gs.up{ background:rgba(46,224,106,.14); color:var(--green); } .gcard .gs.down{ background:rgba(255,59,78,.14); color:var(--red); }
  .gcard .grow{ display:flex; gap:18px; }
  .gcard .gm{ } .gcard .gm .v{ font-size:24px; font-weight:800; font-variant-numeric:tabular-nums; line-height:1; } .gcard .gm .k{ font-size:10px; color:var(--muted); text-transform:uppercase; letter-spacing:.6px; margin-top:4px; }
  /* manual power controls (start/stop an allowlisted game server) */
  .gcard .pwr{ display:flex; align-items:center; gap:8px; margin-top:12px; padding-top:11px; border-top:1px solid var(--line); cursor:default; }
  .gcard .pwrlbl{ font-size:10px; font-weight:800; text-transform:uppercase; letter-spacing:.5px; color:var(--muted); display:flex; align-items:center; gap:5px; margin-right:auto; }
  .gcard .pwrlbl.on{ color:var(--green); } .gcard .pwrlbl.off{ color:var(--red); }
  .pwbtn{ font-size:11px; font-weight:800; padding:5px 12px; border-radius:9px; border:1px solid var(--line2); background:var(--card2); color:var(--txt); cursor:pointer; transition:.12s; }
  .pwbtn.on:not(:disabled):hover{ border-color:var(--green); color:var(--green); }
  .pwbtn.off:not(:disabled):hover{ border-color:var(--red); color:var(--red); }
  .pwbtn:disabled{ opacity:.32; cursor:default; }
  .glogo{ width:34px; height:34px; border-radius:9px; position:relative; overflow:hidden; background:var(--gc); flex:0 0 auto; }
  .glogo .gll{ position:absolute; inset:0; display:grid; place-items:center; color:#fff; font-weight:800; }
  .glogo img{ position:absolute; inset:0; width:100%; height:100%; object-fit:cover; }
  /* chart */
  .chartwrap{ position:relative; height:200px; } .chartwrap svg{ width:100%; height:100%; display:block; }
  .legend{ display:flex; gap:16px; flex-wrap:wrap; font-size:12px; color:var(--muted); margin-top:6px; } .legend span{ display:inline-flex; align-items:center; gap:6px; } .legend i{ width:10px; height:10px; border-radius:3px; display:inline-block; }
  /* world map */
  .mapwrap{ position:relative; } .mapwrap svg{ width:100%; height:auto; display:block; border-radius:12px; background:linear-gradient(180deg,#0a1024,#0a0f1f); }
  .land{ fill:#1b2444; stroke:#2a3556; }
  .geolist{ display:flex; flex-direction:column; gap:7px; max-height:260px; overflow-y:auto; }
  .geolist .gr{ display:flex; align-items:center; gap:9px; font-size:13px; }
  .geolist .bar{ height:7px; border-radius:4px; background:linear-gradient(90deg,var(--acc),transparent 240%); min-width:4px; }
  .geolist .ct{ margin-left:auto; color:var(--muted); font-variant-numeric:tabular-nums; }
  /* lobbies */
  .lobbies{ display:grid; grid-template-columns:repeat(auto-fill,minmax(300px,1fr)); gap:14px; }
  .lobby{ position:relative; background:linear-gradient(160deg,var(--card2),var(--card)); border:1px solid var(--line); border-radius:15px; padding:14px 16px; overflow:hidden; }
  .lobby::before{ content:""; position:absolute; left:0; top:0; bottom:0; width:4px; background:var(--gc,#3b8cff); }
  .lobby .top{ display:flex; align-items:center; justify-content:space-between; margin-bottom:10px; }
  .lobby .gid{ font-weight:800; display:flex; align-items:center; gap:7px; }
  .modetag{ font-size:11px; font-weight:800; padding:3px 9px; border-radius:7px; background:rgba(106,166,255,.16); color:#9cc1ff; display:inline-flex; align-items:center; gap:5px; }
  .badge{ font-size:10.5px; font-weight:800; padding:4px 10px; border-radius:20px; display:inline-flex; align-items:center; gap:5px; }
  .badge.wait{ background:rgba(255,206,58,.14); color:var(--yellow); } .badge.match{ background:rgba(46,224,106,.14); color:var(--green); }
  .lobby .meta{ display:flex; gap:8px 15px; color:var(--muted); font-size:12px; margin-bottom:10px; flex-wrap:wrap; align-items:center; } .lobby .meta b{ color:var(--txt); } .lobby .meta span{ display:inline-flex; align-items:center; gap:5px; }
  .occ{ height:6px; border-radius:4px; background:var(--line); overflow:hidden; margin-bottom:10px; } .occ i{ display:block; height:100%; background:linear-gradient(90deg,var(--green),var(--cyan)); }
  .parts{ display:flex; flex-wrap:wrap; gap:7px; }
  .pill{ display:inline-flex; align-items:center; gap:7px; background:var(--bg2); border:1px solid var(--line); border-radius:30px; padding:3px 11px 3px 3px; font-size:12px; }
  .pill .vr{ color:var(--faint); font-variant-numeric:tabular-nums; }
  .pill .ppid{ color:var(--faint); font-size:10px; font-variant-numeric:tabular-nums; display:inline-flex; align-items:center; gap:1px; border-left:1px solid var(--line); padding-left:6px; margin-left:1px; }
  .pill .ppid .ic{ width:9px; height:9px; opacity:.6; }
  /* avatars */
  .av{ position:relative; display:inline-flex; align-items:center; justify-content:center; font-weight:800; color:#fff; flex:none; overflow:visible; }
  .av.s{ width:26px; height:26px; font-size:11px; } .av.m{ width:46px; height:46px; font-size:17px; }
  /* inner circle clips the image; the crown badge sits on the outer .av so overflow doesn't cut it */
  .avc{ position:absolute; inset:0; border-radius:50%; overflow:hidden; display:flex; align-items:center; justify-content:center; box-shadow:0 0 0 2px rgba(255,255,255,.07) inset; }
  .avc img{ width:100%; height:100%; object-fit:cover; }
  .av .cr{ position:absolute; right:-3px; top:-4px; width:15px; height:15px; background:var(--yellow); color:#3a2c00; border-radius:50%; display:grid; place-items:center; box-shadow:0 0 0 2px var(--card); z-index:2; }
  .av .cr .ic{ width:11px; height:11px; }
  .ic.crown{ color:var(--yellow); }
  /* player cards */
  .players{ display:grid; grid-template-columns:repeat(auto-fill,minmax(330px,1fr)); gap:14px; }
  .pcard{ background:linear-gradient(160deg,var(--card2),var(--card)); border:1px solid var(--line); border-radius:15px; padding:15px; transition:.15s; }
  .pcard:hover{ transform:translateY(-2px); border-color:var(--line2); }
  .pcard .hd{ display:flex; align-items:center; gap:12px; margin-bottom:12px; }
  .pcard .who{ min-width:0; flex:1; } .pcard .nm{ font-weight:800; font-size:15px; display:flex; align-items:center; gap:7px; }
  .pcard .nm .auto{ font-size:10px; color:var(--faint); font-weight:600; border:1px solid var(--line2); padding:1px 5px; border-radius:5px; }
  .pcard .sub{ color:var(--muted); font-size:12px; margin-top:3px; display:flex; align-items:center; gap:6px; flex-wrap:wrap; }
  .flag{ width:19px; height:14px; border-radius:2px; object-fit:cover; box-shadow:0 0 0 1px rgba(255,255,255,.12); vertical-align:-.18em; }
  .gtag{ font-size:9.5px; font-weight:800; padding:2px 7px; border-radius:6px; color:#fff; letter-spacing:.3px; }
  .st{ font-size:10.5px; font-weight:800; padding:4px 10px; border-radius:20px; display:inline-flex; align-items:center; gap:5px; flex:none; }
  .st.lobby{ background:rgba(59,140,255,.16); color:#7fb4ff; } .st.online{ background:rgba(139,151,196,.14); color:var(--muted); }
  .kv{ display:grid; grid-template-columns:1fr 1fr; gap:8px 10px; }
  .kv .k{ color:var(--faint); font-size:10px; text-transform:uppercase; letter-spacing:.5px; display:flex; align-items:center; gap:5px; margin-bottom:2px; }
  .kv .v{ font-size:13px; font-weight:600; font-variant-numeric:tabular-nums; display:flex; align-items:center; gap:6px; }
  .kv .full{ grid-column:1/3; }
  .ratetag{ font-size:11px; font-weight:800; color:var(--yellow); background:rgba(255,206,58,.13); padding:2px 8px; border-radius:6px; }
  /* feed + bars */
  .feed{ max-height:420px; overflow-y:auto; padding:3px 0; }
  .ev{ display:flex; align-items:center; gap:10px; padding:7px 6px; font-size:12.5px; border-bottom:1px solid rgba(38,48,79,.5); }
  .ev:last-child{ border-bottom:none; } .ev .t{ color:var(--faint); font-variant-numeric:tabular-nums; width:60px; flex:none; font-size:11px; }
  .ev .act{ font-weight:600; } .ev .pid{ color:var(--faint); margin-left:auto; font-variant-numeric:tabular-nums; font-size:11.5px; }
  .tag{ width:9px; height:9px; border-radius:3px; flex:none; }
  .bars .row{ display:flex; align-items:center; gap:11px; padding:5px 0; font-size:12px; }
  .bars .nm{ width:230px; flex:none; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .bars .bar{ height:9px; border-radius:5px; min-width:3px; transition:width .4s; } .bars .ct{ color:var(--muted); font-variant-numeric:tabular-nums; margin-left:auto; }
  .srv{ background:var(--card); border:1px solid var(--line); border-radius:13px; padding:11px 16px; display:flex; flex-wrap:wrap; gap:9px 20px; font-size:12px; color:var(--muted); margin-bottom:14px; align-items:center; }
  .srv span{ display:inline-flex; align-items:center; gap:6px; } .srv b{ color:var(--txt); font-variant-numeric:tabular-nums; } .srv .ic{ color:var(--faint); }
  .empty{ color:var(--muted); padding:24px; text-align:center; font-style:italic; background:var(--card); border:1px dashed var(--line2); border-radius:14px; }
  .mono{ font-variant-numeric:tabular-nums; }
  footer{ color:var(--faint); font-size:11.5px; margin-top:30px; line-height:1.7; border-top:1px solid var(--line); padding-top:14px; }
  .hidden{ display:none !important; }
</style>
</head>
<body>
<header>
  <div class="brand"><span class="mark">N</span><h1>Nextendo <b>Network</b><span class="sub">Monitoring live des serveurs en ligne privés</span></h1></div>
  <span class="live" id="live"><span class="dot" id="dot"></span><span id="livetxt">connexion…</span></span>
  <span class="spacer"></span>
  <span class="clock"><span id="i_clock"></span>maj <b id="lastupd">—</b> · uptime <b id="uptime">—</b></span>
</header>
<div class="tabs" id="tabs"></div>
<main>
  <div id="view_overview"></div>
  <div id="view_game" class="hidden"></div>
  <footer id="foot"></footer>
</main>
<script>
  var KEY = location.search || '';
  var TAB = 'overview';
  var GAMECOLORS = { mk8:'#ff3b4e', s2:'#2ee06a', ssbu:'#ff913b', acnh:'#4aa8e8', mc:'#5d8c3f', arms:'#ff4fa3', mta:'#ffd23f' };
  // correct per-game identity (the shared MK8 dashboard mislabels S2/SSBU host/port/NEX)
  var GAMEINFO = {
    mk8:{ nex:'4.3.3', sni:'the-game-host', port:60003 },
    s2:{  nex:'4.0.0', sni:'the-game-host', port:60004 },
    ssbu:{nex:'4.6.3', sni:'the-game-host', port:60005 },
    acnh:{nex:'4.0.0', sni:'the-game-host', port:60007 },
    mc:{  nex:'4.3.1', sni:'the-game-host', port:60006 },
    arms:{nex:'4.3.5', sni:'the-game-host', port:60006 },
    mta:{ nex:'4.3.5', sni:'the-game-host', port:60010 }
  };
  // Real game mode per lobby. S2 encodes it in the MatchmakeSession game_mode (+ maxParticipants
  // and the private-room code): mode 1 = Ranked, mode 0 = Turf War (confirmed in-game), a
  // 4-player cap = Salmon Run, a room code = a private battle. MK8 already sends a readable
  // label; SSBU/ACNH stay generic.
  function modeInfo(key,lo){
    if(key==='mk8'){
      if(!lo) return {name:'—', col:'#9cc1ff', bg:'rgba(106,166,255,.16)', icn:'flag'};
      if(lo.code) return {name:'Privée', col:'#c9d2f0', bg:'rgba(139,151,196,.20)', icn:'lock'};
      // MK8DX game_mode: 1=Course / 3=Bataille (mondial), 2=Course / 4=Bataille (régional).
      // Mondial vs régional déduit du game_mode (à confirmer en jeu).
      var battle=(lo.mode===3||lo.mode===4);
      var type=(lo.mode===1||lo.mode===2)?'Course':battle?'Bataille':('Mode '+(lo.mode==null?'?':lo.mode));
      var scope=(lo.mode===1||lo.mode===3)?'Mondial':(lo.mode===2||lo.mode===4)?'Régional':'';
      var col=scope==='Régional'?'#c19bff':'#7fb4ff', bg=scope==='Régional'?'rgba(176,107,255,.18)':'rgba(59,140,255,.16)';
      var icn=scope==='Régional'?'map':scope==='Mondial'?'globe':(battle?'swords':'flag');
      return {name:(scope?scope+' · ':'')+type, col:col, bg:bg, icn:icn};
    }
    if(key==='s2'&&lo){
      // S2 private battles carry no room-code; they're the only mode that seats 10 (8 players + 2 spectators).
      if(lo.code||lo.max===10) return {name:'Privée',     col:'#c9d2f0', bg:'rgba(139,151,196,.20)', icn:'lock'};
      if((lo.max||8)<=4)   return {name:'Salmon Run', col:'#ff9e5c', bg:'rgba(255,122,61,.20)',  icn:'fish'};
      if(lo.mode===1)      return {name:'Ranked',     col:'#ffbf5a', bg:'rgba(255,159,59,.18)',  icn:'target'};
      if(lo.mode===0)      return {name:'Turf War',   col:'#5fe08c', bg:'rgba(46,224,106,.18)',  icn:'flag'};
      if(lo.mode===2)      return {name:'League',     col:'#c19bff', bg:'rgba(176,107,255,.20)', icn:'swords'};
      return {name:'Mode '+(lo.mode==null?'?':lo.mode), col:'#9cc1ff', bg:'rgba(106,166,255,.16)', icn:'flag'};
    }
    return {name:'Partie en ligne', col:'#9cc1ff', bg:'rgba(106,166,255,.16)', icn:'flag'};
  }
  function modeBadge(key,lo){ var m=modeInfo(key,lo); return '<span class="modetag" style="background:'+m.bg+';color:'+m.col+'">'+ic(m.icn)+esc(m.name)+'</span>'; }
  var LAST = null;
  // country -> [lat, lon] centroid (equirectangular plot)
  var GEO = { FR:[46.6,2.4], BE:[50.5,4.5], DE:[51,10.5], ES:[40.2,-3.7], IT:[42.8,12.6], NL:[52.1,5.3], GB:[54,-2], PT:[39.6,-8],
    US:[39.8,-98.6], CA:[56.1,-106], MX:[23.6,-102], BR:[-10.3,-53], AR:[-38,-63],
    JP:[36.2,138.3], KR:[36.5,127.8], CN:[35.9,104.2], AU:[-25.3,133.8], IN:[22,79], RU:[61.5,105] };
  var P = {
    users:'<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75"/>',
    user:'<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>',
    usercheck:'<path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="8.5" cy="7" r="4"/><polyline points="17 11 19 13 23 9"/>',
    trend:'<polyline points="23 6 13.5 15.5 8.5 10.5 1 18"/><polyline points="17 6 23 6 23 12"/>',
    layers:'<polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/>',
    activity:'<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>',
    clock:'<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>',
    plus:'<circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="16"/><line x1="8" y1="12" x2="16" y2="12"/>',
    pin:'<path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/>',
    zap:'<polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>',
    crown:'<path d="M3 7 8 11 12 5 16 11 21 7 19 17 5 17Z"/>',
    lock:'<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
    fish:'<path d="M2 12c3-5 8-6 12-6 3 0 5 1 6 2-1 1-3 2-6 2-4 0-9-1-12 2zm0 0c3 5 8 6 12 6 3 0 5-1 6-2"/><circle cx="7" cy="11" r="1"/>',
    swords:'<polyline points="14 4 20 4 20 10"/><line x1="20" y1="4" x2="4" y2="20"/><polyline points="10 20 4 20 4 14"/>',
    target:'<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1.5"/>',
    check:'<path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/>',
    search:'<circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>',
    globe:'<circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>',
    hash:'<line x1="4" y1="9" x2="20" y2="9"/><line x1="4" y1="15" x2="20" y2="15"/><line x1="10" y1="3" x2="8" y2="21"/><line x1="16" y1="3" x2="14" y2="21"/>',
    net:'<rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/>',
    gauge:'<path d="M12 14l4-4"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/>',
    bolt:'<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>',
    key:'<path d="M21 2l-2 2m-7.6 7.6a5.5 5.5 0 1 1-7.8 7.8 5.5 5.5 0 0 1 7.8-7.8zm0 0L15.5 7.5l3 3L22 7l-3-3"/>',
    server:'<rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/>',
    timer:'<circle cx="12" cy="13" r="8"/><path d="M12 9v4l2 2M9 2h6"/>',
    map:'<polygon points="1 6 1 22 8 18 16 22 23 18 23 2 16 6 8 2 1 6"/><line x1="8" y1="2" x2="8" y2="18"/><line x1="16" y1="6" x2="16" y2="22"/>',
    flag:'<path d="M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1z"/><line x1="4" y1="22" x2="4" y2="15"/>',
    power:'<path d="M18.36 6.64a9 9 0 1 1-12.73 0"/><line x1="12" y1="2" x2="12" y2="12"/>'
  };
  function ic(n,cls){ return '<svg class="ic '+(cls||'')+'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">'+(P[n]||'')+'</svg>'; }
  function icf(n,cls){ return '<svg class="ic '+(cls||'')+'" viewBox="0 0 24 24" fill="currentColor" stroke="none">'+(P[n]||'')+'</svg>'; }
  function fmtDur(s){ if(s<0)s=0; var h=Math.floor(s/3600),m=Math.floor((s%3600)/60),x=Math.floor(s%60); if(h>0)return h+'h '+m+'m'; if(m>0)return m+'m '+x+'s'; return x+'s'; }
  function fmtN(n){ n=n||0; if(n>=1e6)return (n/1e6).toFixed(1)+'M'; if(n>=1e4)return (n/1e3).toFixed(1)+'k'; return ''+n; }
  function esc(s){ return String(s==null?'':s).replace(/[&<>"]/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c];}); }
  function flag(cc){ if(!cc||cc.length!==2)return ''; return '<img class="flag" src="https://flagcdn.com/'+cc.toLowerCase()+'.svg" alt="'+esc(cc)+'" onerror="this.replaceWith(document.createTextNode(\''+esc(cc)+'\'))">'; }
  // resolved name/avatar: the account server's pseudo+image (LAST.names[pid]) overrides the
  // game's "Joueur-<pid>" fallback for games (S2/SSBU) that don't upload a readable name.
  function rn(p){ var n=LAST&&LAST.names&&p&&LAST.names[''+p.pid]; return (n&&n.name)||(p&&p.name)||''; }
  function ri(p){ var n=LAST&&LAST.names&&p&&LAST.names[''+p.pid]; return (n&&n.image)||(p&&p.miiUrl)||''; }
  function av(p,size){ var cls=size||'s';
    // Initiale colorée TOUJOURS en dessous ; la vraie pp (si le compte en a une) la recouvre.
    // Si l'image 404/échoue, onerror la masque et l'initiale réapparaît (jamais d'avatar vide).
    var pid=(p&&p.pid)||0, mc=p&&p.miiColor, bg;
    if(mc){ bg='linear-gradient(135deg,'+mc+',rgba(0,0,0,.45))'; }
    else{ var h1=(pid*47)%360,h2=(h1+38)%360; bg='linear-gradient(135deg,hsl('+h1+',62%,52%),hsl('+h2+',62%,40%))'; }
    var ch=(rn(p)||'?').trim().charAt(0).toUpperCase()||'?';
    var base='<span style="position:absolute;inset:0;background:'+bg+';display:grid;place-items:center">'+esc(ch)+'</span>';
    var img=ri(p);
    var pic=img?'<img src="'+esc(img)+'" alt="" style="position:absolute;inset:0" onerror="this.style.display=\'none\'">':'';
    var crown=(p&&(p.host||p.isHost))?'<span class="cr">'+icf('crown')+'</span>':'';
    return '<span class="av '+cls+'"><span class="avc">'+base+pic+'</span>'+crown+'</span>'; }
  function protoColor(a){ var p=(a||'').split('::')[0]; var m={TicketGranting:'#b06bff',SecureConnection:'#3b8cff',Utility:'#ffce3a',Ranking:'#ff3b4e',DataStore:'#ff3b4e',MatchmakeExt:'#2ee06a',MatchMaking:'#2ee06a',MatchMakingExt:'#2ee06a',NATTraversal:'#ff913b',Notifications:'#2ad4e6'}; return m[p]||'#8b97c4'; }
  function setStatus(ok){ var d=document.getElementById('dot'),t=document.getElementById('livetxt'); if(ok){d.className='dot';t.textContent='EN DIRECT';t.parentNode.style.color='var(--green)';}else{d.className='dot off';t.textContent='injoignable';t.parentNode.style.color='var(--red)';} }
  document.getElementById('i_clock').innerHTML=ic('clock');

  function gameOf(d,key){ for(var i=0;i<d.games.length;i++) if(d.games[i].key===key) return d.games[i]; return null; }
  // manual power controls: render start/stop buttons for allowlisted servers, and POST the action.
  function pw(gm){
    if(!gm||!gm.controllable) return '';
    var on=gm.running;
    return '<div class="pwr" onclick="event.stopPropagation()">'+
      '<span class="pwrlbl '+(on?'on':'off')+'">'+ic('power')+(on?'Allumé':'Éteint')+'</span>'+
      '<button class="pwbtn on" '+(on?'disabled':'')+' onclick="powerGame(\''+gm.key+'\',\'start\')">Allumer</button>'+
      '<button class="pwbtn off" '+(on?'':'disabled')+' onclick="powerGame(\''+gm.key+'\',\'stop\')">Éteindre</button>'+
    '</div>';
  }
  function powerGame(key,action){
    var g=LAST&&gameOf(LAST,key); var label=(g&&g.label)||key;
    if(action==='stop' && !confirm('Éteindre le serveur '+label+' ?\nLes joueurs connectés seront déconnectés.')) return;
    var url='/api/power'+KEY+(KEY?'&':'?')+'game='+encodeURIComponent(key)+'&action='+action;
    fetch(url,{method:'POST'})
      .then(function(r){ return r.json().catch(function(){ return {ok:r.ok}; }); })
      .then(function(j){ if(!j||!j.ok){ alert('Échec : '+((j&&j.error)||'commande refusée')); } })
      .catch(function(){ alert('Requête échouée'); });
  }
  function rateName(key){ return {mk8:'VR',s2:'Puissance',ssbu:'PSP'}[key]||'Score'; }

  // ---------- tabs ----------
  function renderTabs(d){
    var h='<div class="tab '+(TAB==='overview'?'on':'')+'" onclick="go(\'overview\')">'+ic('layers')+'Vue d\'ensemble</div>';
    for(var i=0;i<d.games.length;i++){ var g=d.games[i]; var st=g.stats||{}; var on=g.online;
      h+='<div class="tab '+(TAB===g.key?'on':'')+'" onclick="go(\''+g.key+'\')" style="--gc:'+g.color+'">'+
         '<img class="tabico'+(on?'':' off')+'" src="gameicons/'+g.key+'.jpg" alt="" onerror="this.outerHTML=\'<span class=\\\'gd '+(on?'up':'')+'\\\' style=\\\'background:'+(on?g.color:'')+';color:'+g.color+'\\\'></span>\'">'+
         esc(g.label)+'<span class="cc">'+(on?(st.connected||0):'·')+'</span>'+(on?'':'<span class="x">hors-ligne</span>')+'</div>'; }
    document.getElementById('tabs').innerHTML=h;
  }
  function go(t){ TAB=t; if(LAST) draw(LAST); }

  // ---------- overview ----------
  function statCard(cls,icn,val,label){ return '<div class="stat '+cls+'"><span class="ico">'+ic(icn)+'</span><div class="n">'+val+'</div><div class="l">'+label+'</div></div>'; }
  function renderOverview(d){
    var g=d.global;
    var h='<div class="cards">'+
      statCard('blue','users',g.connected,'Joueurs en ligne')+
      statCard('green','usercheck',g.inLobby,'En lobby')+
      statCard('yellow','layers',g.activeLobbies,'Lobbies actifs')+
      statCard('cyan','activity',fmtN(g.totalRmc),'Appels RMC')+
      statCard('purple','trend',g.peak,'Pic connectés')+
      statCard('orange','power',g.gamesOnline+'/'+d.games.length,'Serveurs en ligne')+'</div>';
    // per-game strip
    h+='<div class="gstrip">';
    for(var i=0;i<d.games.length;i++){ var gm=d.games[i]; var s=gm.stats||{};
      h+='<div class="gcard" style="--gc:'+gm.color+'" onclick="go(\''+gm.key+'\')">'+
        '<div class="gh"><span class="glogo" style="--gc:'+gm.color+'"><span class="gll">'+esc(gm.label.charAt(0))+'</span><img src="gameicons/'+gm.key+'.jpg" alt="" onerror="this.style.display=\'none\'"></span><span class="gn">'+esc(gm.label)+'</span>'+
        '<span class="gs '+(gm.online?'up':'down')+'">'+(gm.online?'EN LIGNE':'HORS-LIGNE')+'</span></div>'+
        '<div class="grow">'+
        '<div class="gm"><div class="v" style="color:'+gm.color+'">'+(gm.online?(s.connected||0):'—')+'</div><div class="k">connectés</div></div>'+
        '<div class="gm"><div class="v">'+(gm.online?(s.peakConnected||0):'—')+'</div><div class="k">pic</div></div>'+
        '<div class="gm"><div class="v">'+(gm.online?(s.activeLobbies||0):'—')+'</div><div class="k">lobbies</div></div>'+
        '<div class="gm"><div class="v">'+(gm.online?fmtN(s.totalRmc):'—')+'</div><div class="k">RMC</div></div>'+
        '</div>'+pw(gm)+'</div>';
    }
    h+='</div>';
    // chart + map row
    h+='<div class="grid2">'+
      '<div><h2>'+ic('trend')+'Connexions en temps réel</h2><div class="panel"><div class="chartwrap" id="chart"></div>'+
      '<div class="legend"><span><i style="background:var(--mk8)"></i>Mario Kart 8</span><span><i style="background:var(--s2)"></i>Splatoon 2</span><span><i style="background:var(--ssbu)"></i>Smash Ultimate</span><span><i style="background:var(--acnh)"></i>Animal Crossing</span><span><i style="background:var(--mc)"></i>Minecraft</span><span><i style="background:var(--arms)"></i>ARMS</span><span><i style="background:var(--mta)"></i>Mario Tennis Aces</span></div></div></div>'+
      '<div><h2>'+ic('globe')+'Joueurs par pays</h2><div class="panel"><div class="mapwrap" id="map"></div><div class="geolist" id="geolist" style="margin-top:12px"></div></div></div>'+
    '</div>';
    // unified feed
    h+='<h2>'+ic('activity')+'Activité réseau (tous jeux)</h2><div class="panel feed" id="ufeed"></div>';
    document.getElementById('view_overview').innerHTML=h;
    document.getElementById('view_overview').classList.remove('hidden');
    document.getElementById('view_game').classList.add('hidden');
    drawChart(d); drawMap(d); drawUnifiedFeed(d);
  }

  function drawChart(d){
    var hi=d.history||[]; var el=document.getElementById('chart'); if(!el) return;
    var W=600,H=200,pad=24; var keys=['mk8','s2','ssbu','acnh','mc','arms','mta'];
    if(hi.length<2){ el.innerHTML='<div class="empty" style="height:100%;display:grid;place-items:center">Collecte des données…</div>'; return; }
    var maxv=1; for(var i=0;i<hi.length;i++){ var t=0; for(var k=0;k<keys.length;k++) t+=(hi[i].conn&&hi[i].conn[keys[k]])||0; if(t>maxv)maxv=t; }
    maxv=Math.ceil(maxv*1.2);
    var x=function(i){ return pad+(W-2*pad)*i/(hi.length-1); };
    var y=function(v){ return H-pad-(H-2*pad)*v/maxv; };
    var svg='<svg viewBox="0 0 '+W+' '+H+'" preserveAspectRatio="none">';
    for(var gl=0;gl<=4;gl++){ var yy=pad+(H-2*pad)*gl/4; svg+='<line x1="'+pad+'" y1="'+yy+'" x2="'+(W-pad)+'" y2="'+yy+'" stroke="#26304f" stroke-width="1"/>'; svg+='<text x="'+(pad-4)+'" y="'+(yy+3)+'" fill="#586089" font-size="9" text-anchor="end">'+Math.round(maxv*(4-gl)/4)+'</text>'; }
    var cum=[]; for(var i=0;i<hi.length;i++) cum[i]=0;
    var cols={mk8:'#ff3b4e',s2:'#2ee06a',ssbu:'#ff913b',acnh:'#4aa8e8',mc:'#5d8c3f',arms:'#ff4fa3',mta:'#ffd23f'};
    for(var k=0;k<keys.length;k++){ var kk=keys[k]; var area='M '+pad+' '+y(0); var top='';
      for(var i=0;i<hi.length;i++){ var v=(hi[i].conn&&hi[i].conn[kk])||0; var below=cum[i]; cum[i]+=v; top+=(i?' L ':'')+x(i).toFixed(1)+' '+y(cum[i]).toFixed(1); }
      // area between prev cum and new cum
      var ar='M'; var prev=[]; for(var i=0;i<hi.length;i++){ prev[i]=cum[i]-((hi[i].conn&&hi[i].conn[kk])||0); }
      ar=''; for(var i=0;i<hi.length;i++){ ar+=(i?' L ':'M ')+x(i).toFixed(1)+' '+y(cum[i]).toFixed(1); }
      for(var i=hi.length-1;i>=0;i--){ ar+=' L '+x(i).toFixed(1)+' '+y(prev[i]).toFixed(1); }
      ar+=' Z';
      svg+='<path d="'+ar+'" fill="'+cols[kk]+'" opacity="0.16"/>';
      svg+='<path d="'+top+'" fill="none" stroke="'+cols[kk]+'" stroke-width="2" stroke-linejoin="round"/>';
    }
    svg+='</svg>'; el.innerHTML=svg;
  }

  function drawMap(d){
    var el=document.getElementById('map'); if(!el) return;
    var W=560,H=300;
    // gather players across games with geo
    var pts=[]; var byCC={};
    for(var i=0;i<d.games.length;i++){ var gm=d.games[i]; if(!gm.online||!gm.stats) continue; var pls=gm.stats.players||[];
      for(var j=0;j<pls.length;j++){ var p=pls[j]; if(!p.cc||!GEO[p.cc]) continue;
        var ll=GEO[p.cc]; var px=(ll[1]+180)/360*W; var py=(90-ll[0])/180*H;
        pts.push({x:px,y:py,c:gm.color,cc:p.cc}); byCC[p.cc]=(byCC[p.cc]||0)+1; } }
    // real equirectangular world map (NASA blue-marble + relief; full -180..180/90..-90,
    // so the plate-carree lon/lat dot positions align exactly), dimmed for the dark theme
    var MAPURL='/worldmap.jpg';
    var svg='<svg viewBox="0 0 '+W+' '+H+'">'+
      '<image href="'+MAPURL+'" x="0" y="0" width="'+W+'" height="'+H+'" preserveAspectRatio="none" opacity="0.62"/>'+
      '<rect x="0" y="0" width="'+W+'" height="'+H+'" fill="#0a1024" opacity="0.42"/>';
    for(var i=0;i<pts.length;i++){ var p=pts[i];
      svg+='<circle cx="'+p.x.toFixed(1)+'" cy="'+p.y.toFixed(1)+'" r="9" fill="'+p.c+'" opacity="0.18"/>'+
           '<circle cx="'+p.x.toFixed(1)+'" cy="'+p.y.toFixed(1)+'" r="3.5" fill="'+p.c+'"><animate attributeName="opacity" values="1;.5;1" dur="2s" repeatCount="indefinite"/></circle>'; }
    svg+='</svg>'; el.innerHTML=svg;
    // geo list
    var arr=Object.keys(byCC).map(function(cc){return {cc:cc,n:byCC[cc]};}).sort(function(a,b){return b.n-a.n;});
    var mx=arr.length?arr[0].n:1; var gl='';
    for(var i=0;i<arr.length;i++){ gl+='<div class="gr">'+flag(arr[i].cc)+'<span style="width:34px">'+esc(arr[i].cc)+'</span><span class="bar" style="width:'+Math.max(4,160*arr[i].n/mx)+'px"></span><span class="ct">'+arr[i].n+'</span></div>'; }
    var gle=document.getElementById('geolist'); if(gle) gle.innerHTML=arr.length?gl:'<div class="empty">Aucun joueur géolocalisé.</div>';
  }

  function drawUnifiedFeed(d){
    var all=[];
    for(var i=0;i<d.games.length;i++){ var gm=d.games[i]; if(!gm.stats) continue; var evs=gm.stats.events||[];
      for(var j=0;j<evs.length;j++) all.push({ago:evs[j].agoSeconds,pid:evs[j].pid,act:evs[j].action,game:gm.key,color:gm.color,label:gm.label}); }
    all.sort(function(a,b){return a.ago-b.ago;});
    var el=document.getElementById('ufeed'); if(!el) return;
    if(!all.length){ el.innerHTML='<div class="empty">Aucune activité récente.</div>'; return; }
    var h=''; for(var i=0;i<Math.min(all.length,60);i++){ var x=all[i];
      h+='<div class="ev"><span class="t">'+x.ago+'s</span><span class="gtag" style="background:'+x.color+'">'+esc(x.game.toUpperCase())+'</span>'+
         '<span class="tag" style="background:'+protoColor(x.act)+'"></span><span class="act">'+esc(x.act)+'</span><span class="pid">'+ic('hash')+x.pid+'</span></div>'; }
    el.innerHTML=h;
  }

  // ---------- per-game view ----------
  function renderGame(d,key){
    var gm=gameOf(d,key); if(!gm){ go('overview'); return; }
    document.getElementById('view_overview').classList.add('hidden');
    var gv=document.getElementById('view_game'); gv.classList.remove('hidden');
    if(!gm.online||!gm.stats){ gv.innerHTML='<div class="empty" style="margin-top:20px">'+ic('power')+' Serveur '+esc(gm.label)+' hors-ligne.</div>'; return; }
    var s=gm.stats; var sv=s.server||{}; var gi=GAMEINFO[key]||{};
    var h='<div class="srv" style="border-left:3px solid '+gm.color+'">'+
      '<span class="glogo" style="--gc:'+gm.color+';width:26px;height:26px;border-radius:7px"><span class="gll" style="font-size:12px">'+esc(gm.label.charAt(0))+'</span><img src="gameicons/'+gm.key+'.jpg" alt="" onerror="this.style.display=\'none\'"></span>'+
      '<span style="color:'+gm.color+';font-weight:800">'+esc(gm.label)+'</span>'+
      '<span>'+ic('key')+'Clé <b>'+esc(sv.accessKey)+'</b></span>'+
      '<span>'+ic('net')+'Secure <b>:'+esc(gi.port||sv.securePort)+'</b></span>'+
      '<span>'+ic('server')+'NEX <b>'+esc(gi.nex||sv.nexVersion)+'</b></span>'+
      '<span>'+ic('globe')+'Hôte <b>'+esc(gi.sni||sv.sniHost)+'</b></span>'+
      '<span>'+ic('timer')+'Uptime <b>'+fmtDur(s.uptimeSeconds)+'</b></span></div>';
    h+='<div class="cards">'+
      statCard('blue','users',s.connected,'Connectés')+statCard('green','usercheck',s.inLobby,'En lobby')+
      statCard('yellow','layers',s.activeLobbies,'Lobbies')+statCard('cyan','activity',fmtN(s.totalRmc),'Appels RMC')+
      statCard('purple','trend',s.peakConnected,'Pic')+statCard('orange','plus',s.gatheringsMade,'Lobbies créés')+'</div>';
    // lobbies
    h+='<h2>'+ic('layers')+'Lobbies <span class="cnt">'+((s.gatherings||[]).length)+'</span></h2>';
    if(!s.gatherings||!s.gatherings.length){ h+='<div class="empty">Aucun lobby actif.</div>'; }
    else{ h+='<div class="lobbies">'; for(var i=0;i<s.gatherings.length;i++){ var lo=s.gatherings[i]; var bcls=lo.count>=2?'match':'wait'; var pct=Math.min(100,Math.round(100*lo.count/(lo.max||8)));
      h+='<div class="lobby" style="--gc:'+((key==='s2'||key==='mk8')?modeInfo(key,lo).col:gm.color)+'"><div class="top"><span class="gid">'+ic('hash')+'Lobby '+lo.id+' '+modeBadge(key,lo)+'</span><span class="badge '+bcls+'">'+ic(lo.count>=2?'check':'search')+esc(lo.state)+'</span></div>';
      h+='<div class="meta"><span>'+ic('users')+'<b>'+lo.count+'</b>/'+lo.max+'</span>'+(lo.vr&&key==='mk8'?'<span class="ratetag">'+rateName(key)+' '+lo.vr+'</span>':'')+'<span>'+ic('crown','crown')+'<b>'+esc(rn({pid:lo.hostPid,name:lo.hostName}))+'</b></span></div>';
      h+='<div class="occ"><i style="width:'+pct+'%"></i></div><div class="parts">';
      var pls=lo.players||[]; for(var j=0;j<pls.length;j++){ var p=pls[j]; h+='<span class="pill">'+av(p)+'<b>'+esc(rn(p))+'</b><span class="ppid">'+ic('hash')+p.pid+'</span>'+(p.vr&&key==='mk8'?'<span class="vr">'+p.vr+'</span>':'')+'</span>'; }
      h+='</div></div>'; } h+='</div>'; }
    // players
    h+='<h2>'+ic('users')+'Joueurs <span class="cnt">'+((s.players||[]).length)+'</span></h2>';
    if(!s.players||!s.players.length){ h+='<div class="empty">Personne de connecté.</div>'; }
    else{ var gmap={}; for(var gi2=0;gi2<(s.gatherings||[]).length;gi2++){ gmap[s.gatherings[gi2].id]=s.gatherings[gi2]; }
      h+='<div class="players">'; for(var k=0;k<s.players.length;k++){ var pl=s.players[k]; var scls=pl.gathering?'lobby':'online';
      var loc=pl.country?(flag(pl.cc)+' '+esc(pl.country)+(pl.city?(' · '+esc(pl.city)):'')):'<span style="color:var(--faint)">localisation…</span>';
      h+='<div class="pcard"><div class="hd">'+av(pl,'m')+'<div class="who"><div class="nm">'+esc(rn(pl))+(rn(pl).indexOf('Joueur-')===0?' <span class="auto">auto</span>':'')+(pl.isHost?ic('crown','crown'):'')+'</div><div class="sub">'+ic('hash')+'<span class="mono">'+pl.pid+'</span></div></div><span class="st '+scls+'">'+ic(pl.gathering?'usercheck':'user')+esc(pl.state)+'</span></div><div class="kv">';
      var plo=pl.gathering?gmap[pl.gathering]:null;
      h+='<div><div class="k">'+ic('flag')+'Mode</div><div class="v">'+(plo?modeBadge(key,plo):(key==='mk8'&&pl.mode?esc(pl.mode):'<span style="color:var(--faint)">—</span>'))+'</div></div>';
      if(key==='mk8'){ h+='<div><div class="k">'+ic('gauge')+rateName(key)+'</div><div class="v">'+(pl.vr?'<span class="ratetag">'+pl.vr+'</span>':'—')+'</div></div>'; }
      else{ h+='<div><div class="k">'+ic('net')+'NAT</div><div class="v">'+(pl.natType?esc(pl.natType):'—')+'</div></div>'+
            '<div><div class="k">'+ic('gauge')+'Ping</div><div class="v">'+(pl.ping?pl.ping+' ms':'—')+'</div></div>'; }
      h+='<div class="full"><div class="k">'+ic('pin')+'Localisation</div><div class="v">'+loc+(pl.isp?(' <span style="color:var(--faint);font-weight:500">· '+esc(pl.isp)+'</span>'):'')+'</div></div>';
      h+='<div><div class="k">'+ic('net')+'IP</div><div class="v mono" style="font-size:12px">'+esc(pl.ip||'—')+'</div></div>';
      h+='<div><div class="k">'+ic('layers')+'Lobby</div><div class="v mono">'+(pl.gathering?('#'+pl.gathering+(pl.isHost?' (hôte)':'')):'—')+'</div></div>';
      h+='<div><div class="k">'+ic('timer')+'En ligne</div><div class="v">'+fmtDur(pl.onlineSeconds)+'</div></div>';
      h+='<div><div class="k">'+ic('clock')+'Inactif</div><div class="v">'+fmtDur(pl.idleSeconds)+'</div></div>';
      h+='<div class="full"><div class="k">'+ic('activity')+'Dernière action · '+pl.calls+' appels</div><div class="v" style="font-size:12px;color:var(--muted)">'+esc(pl.lastAction||'—')+'</div></div>';
      h+='</div></div>'; } h+='</div>'; }
    // feed + methods
    h+='<div class="grid2"><div><h2>'+ic('activity')+'Activité en direct</h2><div class="panel feed" id="gfeed"></div></div>'+
       '<div><h2>'+ic('bolt')+'Trafic par méthode</h2><div class="panel bars" id="gbars"></div></div></div>';
    gv.innerHTML=h;
    // feed
    var ev=s.events||[]; var fe=document.getElementById('gfeed');
    if(!ev.length){ fe.innerHTML='<div class="empty">Aucune activité.</div>'; }
    else{ var f=''; for(var e=0;e<ev.length;e++){ var x=ev[e]; f+='<div class="ev"><span class="t">'+x.agoSeconds+'s</span><span class="tag" style="background:'+protoColor(x.action)+'"></span><span class="act">'+esc(x.action)+'</span><span class="pid">'+ic('hash')+x.pid+'</span></div>'; } fe.innerHTML=f; }
    var ms=s.methods||[]; var bw=document.getElementById('gbars');
    if(!ms.length){ bw.innerHTML='<div class="empty">Aucun appel.</div>'; }
    else{ var mx=ms[0].count||1; var b=''; for(var mi=0;mi<ms.length;mi++){ var mm=ms[mi]; var w=Math.max(3,Math.round(220*mm.count/mx)); var col=protoColor(mm.name); b+='<div class="row"><span class="nm">'+esc(mm.name)+'</span><span class="bar" style="width:'+w+'px;background:linear-gradient(90deg,'+col+',transparent 220%)"></span><span class="ct">'+mm.count+'</span></div>'; } bw.innerHTML=b; }
  }

  function draw(d){
    LAST=d; renderTabs(d);
    document.getElementById('lastupd').textContent=new Date().toLocaleTimeString();
    document.getElementById('uptime').textContent=fmtDur(d.uptimeSeconds);
    if(TAB==='overview') renderOverview(d); else renderGame(d,TAB);
  }
  document.getElementById('foot').innerHTML=ic('globe')+' Agrégation live des 7 serveurs NEX privés (VPS), rafraîchie toutes les 2 s · géoloc ip-api.com · drapeaux flagcdn.com · rendu Mii Studio Nintendo.';
  function tick(){ fetch('/api/stats'+KEY,{cache:'no-store'}).then(function(r){return r.json();}).then(function(d){ setStatus(true); draw(d); }).catch(function(e){ setStatus(false); }); }
  tick(); setInterval(tick,2000);
</script>
</body>
</html>`
