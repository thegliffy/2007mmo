(() => {
  const $ = (id) => document.getElementById(id);
  const canvas = $("stage");
  const ctx = canvas.getContext("2d");

  const KEYS = {
    playerId: "hollowmere.playerId",
    name: "hollowmere.name",
    session: "hollowmere.session",
  };

  const state = {
    ws: null,
    tickMs: 600,
    world: "hollowmere",
    map: null,
    you: null,
    players: [],
    npcs: [],
    nodes: [],
    items: {},
    last: null,
    prevPos: {},
    lastTickAt: 0,
    n: 0,
    online: 0,
    lastMs: 0,
    reconnects: 0,
    wantJoin: false,
    dead: false,
  };

  function wsURL() {
    const q = new URLSearchParams(location.search);
    if (q.get("ws")) return q.get("ws");
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    return proto + "//" + location.host + "/ws";
  }

  function log(kind, html) {
    const el = document.createElement("div");
    el.className = kind;
    el.innerHTML = html;
    $("log").appendChild(el);
    $("log").scrollTop = $("log").scrollHeight;
  }

  function setConn(mode, text) {
    const el = $("conn");
    el.className = "lamp " + (mode === "on" ? "" : mode === "wait" ? "wait" : "off");
    el.textContent = text;
  }

  function persistIdentity(id, name, session) {
    if (id) localStorage.setItem(KEYS.playerId, id);
    if (name) localStorage.setItem(KEYS.name, name);
    if (session) localStorage.setItem(KEYS.session, session);
  }

  function connect() {
    if (state.ws && (state.ws.readyState === 0 || state.ws.readyState === 1)) return;
    setConn("wait", "connecting");
    const ws = new WebSocket(wsURL());
    state.ws = ws;
    ws.onopen = () => {
      setConn("on", "online");
      state.reconnects = 0;
      ws.send(JSON.stringify({
        t: "hello",
        playerId: localStorage.getItem(KEYS.playerId) || "",
        name: $("name").value.trim() || localStorage.getItem(KEYS.name) || "Wanderer",
        session: localStorage.getItem(KEYS.session) || "",
      }));
      log("sys", "The stile opens. Hollowmere remembers your pack.");
    };
    ws.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch { return; }
      onMsg(msg);
    };
    ws.onclose = () => {
      setConn("off", "offline");
      if (state.dead) return;
      const wait = Math.min(8000, 600 * Math.pow(2, state.reconnects++));
      setConn("wait", "reconnect " + Math.ceil(wait / 1000) + "s");
      setTimeout(connect, wait);
    };
    ws.onerror = () => {};
  }

  function send(obj) {
    if (state.ws && state.ws.readyState === 1) state.ws.send(JSON.stringify(obj));
  }

  function rememberPos() {
    const next = {};
    const all = [];
    if (state.you) all.push(state.you);
    for (const p of state.players) all.push(p);
    for (const n of state.npcs) all.push(n);
    for (const e of all) {
      const prev = state.prevPos[e.id] || { x: e.x, y: e.y };
      next[e.id] = { x: e.x, y: e.y, px: prev.x, py: prev.y };
    }
    state.prevPos = next;
    state.lastTickAt = performance.now();
  }

  function onMsg(msg) {
    switch (msg.t) {
      case "welcome":
        state.tickMs = msg.tickMs || 600;
        state.world = msg.world;
        state.map = msg.map;
        state.you = msg.you;
        state.items = msg.items || {};
        persistIdentity(msg.playerId, msg.you && msg.you.name, msg.session);
        $("gate").classList.add("hidden");
        renderSkills();
        renderInv();
        break;
      case "state":
        state.last = msg;
        state.n = msg.n;
        state.online = msg.online;
        state.lastMs = msg.ms;
        state.you = msg.you;
        state.players = msg.players || [];
        state.npcs = msg.npcs || [];
        state.nodes = msg.nodes || [];
        rememberPos();
        renderSkills();
        renderInv();
        $("tickinfo").textContent = "tick " + msg.n + " · " + msg.online + " online · " + (msg.ms || 0).toFixed(1) + "ms";
        break;
      case "chat":
        log("say", '<span class="who">' + esc(msg.from) + ":</span> " + esc(msg.text));
        break;
      case "evt":
        log("evt", esc(msg.text));
        break;
      case "err":
        log("sys", esc(msg.msg));
        break;
      case "pong":
        break;
    }
  }

  function esc(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }

  function renderSkills() {
    const sk = (state.you && state.you.skills) || {};
    const forage = sk.forage || { xp: 0, lv: 1 };
    const cook = sk.cook || { xp: 0, lv: 1 };
    $("skills").innerHTML =
      skillRow("Foraging", forage) + skillRow("Cooking", cook);
  }

  function skillRow(label, s) {
    const need = 25 + (s.lv - 1) * 15;
    const pct = Math.min(100, Math.round(((s.xp % Math.max(need, 1)) / need) * 100));
    return '<div class="skill"><div class="row"><span>' + label + "</span><span>lv " +
      s.lv + " · " + s.xp + " xp</span></div><div class=\"bar\"><i style=\"width:" + pct + '%"></i></div></div>';
  }

  function renderInv() {
    const inv = (state.you && state.you.inv) || [];
    const byId = {};
    for (const it of inv) byId[it.id] = it;
    const order = ["berry", "pulp", "tart", "nut", "roast"];
    let html = "";
    for (const id of order) {
      const it = byId[id];
      const info = state.items[id] || { name: id, glyph: "?" };
      if (!it) {
        html += '<div class="slot empty"></div>';
        continue;
      }
      html += '<div class="slot" data-id="' + id + '" title="' + esc(info.name) + '"><div>' +
        esc(info.glyph) + "</div><div class=\"qty\">" + it.n + "</div></div>";
    }
    for (let i = order.length; i < 8; i++) html += '<div class="slot empty"></div>';
    $("inv").innerHTML = html;
  }

  $("inv").addEventListener("click", (e) => {
    const slot = e.target.closest(".slot");
    if (!slot || !slot.dataset.id) return;
    send({ t: "use", id: slot.dataset.id });
  });

  function tileAt(mx, my) {
    if (!state.map) return null;
    const r = canvas.getBoundingClientRect();
    const x = Math.floor((mx - r.left) / (r.width / state.map.w));
    const y = Math.floor((my - r.top) / (r.height / state.map.h));
    if (x < 0 || y < 0 || x >= state.map.w || y >= state.map.h) return null;
    return { x, y };
  }

  canvas.addEventListener("click", (e) => {
    const t = tileAt(e.clientX, e.clientY);
    if (!t) return;
    const node = (state.nodes || []).find((n) => n.x === t.x && n.y === t.y);
    if (node) send({ t: "interact", id: node.id });
    else send({ t: "move", x: t.x, y: t.y });
  });

  const held = {};
  window.addEventListener("keydown", (e) => {
    if (e.target === $("chat") || e.target === $("name")) return;
    const k = e.key.toLowerCase();
    if ("wasd".includes(k)) held[k] = true;
  });
  window.addEventListener("keyup", (e) => {
    const k = e.key.toLowerCase();
    if ("wasd".includes(k)) held[k] = false;
  });

  setInterval(() => {
    if (!state.you) return;
    let dx = 0, dy = 0;
    if (held.w) dy--;
    if (held.s) dy++;
    if (held.a) dx--;
    if (held.d) dx++;
    if (!dx && !dy) return;
    send({ t: "move", x: state.you.x + dx, y: state.you.y + dy });
  }, 200);

  $("chatform").addEventListener("submit", (e) => {
    e.preventDefault();
    const text = $("chat").value.trim();
    if (!text) return;
    send({ t: "chat", text });
    $("chat").value = "";
  });

  $("enter").addEventListener("click", () => {
    const name = $("name").value.trim() || "Wanderer";
    persistIdentity(localStorage.getItem(KEYS.playerId), name);
    state.wantJoin = true;
    connect();
  });
  $("name").addEventListener("keydown", (e) => {
    if (e.key === "Enter") $("enter").click();
  });

  $("name").value = localStorage.getItem(KEYS.name) || "";

  setInterval(() => {
    if (state.ws && state.ws.readyState === 1) send({ t: "ping", ts: Date.now() });
  }, 15000);

  function lerp(a, b, u) { return a + (b - a) * u; }
  function hashHue(s) {
    let h = 0;
    for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
    return Math.abs(h) % 360;
  }

  function posOf(id, x, y, u) {
    const p = state.prevPos[id];
    if (!p) return { x, y };
    return { x: lerp(p.px, x, u), y: lerp(p.py, y, u) };
  }

  function draw() {
    requestAnimationFrame(draw);
    const m = state.map;
    if (!m) {
      ctx.fillStyle = "#3d5a28";
      ctx.fillRect(0, 0, canvas.width, canvas.height);
      ctx.fillStyle = "#f3e2c7";
      ctx.font = "16px Trebuchet MS";
      ctx.fillText("Waiting at the stile…", 24, 40);
      return;
    }
    const tw = canvas.width / m.w;
    const th = canvas.height / m.h;
    const now = performance.now();
    const u = Math.min(1, (now - state.lastTickAt) / state.tickMs);
    const flicker = 0.5 + 0.5 * Math.sin(now / 90);

    for (let y = 0; y < m.h; y++) {
      const row = m.tiles[y] || "";
      for (let x = 0; x < m.w; x++) {
        const g = row[x] || ".";
        const px = x * tw, py = y * th;
        switch (g) {
          case "#":
            ctx.fillStyle = "#4a4035";
            ctx.fillRect(px, py, tw, th);
            ctx.fillStyle = "#3a3228";
            ctx.fillRect(px + 2, py + 2, tw - 4, th - 4);
            break;
          case "T":
            ctx.fillStyle = "#4f7a34";
            ctx.fillRect(px, py, tw, th);
            ctx.fillStyle = "#6b3e1a";
            ctx.fillRect(px + tw * 0.4, py + th * 0.45, tw * 0.2, th * 0.5);
            ctx.fillStyle = "#245218";
            ctx.beginPath();
            ctx.arc(px + tw * 0.5, py + th * 0.38, tw * 0.38, 0, Math.PI * 2);
            ctx.fill();
            break;
          case "~":
            ctx.fillStyle = "#2d5f94";
            ctx.fillRect(px, py, tw, th);
            ctx.strokeStyle = "rgba(180,220,255,0.35)";
            ctx.beginPath();
            ctx.moveTo(px, py + th * 0.4);
            ctx.quadraticCurveTo(px + tw * 0.5, py + th * (0.25 + 0.1 * flicker), px + tw, py + th * 0.4);
            ctx.stroke();
            break;
          case "P":
            ctx.fillStyle = "#c2a36b";
            ctx.fillRect(px, py, tw, th);
            ctx.fillStyle = "#b08950";
            ctx.fillRect(px + 4, py + 8, 3, 3);
            break;
          case "H":
          case "*":
            ctx.fillStyle = "#b08968";
            ctx.fillRect(px, py, tw, th);
            break;
          case "B":
          case "Z":
          case "M":
            ctx.fillStyle = (x + y) % 2 ? "#5a8f3c" : "#4f8236";
            ctx.fillRect(px, py, tw, th);
            break;
          default:
            ctx.fillStyle = (x + y) % 2 ? "#5a8f3c" : "#4f8236";
            ctx.fillRect(px, py, tw, th);
        }
      }
    }

    for (const n of state.nodes) {
      const px = n.x * tw, py = n.y * th;
      if (n.kind === "bush") {
        ctx.fillStyle = n.ready ? "#2f5a22" : "#3a4a30";
        ctx.beginPath();
        ctx.ellipse(px + tw / 2, py + th * 0.62, tw * 0.38, th * 0.28, 0, 0, Math.PI * 2);
        ctx.fill();
        if (n.ready) {
          ctx.fillStyle = "#b33";
          ctx.fillRect(px + tw * 0.3, py + th * 0.48, 4, 4);
          ctx.fillRect(px + tw * 0.55, py + th * 0.58, 4, 4);
        }
      } else if (n.kind === "hazel") {
        ctx.fillStyle = n.ready ? "#3d5a22" : "#3a4a30";
        ctx.beginPath();
        ctx.ellipse(px + tw / 2, py + th * 0.6, tw * 0.34, th * 0.26, 0, 0, Math.PI * 2);
        ctx.fill();
        ctx.fillStyle = "#6b3e1a";
        ctx.fillRect(px + tw * 0.45, py + th * 0.22, tw * 0.12, th * 0.28);
        if (n.ready) {
          ctx.fillStyle = "#c4a36a";
          ctx.beginPath();
          ctx.arc(px + tw * 0.38, py + th * 0.55, 3, 0, Math.PI * 2);
          ctx.arc(px + tw * 0.62, py + th * 0.62, 3, 0, Math.PI * 2);
          ctx.fill();
        }
      } else if (n.kind === "mill") {
        ctx.fillStyle = "#8a8070";
        ctx.beginPath();
        ctx.arc(px + tw / 2, py + th / 2, tw * 0.36, 0, Math.PI * 2);
        ctx.fill();
        ctx.strokeStyle = "#4a4035";
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.arc(px + tw / 2, py + th / 2, tw * 0.2, 0, Math.PI * 2);
        ctx.moveTo(px + tw / 2, py + th * 0.22);
        ctx.lineTo(px + tw / 2, py + th * 0.78);
        ctx.stroke();
      } else if (n.kind === "fire") {
        ctx.fillStyle = "#5a3a18";
        ctx.fillRect(px + 6, py + th * 0.62, tw - 12, 6);
        ctx.fillStyle = "rgba(255," + Math.floor(120 + 80 * flicker) + ",20,0.95)";
        ctx.beginPath();
        ctx.moveTo(px + tw / 2, py + 6);
        ctx.lineTo(px + tw * 0.28, py + th * 0.7);
        ctx.lineTo(px + tw * 0.72, py + th * 0.7);
        ctx.fill();
      }
    }

    const figures = [];
    for (const n of state.npcs) figures.push({ kind: "npc", e: n });
    for (const p of state.players) figures.push({ kind: "pl", e: p });
    if (state.you) figures.push({ kind: "you", e: state.you });
    figures.sort((a, b) => a.e.y - b.e.y);

    for (const f of figures) {
      const e = f.e;
      const p = posOf(e.id, e.x, e.y, u);
      const px = p.x * tw + tw / 2;
      const py = p.y * th + th * 0.62;
      const hue = f.kind === "npc" ? 35 : hashHue(e.name || e.id);
      ctx.fillStyle = "rgba(0,0,0,0.25)";
      ctx.beginPath();
      ctx.ellipse(px, py + 10, 9, 4, 0, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = f.kind === "you" ? "hsl(" + hue + ",70%,42%)" : "hsl(" + hue + ",45%,38%)";
      ctx.fillRect(px - 7, py - 8, 14, 16);
      ctx.fillStyle = "#f0d2b0";
      ctx.beginPath();
      ctx.arc(px, py - 12, 6, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = f.kind === "you" ? "#fff4b0" : "#f3e2c7";
      ctx.font = "11px Trebuchet MS";
      ctx.textAlign = "center";
      ctx.fillText(e.name || "?", px, py - 20);
      if (e.action && e.action !== "idle" && e.action !== "walk") {
        ctx.fillStyle = "#fff4b0";
        ctx.fillText(actionVoice(e.action), px, py + 22);
      }
    }
  }

  function actionVoice(action) {
    switch (action) {
      case "forage": return "gathering";
      case "mill": return "crushing";
      case "cook": return "baking";
      case "roast": return "roasting";
      default: return action;
    }
  }

  draw();
})();
