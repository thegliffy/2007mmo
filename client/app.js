(() => {
  const $ = (id) => document.getElementById(id);
  const canvas = $("stage");
  const ctx = canvas.getContext("2d");

  const PACK_SLOTS = 28;

  const state = {
    ws: null,
    tickMs: 600,
    world: "hollowmere",
    map: null,
    you: null,
    players: [],
    npcs: [],
    nodes: [],
    baseNodes: [],
    ground: [],
    items: {},
    skillInfo: {},
    shop: null,
    heldTool: null,
    last: null,
    prevPos: {},
    lastTickAt: 0,
    n: 0,
    online: 0,
    lastMs: 0,
    reconnects: 0,
    handle: null,
    username: null,
    authed: false,
    stopped: false,
    looksCatalog: null,
    draftLooks: null,
  };

  // The ?ws= override stays for local development only. On any other
  // host a crafted link could have pointed the socket at someone else's
  // server; the session cookie would not follow, but the override has no
  // business existing in production either.
  function isLocalHost() {
    return location.hostname === "127.0.0.1" ||
      location.hostname === "localhost" ||
      location.hostname === "[::1]";
  }

  function wsURL() {
    const q = new URLSearchParams(location.search);
    if (q.get("ws") && isLocalHost()) return q.get("ws");
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

  function connect() {
    if (state.stopped) return;
    if (state.ws && (state.ws.readyState === 0 || state.ws.readyState === 1)) return;
    setConn("wait", "connecting");
    const ws = new WebSocket(wsURL());
    state.ws = ws;
    ws.onopen = () => {
      setConn("on", "online");
      state.reconnects = 0;
      // No identity in the frame. The cookie sent with the upgrade already
      // decided who this socket belongs to.
      ws.send(JSON.stringify({ t: "hello" }));
      log("sys", "The stile opens. Hollowmere remembers your pack.");
    };
    ws.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch { return; }
      onMsg(msg);
    };
    ws.onclose = async () => {
      setConn("off", "offline");
      if (state.stopped) return;
      // A refused upgrade looks exactly like a dropped socket from here,
      // so ask the server which it was before retrying forever.
      let stillIn;
      try {
        stillIn = await checkSession();
      } catch {
        stillIn = true; // probe failed; keep the cookie and try the socket
      }
      if (!stillIn) {
        showGate("Your session ended. Log in again.");
        return;
      }
      const wait = Math.min(8000, 600 * Math.pow(2, state.reconnects++));
      setConn("wait", "reconnect " + Math.ceil(wait / 1000) + "s");
      setTimeout(connect, wait);
    };
    ws.onerror = () => {};
  }

  function send(obj) {
    if (state.ws && state.ws.readyState === 1) state.ws.send(JSON.stringify(obj));
  }

  // Anything absent from the frame is back at rest; anything present
  // overrides. A campfire arrives complete, because it did not exist when
  // the welcome was sent.
  function mergeNodes(changed) {
    const byId = {};
    for (const n of changed) byId[n.id] = n;
    const out = state.baseNodes.map((n) =>
      byId[n.id] ? { ...n, ...byId[n.id] } : { ...n, ready: true, left: n.left }
    );
    for (const n of changed) {
      if (!state.baseNodes.some((b) => b.id === n.id)) out.push(n);
    }
    return out;
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
      state.skillInfo = msg.skillInfo || {};
      // Every node's position and kind, sent once. State frames carry only
      // the ones that are not at rest.
      state.baseNodes = msg.nodes || [];
      state.nodes = state.baseNodes.map((n) => ({ ...n, ready: true }));
        state.handle = msg.handle || null;
        state.username = msg.username || null;
        if (msg.looksCatalog) state.looksCatalog = msg.looksCatalog;
        $("acct-name").textContent = state.username || "";
        $("gate").classList.add("hidden");
        $("looks").classList.add("hidden");
        renderSkills();
        renderVitals();
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
        state.nodes = mergeNodes(msg.nodes || []);
      state.ground = msg.ground || [];
        rememberPos();
        renderSkills();
        renderVitals();
        renderInv();
        if (state.shop) renderShop();
        $("tickinfo").textContent = "tick " + msg.n + " · " + msg.online + " online · " + (msg.ms || 0).toFixed(1) + "ms";
        break;
      case "chat":
        log("say", '<span class="who">' + esc(msg.from) + ":</span> " + esc(msg.text));
        break;
      case "evt":
        log("evt", esc(msg.text));
        if (state.shop) shopMsg(msg.text);
        break;
      case "err":
        log("sys", esc(msg.msg));
        // Server-told ends: do not reconnect. A later login (other tab
        // or other device) sends replaced; a dead cookie sends session.
        if (msg.code === "replaced" || msg.code === "session") {
          state.stopped = true;
          showGate(msg.msg || (msg.code === "replaced"
            ? "Signed in somewhere else."
            : "Your session ended. Log in again."));
        }
        break;
      case "trade":
        state.shop = msg;
        renderShop();
        $("shop").classList.remove("hidden");
        break;
      case "pong":
        break;
    }
  }

  // Escapes for both element text and attribute values. The quote cases
  // matter because esc() is used inside title="…"; names are sanitized
  // server-side today, but that is not a property this function should
  // depend on.
  function esc(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
  }

  // Driven by the catalog the server sends, not a list baked in here. A
  // skill added server-side used to be invisible until this function was
  // edited to match.
  function renderSkills() {
    const sk = (state.you && state.you.skills) || {};
    const info = state.skillInfo || {};
    const ids = Object.keys(info).length
      ? Object.keys(info).sort((a, b) => (info[a].order || 0) - (info[b].order || 0))
      : Object.keys(sk).sort();
    $("skills").innerHTML = ids
      .map((id) => skillRow((info[id] && info[id].name) || id, sk[id] || { xp: 0, lv: 1 }))
      .join("");
  }

  // Mirror of LevelFromXP on the server: each level costs 25 + 15 per
  // level already gained, and the cost of earlier levels is spent. The bar
  // used `xp % need`, which treats total XP as progress into the current
  // level — at 25 xp (exactly level 2, no progress) it showed 63%.
  function levelProgress(xp, lv) {
    let spent = 0;
    let need = 25;
    for (let l = 1; l < lv && l < 20; l++) {
      spent += need;
      need += 15;
    }
    const into = Math.max(0, xp - spent);
    return { into, need, pct: Math.min(100, Math.round((into / Math.max(need, 1)) * 100)) };
  }

  function skillRow(label, s) {
    const { into, need, pct } = levelProgress(s.xp, s.lv);
    return '<div class="skill"><div class="row"><span>' + label + "</span><span>lv " +
      s.lv + " · " + into + "/" + need + " xp</span></div><div class=\"bar\"><i style=\"width:" +
      pct + '%"></i></div></div>';
  }

  function renderVitals() {
    const el = $("vitals");
    if (!el) return;
    const you = state.you || {};
    const hp = you.hp == null ? 10 : you.hp;
    const max = you.maxHp || 10;
    const pct = Math.max(0, Math.min(100, Math.round((hp / Math.max(max, 1)) * 100)));
    let targetName = "";
    if (you.target) {
      const n = (state.npcs || []).find((npc) => npc.id === you.target);
      if (n) targetName = n.name + " · " + (n.hp == null ? "?" : n.hp) + "/" + (n.maxHp || "?");
      else targetName = "closing in…";
    }
    el.innerHTML =
      '<div class="vital"><div class="row"><span>Heart</span><span>' + hp + " / " + max +
      '</span></div><div class="bar hp"><i style="width:' + pct + '%"></i></div>' +
      (targetName ? '<p class="target">Upon the ' + esc(targetName) + "</p>" : "") +
      "</div>";
  }

  // The pedlar's board. Left column is what she stocks, right is what she
  // will take off you; a row is only clickable if the trade can actually
  // happen, so the panel never invites a refusal.
  function renderShop() {
    const shop = state.shop;
    if (!shop) return;
    const coins = (state.you && state.you.coins) || 0;
    const inv = {};
    for (const it of (state.you && state.you.inv) || []) inv[it.id] = it.n;

    $("shop-who").textContent = shop.with || "The Pedlar";
    $("shop-purse").textContent = coins === 1 ? "1 coin" : coins + " coins";

    const name = (id) => (state.items[id] && state.items[id].name) || id;
    const row = (id, priceLabel, sub, disabled, action) =>
      '<button class="shoprow" data-action="' + action + '" data-id="' + esc(id) + '"' +
      (disabled ? " disabled" : "") + '><span>' + esc(name(id)) +
      (sub ? ' <span class="held">' + esc(sub) + "</span>" : "") +
      '</span><span class="coin">' + esc(priceLabel) + "</span></button>";

    const sells = (shop.offers || []).filter((o) => o.costs > 0);
    $("shop-sells").innerHTML = sells.length
      ? sells.map((o) => row(o.item, o.costs + "c", "", coins < o.costs, "buy")).join("")
      : '<p class="shopempty">Nothing today.</p>';

    const buys = (shop.offers || []).filter((o) => o.pays > 0);
    $("shop-buys").innerHTML = buys.length
      ? buys
          .map((o) =>
            row(o.item, o.pays + "c", inv[o.item] ? "x" + inv[o.item] : "", !inv[o.item], "sell")
          )
          .join("")
      : '<p class="shopempty">She wants for nothing.</p>';
  }

  function shopMsg(text) {
    const el = $("shop-msg");
    if (el) el.textContent = text || "";
  }

  $("shop").addEventListener("click", (e) => {
    const btn = e.target.closest(".shoprow");
    if (!btn || btn.disabled) return;
    send({ t: btn.dataset.action, id: btn.dataset.id });
  });

  $("shop-close").addEventListener("click", () => {
    state.shop = null;
    shopMsg("");
    $("shop").classList.add("hidden");
  });

  function renderInv() {
    const purse = $("purse");
    if (purse) {
      const coins = (state.you && state.you.coins) || 0;
      purse.textContent = coins === 1 ? "1 coin" : coins + " coins";
    }
    // The pack in slot order, exactly as the server holds it. It used to be
    // drawn as one fixed slot per item type, which is a catalogue rather
    // than an inventory: tools had nowhere to sit and two of a thing looked
    // like one.
    const inv = (state.you && state.you.inv) || [];
    let html = "";
    for (let i = 0; i < PACK_SLOTS; i++) {
      const it = inv[i];
      if (!it) {
        html += '<div class="slot empty"></div>';
        continue;
      }
      const info = state.items[it.id] || { name: it.id, glyph: "?" };
      const held = state.heldTool === i ? " held" : "";
      const tool = info.tool ? " tool" : "";
      html +=
        '<div class="slot' + tool + held + '" data-id="' + esc(it.id) + '" data-slot="' + i +
        '" title="' + esc(info.name) + (info.tool && info.verb ? " — click, then click what to use it on" : "") +
        '"><div>' + esc(info.glyph) + "</div>" +
        (it.n > 1 ? '<div class="qty">' + it.n + "</div>" : "") +
        "</div>";
    }
    $("inv").innerHTML = html;
    const hint = $("held-hint");
    if (hint) {
      const h = heldToolItem();
      hint.textContent = h ? "Holding the " + (state.items[h.id] || {}).name + " — click what to use it on." : "";
      hint.classList.toggle("hidden", !h);
    }
  }

  function heldToolItem() {
    const inv = (state.you && state.you.inv) || [];
    if (state.heldTool == null) return null;
    const it = inv[state.heldTool];
    if (!it) return null;
    const info = state.items[it.id];
    return info && info.tool && info.verb ? it : null;
  }

  $("inv").addEventListener("click", (e) => {
    const slot = e.target.closest(".slot");
    if (!slot || !slot.dataset.id) return;
    const info = state.items[slot.dataset.id] || {};
    if (info.tool && info.verb) {
      // Take it in hand; the next click on the world says what to use it on.
      const i = Number(slot.dataset.slot);
      state.heldTool = state.heldTool === i ? null : i;
      renderInv();
      return;
    }
    send({ t: "use", id: slot.dataset.id });
  });

  // Right-click sets one down at your feet. preventDefault so the browser
  // menu does not appear over the hamlet.
  $("inv").addEventListener("contextmenu", (e) => {
    const slot = e.target.closest(".slot");
    if (!slot || !slot.dataset.id) return;
    e.preventDefault();
    state.heldTool = null;
    send({ t: "drop", id: slot.dataset.id });
  });

  function tileAt(mx, my) {
    if (!state.map) return null;
    const r = canvas.getBoundingClientRect();
    const x = Math.floor((mx - r.left) / (r.width / state.map.w));
    const y = Math.floor((my - r.top) / (r.height / state.map.h));
    if (x < 0 || y < 0 || x >= state.map.w || y >= state.map.h) return null;
    return { x, y };
  }

  // Right-click a pile to set it alight, mirroring the pack where
  // left-click uses and right-click drops.
  canvas.addEventListener("contextmenu", (e) => {
    const t = tileAt(e.clientX, e.clientY);
    if (!t) return;
    const pile = state.ground.find((g) => g.x === t.x && g.y === t.y);
    if (!pile) return;
    e.preventDefault();
    send({ t: "light", id: pile.id });
  });

  canvas.addEventListener("click", (e) => {
    const t = tileAt(e.clientX, e.clientY);
    if (!t) return;

    // Holding a tool? Point it at something.
    const held = heldToolItem();
    if (held) {
      const info = state.items[held.id] || {};
      if (info.verb === "light") {
        const pile = state.ground.find((g) => g.x === t.x && g.y === t.y);
        if (pile) {
          send({ t: "light", id: pile.id });
          state.heldTool = null;
          renderInv();
          return;
        }
      }
      if (info.verb === "chop") {
        const tree = state.nodes.find((n) => n.kind === "tree" && n.x === t.x && n.y === t.y);
        if (tree) {
          send({ t: "interact", id: tree.id });
          state.heldTool = null;
          renderInv();
          return;
        }
      }
      if (info.verb === "mine") {
        const vein = state.nodes.find((n) =>
          (n.kind === "copper" || n.kind === "tin") && n.x === t.x && n.y === t.y);
        if (vein) {
          send({ t: "interact", id: vein.id });
          state.heldTool = null;
          renderInv();
          return;
        }
      }
      // Clicked nothing the tool works on: put it away and carry on.
      state.heldTool = null;
      renderInv();
    }
    const pick = globalThis.HollowmerePick;
    const intent = pick && pick.resolveCanvasClick
      ? pick.resolveCanvasClick(state.npcs, state.nodes, t, state.ground)
      : resolveClickFallback(state.npcs, state.nodes, t, state.ground);
    if (intent.t === "attack" && state.you) {
      state.you.target = intent.id;
      renderVitals();
    }
    send(intent);
  });

  // Same Chebyshev ≤ 1 rule as pick-npc.js. Kept here so a missing
  // pick-npc.js (or a cached app.js-only deploy) still sends attack.
  function resolveClickFallback(npcs, nodes, tile, ground) {
    const pile = (ground || []).find((g) => g.x === tile.x && g.y === tile.y);
    const npcOnTile = (npcs || []).some((n) => n.x === tile.x && n.y === tile.y);
    if (pile && !npcOnTile) return { t: "interact", id: pile.id };
    let best = null, bestDist = 99, bestHostile = false;
    for (const n of npcs || []) {
      if (n.maxHp > 0 && (n.hp == null ? n.maxHp : n.hp) <= 0) continue;
      const d = Math.max(Math.abs(n.x - tile.x), Math.abs(n.y - tile.y));
      if (d > 1) continue;
      const hostile = !!n.hostile;
      if (!best || (hostile && !bestHostile) || (hostile === bestHostile && d < bestDist)) {
        best = n;
        bestDist = d;
        bestHostile = hostile;
      }
    }
    if (best) return { t: best.hostile ? "attack" : "interact", id: best.id };
    if (pile) return { t: "interact", id: pile.id };
    const node = (nodes || []).find((n) => n.x === tile.x && n.y === tile.y);
    if (node) return { t: "interact", id: node.id };
    return { t: "move", x: tile.x, y: tile.y };
  }

  const held = {};
  window.addEventListener("keydown", (e) => {
    const tag = e.target && e.target.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA") return;
    const k = e.key.toLowerCase();
    if ("wasd".includes(k)) held[k] = true;
  });
  window.addEventListener("keyup", (e) => {
    const k = e.key.toLowerCase();
    if ("wasd".includes(k)) held[k] = false;
  });

  setInterval(() => {
    if (!state.you || !state.authed) return;
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

  // ---- auth portal -------------------------------------------------------

  async function authFetch(path, body) {
    const opts = {
      method: body === undefined ? "GET" : "POST",
      credentials: "same-origin",
      headers: {},
    };
    if (body !== undefined) {
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }
    let res;
    try {
      res = await fetch(path, opts);
    } catch {
      return { ok: false, status: 0, error: "Hollowmere is not answering." };
    }
    let data = {};
    try { data = await res.json(); } catch { /* empty body is fine */ }
    return {
      ok: res.ok,
      status: res.status,
      error: data.error,
      username: data.username,
      looks: data.looks || null,
      needsLooks: !!data.needsLooks,
      catalog: data.catalog || data.looksCatalog || null,
    };
  }

  function showGate(message, kind) {
    state.authed = false;
    state.stopped = true;
    if (state.ws) { try { state.ws.close(); } catch { /* already gone */ } }
    state.ws = null;
    state.you = null;
    state.map = null;
    $("looks").classList.add("hidden");
    $("gate").classList.remove("hidden");
    selectTab("login");
    // selectTab clears the message, so set it afterwards.
    authMsg(message || "", message ? (kind || "bad") : "");
  }

  function authMsg(text, kind) {
    const el = $("auth-msg");
    el.textContent = text || "";
    el.className = "authmsg" + (kind ? " " + kind : "");
  }

  function acctMsg(text, kind) {
    const el = $("acct-msg");
    el.textContent = text || "";
    el.className = "authmsg" + (kind ? " " + kind : "");
  }

  // checkSession reports whether the cookie is still good.
  // A failed probe (world restarting, brief blip) must not look like a
  // logout — that is how a live tab used to dump people at the stile.
  async function checkSession() {
    const r = await authFetch("/auth/me");
    if (r.ok) {
      state.username = r.username || state.username;
      $("acct-name").textContent = state.username || "";
      return true;
    }
    if (r.status === 0) return true;
    return false;
  }

  function enterWorld(username) {
    state.username = username || state.username;
    state.authed = true;
    state.stopped = false;
    state.reconnects = 0;
    $("acct-name").textContent = state.username || "";
    $("looks").classList.add("hidden");
    authMsg("");
    connect();
  }

  function selectTab(which) {
    const login = which === "login";
    $("tab-login").classList.toggle("on", login);
    $("tab-register").classList.toggle("on", !login);
    $("tab-login").setAttribute("aria-selected", String(login));
    $("tab-register").setAttribute("aria-selected", String(!login));
    $("form-login").classList.toggle("hidden", !login);
    $("form-register").classList.toggle("hidden", login);
    authMsg("");
  }

  $("tab-login").addEventListener("click", () => selectTab("login"));
  $("tab-register").addEventListener("click", () => selectTab("register"));

  function busy(form, on) {
    for (const el of form.querySelectorAll("input,button")) el.disabled = on;
  }

  $("form-login").addEventListener("submit", async (e) => {
    e.preventDefault();
    const form = e.currentTarget;
    const username = $("login-user").value.trim();
    const password = $("login-pass").value;
    if (!username || !password) {
      authMsg("A name and a password, please.", "bad");
      return;
    }
    busy(form, true);
    authMsg("Knocking…");
    const r = await authFetch("/auth/login", { username, password });
    busy(form, false);
    if (!r.ok) {
      authMsg(r.error || "That did not work.", "bad");
      return;
    }
    $("login-pass").value = "";
    afterAuth(r);
  });

  $("form-register").addEventListener("submit", async (e) => {
    e.preventDefault();
    const form = e.currentTarget;
    const username = $("reg-user").value.trim();
    const password = $("reg-pass").value;
    if (password !== $("reg-pass2").value) {
      authMsg("Those two passwords are not the same.", "bad");
      return;
    }
    busy(form, true);
    authMsg("Carving your name…");
    const r = await authFetch("/auth/register", { username, password });
    busy(form, false);
    if (!r.ok) {
      authMsg(r.error || "That did not work.", "bad");
      return;
    }
    $("reg-pass").value = "";
    $("reg-pass2").value = "";
    afterAuth(r);
  });

  $("do-logout").addEventListener("click", async () => {
    // Logging out closes the socket server-side. Stop the reconnect path
    // first, or its onclose handler races this one for the gate message.
    state.stopped = true;
    await authFetch("/auth/logout", {});
    $("form-password").classList.add("hidden");
    acctMsg("");
    showGate("You have left the hamlet. Come back soon.", "good");
  });

  $("show-pw").addEventListener("click", () => {
    $("form-password").classList.toggle("hidden");
    acctMsg("");
  });

  $("cancel-pw").addEventListener("click", () => {
    $("form-password").classList.add("hidden");
    acctMsg("");
  });

  $("form-password").addEventListener("submit", async (e) => {
    e.preventDefault();
    const form = e.currentTarget;
    const current = $("pw-current").value;
    const next = $("pw-next").value;
    if (next !== $("pw-next2").value) {
      acctMsg("Those two passwords are not the same.", "bad");
      return;
    }
    busy(form, true);
    const r = await authFetch("/auth/password", { current, next });
    busy(form, false);
    if (!r.ok) {
      acctMsg(r.error || "That did not work.", "bad");
      return;
    }
    $("pw-current").value = "";
    $("pw-next").value = "";
    $("pw-next2").value = "";
    $("form-password").classList.add("hidden");
    acctMsg("Password changed. Other sessions were signed out.", "good");
  });

  function afterAuth(r) {
    state.username = r.username || state.username;
    $("acct-name").textContent = state.username || "";
    if (r.catalog) state.looksCatalog = r.catalog;
    if (r.needsLooks || !r.looks) {
      showCreator(state.username);
      return;
    }
    enterWorld(state.username);
  }

  function looksMsg(text, kind) {
    const el = $("looks-msg");
    el.textContent = text || "";
    el.className = "authmsg" + (kind ? " " + kind : "");
  }

  function showCreator(username) {
    state.username = username || state.username;
    state.authed = true;
    state.stopped = true;
    if (state.ws) { try { state.ws.close(); } catch { /* already gone */ } }
    state.ws = null;
    $("gate").classList.add("hidden");
    $("looks").classList.remove("hidden");
    $("looks-name").textContent = state.username || "";
    looksMsg("");
    loadLooksDesk();
  }

  const FALLBACK_CATALOG = {
    skin: [
      { id: "fair", name: "Fair", color: "#f0d2b0" },
      { id: "tan", name: "Tan", color: "#d4a574" },
      { id: "olive", name: "Olive", color: "#b08a58" },
      { id: "deep", name: "Deep", color: "#6b4226" },
    ],
    hair: [
      { id: "cropped", name: "Cropped" },
      { id: "short", name: "Short" },
      { id: "tied", name: "Tied" },
      { id: "long", name: "Long" },
    ],
    hairColor: [
      { id: "umber", name: "Umber", color: "#3d2412" },
      { id: "straw", name: "Straw", color: "#d4b46a" },
      { id: "soot", name: "Soot", color: "#1c140c" },
      { id: "russet", name: "Russet", color: "#8a3a1c" },
      { id: "snow", name: "Snow", color: "#e8e0d0" },
    ],
    top: [
      { id: "moss", name: "Moss", color: "#4a6a32" },
      { id: "clay", name: "Clay", color: "#a85a32" },
      { id: "ink", name: "Ink", color: "#2a3a5a" },
      { id: "cream", name: "Cream", color: "#e8d4a8" },
      { id: "berry", name: "Berry", color: "#7a2a40" },
    ],
  };

  function looksCatalog() {
    return state.looksCatalog || FALLBACK_CATALOG;
  }

  function colorOf(axis, id) {
    const opts = looksCatalog()[axis] || [];
    const hit = opts.find((o) => o.id === id);
    return (hit && hit.color) || "";
  }

  function defaultDraft() {
    return { skin: "tan", hair: "short", hairColor: "umber", top: "moss" };
  }

  function renderLooksPickers() {
    const cat = looksCatalog();
    const draft = state.draftLooks || defaultDraft();
    const axes = [
      { key: "skin", label: "Skin" },
      { key: "hair", label: "Hair" },
      { key: "hairColor", label: "Hair colour" },
      { key: "top", label: "Tunic" },
    ];
    const host = $("looks-pickers");
    host.innerHTML = "";
    for (const axis of axes) {
      const set = document.createElement("fieldset");
      set.className = "looks-axis";
      const legend = document.createElement("legend");
      legend.textContent = axis.label;
      set.appendChild(legend);
      const row = document.createElement("div");
      row.className = "swatches";
      for (const opt of cat[axis.key] || []) {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.dataset.axis = axis.key;
        btn.dataset.id = opt.id;
        btn.title = opt.name;
        if (opt.color) {
          btn.className = "swatch" + (draft[axis.key] === opt.id ? " on" : "");
          btn.style.background = opt.color;
          btn.setAttribute("aria-label", opt.name);
        } else {
          btn.className = "stylepick" + (draft[axis.key] === opt.id ? " on" : "");
          btn.textContent = opt.name;
        }
        btn.addEventListener("click", () => {
          state.draftLooks = Object.assign({}, state.draftLooks || defaultDraft(), { [axis.key]: opt.id });
          renderLooksPickers();
          paintLooksPreview();
        });
        row.appendChild(btn);
      }
      set.appendChild(row);
      host.appendChild(set);
    }
  }

  function paintLooksPreview() {
    const canvas = $("looks-preview");
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    drawPaperdoll(ctx, canvas.width / 2, canvas.height * 0.62, 22, state.draftLooks || defaultDraft(), true);
    ctx.fillStyle = "#3d2412";
    ctx.font = "14px Georgia";
    ctx.textAlign = "center";
    ctx.fillText(state.username || "Wanderer", canvas.width / 2, 28);
  }

  async function loadLooksDesk() {
    state.draftLooks = defaultDraft();
    const r = await authFetch("/auth/looks");
    if (r.ok && r.catalog) state.looksCatalog = r.catalog;
    if (r.ok && r.looks) state.draftLooks = r.looks;
    renderLooksPickers();
    paintLooksPreview();
  }

  $("form-looks").addEventListener("submit", async (e) => {
    e.preventDefault();
    const form = e.currentTarget;
    const looks = state.draftLooks || defaultDraft();
    busy(form, true);
    looksMsg("Carving…");
    const r = await authFetch("/auth/looks", looks);
    busy(form, false);
    if (!r.ok) {
      looksMsg(r.error || "That did not take.", "bad");
      return;
    }
    $("looks").classList.add("hidden");
    enterWorld(state.username);
  });

  // Boot: a live cookie with a face walks straight in. A live cookie
  // without one stops at the creator. No cookie, the stile.
  (async () => {
    const r = await authFetch("/auth/me");
    if (r.status === 0) {
      enterWorld(state.username);
      return;
    }
    if (r.ok) {
      afterAuth(r);
    } else {
      $("gate").classList.remove("hidden");
      selectTab("login");
    }
  })();

  setInterval(() => {
    if (state.ws && state.ws.readyState === 1) send({ t: "ping", ts: Date.now() });
  }, 15000);

  function lerp(a, b, u) { return a + (b - a) * u; }

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
    // Derive from the canvas, not m.tile: the canvas is a fixed size and
    // the map dimensions decide the rest. m.tile is advisory only.
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
        const scars = x >= 27;
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
            ctx.fillStyle = scars ? "#8a7350" : "#c2a36b";
            ctx.fillRect(px, py, tw, th);
            ctx.fillStyle = scars ? "#6a5340" : "#b08950";
            ctx.fillRect(px + 4, py + 8, 3, 3);
            break;
          case "H":
          case "*":
            ctx.fillStyle = "#b08968";
            ctx.fillRect(px, py, tw, th);
            break;
          case "C":
          case "N":
          case "K":
          case "A":
            ctx.fillStyle = (x + y) % 2 ? "#6a5a3c" : "#5a4a30";
            ctx.fillRect(px, py, tw, th);
            break;
          case "B":
          case "Z":
          case "M":
            ctx.fillStyle = (x + y) % 2 ? "#5a8f3c" : "#4f8236";
            ctx.fillRect(px, py, tw, th);
            break;
          default:
            if (scars) {
              ctx.fillStyle = (x + y) % 2 ? "#6e5a40" : "#5c4a34";
            } else {
              ctx.fillStyle = (x + y) % 2 ? "#5a8f3c" : "#4f8236";
            }
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
      } else if (n.kind === "tree") {
        // The map already paints a tree on this tile; when it has been
        // chopped bare, cover it with a stump so it reads as spent.
        if (!n.ready) {
          ctx.fillStyle = (n.x + n.y) % 2 ? "#5a8f3c" : "#4f8236";
          ctx.fillRect(px, py, tw, th);
          ctx.fillStyle = "#6b3e1a";
          ctx.beginPath();
          ctx.ellipse(px + tw / 2, py + th * 0.62, tw * 0.22, th * 0.14, 0, 0, Math.PI * 2);
          ctx.fill();
          ctx.strokeStyle = "#4a2a10";
          ctx.lineWidth = 1;
          ctx.stroke();
        }
      } else if (n.kind === "copper" || n.kind === "tin") {
        ctx.fillStyle = n.ready ? "#6a5340" : "#4a4035";
        ctx.beginPath();
        ctx.moveTo(px + tw * 0.18, py + th * 0.72);
        ctx.lineTo(px + tw * 0.38, py + th * 0.28);
        ctx.lineTo(px + tw * 0.68, py + th * 0.32);
        ctx.lineTo(px + tw * 0.84, py + th * 0.7);
        ctx.closePath();
        ctx.fill();
        if (n.ready) {
          ctx.fillStyle = n.kind === "copper" ? "#c46a32" : "#c8c4b0";
          ctx.fillRect(px + tw * 0.4, py + th * 0.48, 4, 4);
          ctx.fillRect(px + tw * 0.55, py + th * 0.58, 3, 3);
        }
      } else if (n.kind === "kiln") {
        ctx.fillStyle = "#3a2a22";
        ctx.fillRect(px + 2, py + th * 0.18, tw - 4, th * 0.72);
        ctx.fillStyle = "#1a140e";
        ctx.fillRect(px + tw * 0.28, py + th * 0.42, tw * 0.44, th * 0.36);
        ctx.fillStyle = "rgba(255," + Math.floor(90 + 70 * flicker) + ",20,0.95)";
        ctx.fillRect(px + tw * 0.34, py + th * 0.48, tw * 0.32, th * 0.22);
        ctx.fillStyle = "#c4a36a";
        ctx.font = Math.max(9, Math.floor(th * 0.28)) + "px Trebuchet MS";
        ctx.textAlign = "center";
        ctx.fillText("kiln", px + tw / 2, py + th * 0.16);
      } else if (n.kind === "anvil") {
        ctx.fillStyle = "#2a2a2a";
        ctx.fillRect(px + tw * 0.12, py + th * 0.38, tw * 0.76, th * 0.22);
        ctx.fillRect(px + tw * 0.36, py + th * 0.56, tw * 0.28, th * 0.28);
        ctx.fillStyle = "#8a8a8a";
        ctx.fillRect(px + tw * 0.08, py + th * 0.3, tw * 0.84, th * 0.14);
        ctx.fillStyle = "#c4a36a";
        ctx.font = Math.max(9, Math.floor(th * 0.28)) + "px Trebuchet MS";
        ctx.textAlign = "center";
        ctx.fillText("anvil", px + tw / 2, py + th * 0.22);
      } else if (n.kind === "fire") {
        ctx.fillStyle = "#5a3a18";
        ctx.fillRect(px + 6, py + th * 0.62, tw - 12, 6);
        ctx.fillStyle = "rgba(255," + Math.floor(120 + 80 * flicker) + ",20,0.95)";
        ctx.beginPath();
        ctx.moveTo(px + tw / 2, py + 6);
        ctx.lineTo(px + tw * 0.28, py + th * 0.7);
        ctx.lineTo(px + tw * 0.72, py + th * 0.7);
        ctx.fill();
        // A campfire dies down; show what is left of it.
        if (n.burns > 0) {
          const life = Math.max(0, Math.min(1, n.burns / 150));
          ctx.fillStyle = "rgba(40,24,12,0.75)";
          ctx.fillRect(px + 4, py + th - 6, tw - 8, 3);
          ctx.fillStyle = "hsl(28,80%,55%)";
          ctx.fillRect(px + 4, py + th - 6, (tw - 8) * life, 3);
        }
      }
    }

    for (const g of state.ground) {
      const px = g.x * tw, py = g.y * th;
      ctx.fillStyle = "rgba(0,0,0,0.25)";
      ctx.beginPath();
      ctx.ellipse(px + tw / 2, py + th * 0.72, tw * 0.3, th * 0.16, 0, 0, Math.PI * 2);
      ctx.fill();
      // A small sack; a coin glint on top when there is coin in it.
      ctx.fillStyle = "#6b4423";
      ctx.beginPath();
      ctx.moveTo(px + tw * 0.32, py + th * 0.72);
      ctx.lineTo(px + tw * 0.4, py + th * 0.44);
      ctx.lineTo(px + tw * 0.6, py + th * 0.44);
      ctx.lineTo(px + tw * 0.68, py + th * 0.72);
      ctx.closePath();
      ctx.fill();
      ctx.fillStyle = "#3d2412";
      ctx.fillRect(px + tw * 0.38, py + th * 0.4, tw * 0.24, 3);
      // Still reserved for you: a soft ring so it reads as "mine for now".
      if (g.mine) {
        ctx.strokeStyle = "rgba(250,225,150," + (0.45 + 0.3 * flicker) + ")";
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.ellipse(px + tw / 2, py + th * 0.72, tw * 0.36, th * 0.2, 0, 0, Math.PI * 2);
        ctx.stroke();
      }
      if (g.coins > 0) {
        ctx.fillStyle = "hsl(45,80%," + Math.floor(50 + 12 * flicker) + "%)";
        ctx.beginPath();
        ctx.arc(px + tw * 0.5, py + th * 0.55, 3, 0, Math.PI * 2);
        ctx.fill();
      }
    }

    const figures = [];
    for (const n of state.npcs) figures.push({ kind: "npc", e: n });
    for (const p of state.players) figures.push({ kind: "pl", e: p });
    if (state.you) figures.push({ kind: "you", e: state.you });
    figures.sort((a, b) => a.e.y - b.e.y);

    const targetId = state.you && state.you.target;
    for (const f of figures) {
      const e = f.e;
      const p = posOf(e.id, e.x, e.y, u);
      const px = p.x * tw + tw / 2;
      const py = p.y * th + th * 0.62;
      const hostile = f.kind === "npc" && e.hostile;
      if (e.id && e.id === targetId) {
        ctx.strokeStyle = "rgba(190,50,30,0.85)";
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.ellipse(px, py + 10, 12, 6, 0, 0, Math.PI * 2);
        ctx.stroke();
      }
      if (f.kind === "npc") {
        const hue = hostile ? 8 : 35;
        ctx.fillStyle = "rgba(0,0,0,0.25)";
        ctx.beginPath();
        ctx.ellipse(px, py + 10, 9, 4, 0, 0, Math.PI * 2);
        ctx.fill();
        ctx.fillStyle = "hsl(" + hue + "," + (hostile ? "55" : "45") + "%," + (hostile ? "28" : "38") + "%)";
        ctx.fillRect(px - 7, py - 8, 14, 16);
        if (hostile) {
          ctx.fillStyle = "#5a2a18";
          ctx.fillRect(px - 3, py - 16, 6, 5);
        }
        ctx.fillStyle = hostile ? "#d8b090" : "#f0d2b0";
        ctx.beginPath();
        ctx.arc(px, py - 12, 6, 0, Math.PI * 2);
        ctx.fill();
      } else {
        drawPaperdoll(ctx, px, py, 8, e.looks || defaultDraft(), f.kind === "you");
      }
      ctx.fillStyle = f.kind === "you" ? "#fff4b0" : hostile ? "#f0c8a0" : "#f3e2c7";
      ctx.font = "11px Trebuchet MS";
      ctx.textAlign = "center";
      ctx.fillText(e.name || "?", px, py - 22);
      if (e.maxHp > 0 && (f.kind === "you" || hostile)) {
        const hp = e.hp == null ? e.maxHp : e.hp;
        const bw = 18;
        const bh = 3;
        const bx = px - bw / 2;
        const by = py - 34;
        ctx.fillStyle = "#2a160c";
        ctx.fillRect(bx - 1, by - 1, bw + 2, bh + 2);
        ctx.fillStyle = "#5a381c";
        ctx.fillRect(bx, by, bw, bh);
        ctx.fillStyle = hostile ? "#c44" : "#6a3";
        ctx.fillRect(bx, by, bw * Math.max(0, Math.min(1, hp / e.maxHp)), bh);
      }
      if (e.action && e.action !== "idle" && e.action !== "walk") {
        ctx.fillStyle = "#fff4b0";
        ctx.fillText(actionVoice(e.action), px, py + 22);
      }
    }
  }

  function shadeHex(hex, amt) {
    const n = String(hex || "").replace("#", "");
    if (n.length < 6) return hex;
    const k = (i) => Math.max(0, Math.min(255, Math.round(parseInt(n.slice(i, i + 2), 16) * amt)));
    const h = (v) => v.toString(16).padStart(2, "0");
    return "#" + h(k(0)) + h(k(2)) + h(k(4));
  }

  function drawHair(ctx, hx, hy, r, style, color) {
    ctx.fillStyle = color;
    if (style === "cropped") {
      ctx.beginPath();
      ctx.ellipse(hx, hy - r * 0.55, r * 0.95, r * 0.38, 0, Math.PI, 0);
      ctx.fill();
      return;
    }
    ctx.beginPath();
    ctx.arc(hx, hy - r * 0.15, r * 1.05, Math.PI * 1.05, Math.PI * 1.95);
    ctx.fill();
    if (style === "tied") {
      ctx.beginPath();
      ctx.arc(hx + r * 0.85, hy - r * 0.15, r * 0.42, 0, Math.PI * 2);
      ctx.fill();
    }
    if (style === "long") {
      ctx.fillRect(hx - r * 1.05, hy - r * 0.1, r * 0.42, r * 1.35);
      ctx.fillRect(hx + r * 0.63, hy - r * 0.1, r * 0.42, r * 1.35);
    }
  }

  // Stylized paperdoll: coloured blob with hair and a tunic until real
  // 3D assets exist. Same function paints the creator preview and AOI.
  function drawPaperdoll(ctx, cx, cy, s, looks, highlight) {
    const l = looks || defaultDraft();
    const skin = colorOf("skin", l.skin) || "#d4a574";
    const top = colorOf("top", l.top) || "#4a6a32";
    const hair = colorOf("hairColor", l.hairColor) || "#3d2412";
    const unit = s / 8;
    ctx.fillStyle = "rgba(0,0,0,0.25)";
    ctx.beginPath();
    ctx.ellipse(cx, cy + 10 * unit, 9 * unit, 4 * unit, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = shadeHex(skin, 0.78);
    ctx.fillRect(cx - 5 * unit, cy + 2 * unit, 4 * unit, 8 * unit);
    ctx.fillRect(cx + 1 * unit, cy + 2 * unit, 4 * unit, 8 * unit);
    ctx.fillStyle = top;
    ctx.fillRect(cx - 7 * unit, cy - 8 * unit, 14 * unit, 13 * unit);
    if (highlight) {
      ctx.strokeStyle = "rgba(255,244,176,0.7)";
      ctx.lineWidth = Math.max(1, unit);
      ctx.strokeRect(cx - 7 * unit, cy - 8 * unit, 14 * unit, 13 * unit);
    }
    const hx = cx;
    const hy = cy - 12 * unit;
    const hr = 6 * unit;
    ctx.fillStyle = skin;
    ctx.beginPath();
    ctx.arc(hx, hy, hr, 0, Math.PI * 2);
    ctx.fill();
    drawHair(ctx, hx, hy, hr, l.hair || "short", hair);
    ctx.fillStyle = "#2a160c";
    ctx.fillRect(hx - 2.4 * unit, hy - 1.2 * unit, 1.4 * unit, 1.4 * unit);
    ctx.fillRect(hx + 1 * unit, hy - 1.2 * unit, 1.4 * unit, 1.4 * unit);
  }

  function actionVoice(action) {
    switch (action) {
      case "forage": return "gathering";
      case "mill": return "crushing";
      case "cook": return "baking";
      case "roast": return "roasting";
      case "chop": return "chopping";
      case "paper": return "pulping";
      case "mine": return "mining";
      case "smelt": return "smelting";
      case "forge": return "forging";
      case "fight": return "fighting";
      default: return action;
    }
  }

  draw();
})();
