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
    chest: null,
    bankSlots: 20,
    heldTool: null,
    equipSlots: [],
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
    cam: null,
    camFocus: null,
    camSnap: true,
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
      state.equipSlots = msg.equipSlots || [];
      // Every node's position and kind, sent once. State frames carry only
      // the ones that are not at rest.
      state.baseNodes = msg.nodes || [];
      state.nodes = state.baseNodes.map((n) => ({ ...n, ready: true }));
        state.handle = msg.handle || null;
        state.username = msg.username || null;
        if (msg.looksCatalog) state.looksCatalog = msg.looksCatalog;
        if (msg.bankSlots) state.bankSlots = msg.bankSlots;
        state.cam = null;
        state.camSnap = true;
        state.camFocus = state.you
          ? { x: state.you.x + 0.5, y: state.you.y + 0.5 }
          : { x: 8.5, y: 8.5 };
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
        renderEquipped();
        if (state.shop) renderShop();
        if (state.chest) renderChest();
        $("tickinfo").textContent = "tick " + msg.n + " · " + msg.online + " online · " + (msg.ms || 0).toFixed(1) + "ms";
        break;
      case "chat":
        log("say", '<span class="who">' + esc(msg.from) + ":</span> " + esc(msg.text));
        break;
      case "evt":
        log("evt", esc(msg.text));
        if (state.shop) shopMsg(msg.text);
        if (state.chest) chestMsg(msg.text);
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
        closeChest();
        state.shop = msg;
        renderShop();
        $("shop").classList.remove("hidden");
        break;
      case "bank":
        closeShop();
        state.chest = msg;
        if (msg.slots) state.bankSlots = msg.slots;
        renderChest();
        $("chest").classList.remove("hidden");
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

  function closeShop() {
    state.shop = null;
    shopMsg("");
    $("shop").classList.add("hidden");
  }

  $("shop-close").addEventListener("click", closeShop);

  function coinLabel(n) {
    return n === 1 ? "1 coin" : n + " coins";
  }

  function renderSlotGrid(hostId, items, slots, dataBank) {
    const inv = items || [];
    let html = "";
    for (let i = 0; i < slots; i++) {
      const it = inv[i];
      if (!it) {
        html += '<div class="slot empty"></div>';
        continue;
      }
      const info = state.items[it.id] || { name: it.id, glyph: "?" };
      const tool = info.tool ? " tool" : "";
      html +=
        '<div class="slot' + tool + '" data-id="' + esc(it.id) + '" data-slot="' + i +
        (dataBank ? '" data-bank="' + dataBank : "") +
        '" title="' + esc(info.name) +
        '"><div>' + esc(info.glyph) + "</div>" +
        (it.n > 1 ? '<div class="qty">' + it.n + "</div>" : "") +
        "</div>";
    }
    $(hostId).innerHTML = html;
  }

  function renderChest() {
    if (!state.chest) return;
    const you = state.you || {};
    const slots = state.chest.slots || state.bankSlots || 20;
    $("chest-who").textContent = state.chest.name || "The oak chest";
    $("chest-purse").textContent = coinLabel(you.coins || 0);
    $("chest-bankpurse").textContent = coinLabel(you.bankCoins || 0);
    const used = (you.bank || []).length;
    $("chest-used").textContent = used + " / " + slots;
    renderSlotGrid("chest-pack", you.inv, PACK_SLOTS, "deposit");
    renderSlotGrid("chest-slots", you.bank, slots, "withdraw");
    const purseBtns = $("chest").querySelectorAll('[data-id="coins"]');
    for (const btn of purseBtns) {
      const all = btn.dataset.n === "all";
      const n = all
        ? (btn.dataset.bank === "deposit" ? you.coins : you.bankCoins)
        : 1;
      btn.disabled = !n;
    }
  }

  function chestMsg(text) {
    const el = $("chest-msg");
    if (el) el.textContent = text || "";
  }

  function closeChest() {
    state.chest = null;
    chestMsg("");
    $("chest").classList.add("hidden");
  }

  function sendBank(action, id, n) {
    const you = state.you || {};
    let qty = n;
    if (n === "all") {
      qty = id === "coins"
        ? (action === "deposit" ? you.coins : you.bankCoins)
        : 99;
    }
    qty = Number(qty) || 1;
    send({ t: action, id: id, n: qty });
  }

  $("chest").addEventListener("click", (e) => {
    const btn = e.target.closest("[data-bank]");
    if (!btn || btn.disabled) return;
    if (btn.dataset.id) sendBank(btn.dataset.bank, btn.dataset.id, btn.dataset.n || 1);
  });

  $("chest-close").addEventListener("click", closeChest);

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

  // What is worn, and what each slot is for.
  const SLOT_LABEL = { hand: "Hand", body: "Body" };

  function renderEquipped() {
    const el = $("equipped");
    if (!el) return;
    const worn = (state.you && state.you.equipped) || {};
    const slots = state.equipSlots.length ? state.equipSlots : ["hand", "body"];
    el.innerHTML = slots
      .map((slot) => {
        const id = worn[slot];
        const info = id ? state.items[id] || { name: id, glyph: "?" } : null;
        const label = SLOT_LABEL[slot] || slot;
        if (!info) {
          return '<div class="equipslot empty"><span class="where">' + esc(label) + "</span></div>";
        }
        const bonus = [];
        if (info.attack) bonus.push("+" + info.attack + " atk");
        if (info.damage) bonus.push("+" + info.damage + " dmg");
        if (info.defense) bonus.push("+" + info.defense + " def");
        return (
          '<div class="equipslot" data-slot="' + esc(slot) + '" title="' + esc(info.name) +
          (bonus.length ? " — " + bonus.join(", ") : "") + '">' +
          '<span class="what">' + esc(info.glyph) + "</span>" +
          '<span class="where">' + esc(label) + "</span></div>"
        );
      })
      .join("");
  }

  $("equipped").addEventListener("click", (e) => {
    const slot = e.target.closest(".equipslot");
    if (!slot || !slot.dataset.slot) return;
    send({ t: "unequip", id: slot.dataset.slot });
  });

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
    if (state.chest) {
      sendBank("deposit", slot.dataset.id, 1);
      return;
    }
    const info = state.items[slot.dataset.id] || {};
    if (info.slot) {
      send({ t: "equip", id: slot.dataset.id });
      state.heldTool = null;
      return;
    }
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

  function figuresNow() {
    const out = [];
    for (const n of state.npcs) out.push({ kind: "npc", e: n });
    for (const p of state.players) out.push({ kind: "pl", e: p });
    if (state.you) out.push({ kind: "you", e: state.you });
    return out;
  }

  function ensureCam() {
    if (state.cam) return state.cam;
    const I = globalThis.HollowmereIso;
    if (!I) return { x: 0, y: 0 };
    const f = state.camFocus || (state.you
      ? { x: state.you.x + 0.5, y: state.you.y + 0.5 }
      : { x: 8.5, y: 8.5 });
    state.cam = I.cameraFollow(f.x, f.y, canvas.width, canvas.height);
    return state.cam;
  }

  // Canvas pixels → tile through the ¾ chase camera. Gameplay x/y
  // stay the integer coordinates the server already knows.
  function tileAt(mx, my) {
    if (!state.map) return null;
    const r = canvas.getBoundingClientRect();
    const sx = (mx - r.left) * (canvas.width / r.width);
    const sy = (my - r.top) * (canvas.height / r.height);
    const cam = ensureCam();
    const art = globalThis.HollowmereArt;
    if (art && art.hitTile) {
      return art.hitTile(sx, sy, cam, state.map, state.nodes, figuresNow());
    }
    const I = globalThis.HollowmereIso;
    if (!I) return null;
    const t = I.screenToTile(sx, sy, cam);
    if (t.x < 0 || t.y < 0 || t.x >= state.map.w || t.y >= state.map.h) return null;
    return t;
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
    state.cam = null;
    state.camFocus = null;
    state.camSnap = true;
    $("looks").classList.add("hidden");
    closeShop();
    closeChest();
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
    ctx.fillStyle = "#4f8236";
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    const art = globalThis.HollowmereArt;
    const looks = state.draftLooks || defaultDraft();
    if (art && art.drawPaperdoll) {
      art.drawPaperdoll(ctx, canvas.width / 2, canvas.height * 0.82, 20, looks, true);
    }
    ctx.fillStyle = "#3d2412";
    ctx.font = "14px Georgia, serif";
    ctx.textAlign = "center";
    ctx.fillText(state.username || "Wanderer", canvas.width / 2, 26);
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

  function draw() {
    requestAnimationFrame(draw);
    const art = globalThis.HollowmereArt;
    if (!art || !art.drawWorld) {
      ctx.fillStyle = "#1c140c";
      ctx.fillRect(0, 0, canvas.width, canvas.height);
      return;
    }
    const out = art.drawWorld(ctx, canvas, {
      map: state.map,
      you: state.you,
      players: state.players,
      npcs: state.npcs,
      nodes: state.nodes,
      ground: state.ground,
      prevPos: state.prevPos,
      lastTickAt: state.lastTickAt,
      tickMs: state.tickMs,
      now: performance.now(),
      camFocus: state.camFocus,
      snap: state.camSnap,
    });
    state.cam = out.cam;
    state.camFocus = out.focus;
    if (out.focus) state.camSnap = false;
  }

  if (globalThis.HollowmereSprites && HollowmereSprites.load) HollowmereSprites.load();
  draw();
})();
