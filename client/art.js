// ¾ hamlet painter: chase camera + AD sprites + chunky canvas fallbacks.
// Presentation only. Tile (x, y) stays what the server believes.
(function (root) {
  function iso() { return root.HollowmereIso; }
  function spr() { return root.HollowmereSprites; }

  const VILLAGER = {
    Marta: { skin: "olive", hair: "tied", hairColor: "russet", top: "cream" },
    "Old Fen": { skin: "deep", hair: "long", hairColor: "snow", top: "ink" },
    "Wend the Pedlar": { skin: "tan", hair: "tied", hairColor: "straw", top: "clay" },
    Pip: { skin: "fair", hair: "cropped", hairColor: "umber", top: "berry" },
  };

  function defaultLooks() {
    return { skin: "tan", hair: "short", hairColor: "umber", top: "moss" };
  }

  function looksOf(e) {
    if (e && e.looks && e.looks.skin) return e.looks;
    if (e && e.name && VILLAGER[e.name]) return VILLAGER[e.name];
    return defaultLooks();
  }

  function shadeHex(hex, amt) {
    const n = String(hex || "").replace("#", "");
    if (n.length < 6) return hex;
    const k = (i) => Math.max(0, Math.min(255, Math.round(parseInt(n.slice(i, i + 2), 16) * amt)));
    const h = (v) => v.toString(16).padStart(2, "0");
    return "#" + h(k(0)) + h(k(2)) + h(k(4));
  }

  function fillPoly(ctx, pts, fill, stroke, width) {
    if (!pts.length) return;
    ctx.beginPath();
    ctx.moveTo(pts[0].x, pts[0].y);
    for (let i = 1; i < pts.length; i++) ctx.lineTo(pts[i].x, pts[i].y);
    ctx.closePath();
    if (fill) { ctx.fillStyle = fill; ctx.fill(); }
    if (stroke) { ctx.strokeStyle = stroke; ctx.lineWidth = width || 1; ctx.stroke(); }
  }

  function groundFill(glyph, x, y, scars) {
    switch (glyph) {
      case "~": return "#2d5f94";
      case "P": return scars ? "#8a7350" : "#c2a36b";
      case "H":
      case "*": return "#b08968";
      case "#": return "#4a4035";
      case "C":
      case "N":
      case "K":
      case "A": return (x + y) % 2 ? "#6a5a3c" : "#5a4a30";
      default:
        if (scars) return (x + y) % 2 ? "#6e5a40" : "#5c4a34";
        return (x + y) % 2 ? "#5a8f3c" : "#4f8236";
    }
  }

  function drawDiamond(ctx, tx, ty, cam, fill, key, flicker) {
    const I = iso();
    const pts = I.diamond(tx, ty, cam);
    const S = spr();
    const im = S && key && S.ready(key);
    // Flat diamond fill first so gaps never flash through.
    fillPoly(ctx, pts, fill, "rgba(20,12,6,0.28)", 0.8);
    if (im) {
      // AD terrain is a ¾ cube. Plant the cube's top face on this
      // cell's diamond — do not stamp the padded 64×64 canvas at
      // (left, north), which left the grass floating inside the tile.
      const d = S.destAtTopDiamond(
        pts[3].x, pts[0].y, I.TW,
        im.naturalWidth, im.naturalHeight,
        S.boundsOf(key, im.naturalWidth, im.naturalHeight)
      );
      ctx.imageSmoothingEnabled = false;
      ctx.drawImage(im, Math.round(d.x), Math.round(d.y), Math.round(d.w), Math.round(d.h));
    }
    if (key === "terrain/water") {
      ctx.strokeStyle = "rgba(180,220,255," + (0.25 + 0.2 * (flicker || 0)) + ")";
      ctx.lineWidth = 1.2;
      ctx.beginPath();
      ctx.moveTo(pts[3].x + 8, pts[0].y + I.TH * 0.45);
      ctx.quadraticCurveTo(
        (pts[0].x + pts[2].x) / 2,
        pts[0].y + I.TH * (0.32 + 0.1 * (flicker || 0)),
        pts[1].x - 8,
        pts[0].y + I.TH * 0.5
      );
      ctx.stroke();
    }
  }

  // Plant a ¾ plate so its ground diamond matches the cell (or footprint).
  function blitPlate(ctx, key, tx, ty, cam, fw, fh) {
    const I = iso();
    const S = spr();
    const im = S && S.ready(key);
    if (!im) return false;
    const w = fw == null ? 1 : fw;
    const h = fh == null ? 1 : fh;
    const pts = I.footprintDiamond(tx, ty, w, h, cam);
    const d = S.destAtGroundDiamond(
      pts[3].x, pts[0].y, I.TW * w, I.TH * h,
      im.naturalWidth, im.naturalHeight,
      S.boundsOf(key, im.naturalWidth, im.naturalHeight)
    );
    return S.blit(ctx, key, d.x, d.y, d.w, d.h);
  }

  function raiseWall(ctx, tx, ty, cam) {
    const I = iso();
    const h = 28;
    const t = I.toScreen(tx, ty, cam);
    const r = I.toScreen(tx + 1, ty, cam);
    const b = I.toScreen(tx + 1, ty + 1, cam);
    const l = I.toScreen(tx, ty + 1, cam);
    fillPoly(ctx, [
      { x: l.x, y: l.y }, { x: b.x, y: b.y },
      { x: b.x, y: b.y - h }, { x: l.x, y: l.y - h },
    ], "#4a3a28", "rgba(20,12,6,0.35)", 0.8);
    fillPoly(ctx, [
      { x: r.x, y: r.y }, { x: b.x, y: b.y },
      { x: b.x, y: b.y - h }, { x: r.x, y: r.y - h },
    ], "#3a2a1c", "rgba(20,12,6,0.35)", 0.8);
    const top = [
      { x: t.x, y: t.y - h }, { x: r.x, y: r.y - h },
      { x: b.x, y: b.y - h }, { x: l.x, y: l.y - h },
    ];
    const S = spr();
    if (S && S.ready("terrain/wall")) {
      ctx.save();
      ctx.beginPath();
      ctx.moveTo(top[0].x, top[0].y);
      for (let i = 1; i < top.length; i++) ctx.lineTo(top[i].x, top[i].y);
      ctx.closePath();
      ctx.clip();
      ctx.drawImage(S.ready("terrain/wall"), l.x, t.y - h, r.x - l.x, h + (b.y - t.y));
      ctx.restore();
    }
    fillPoly(ctx, top, S && S.ready("terrain/wall") ? null : "#6b5340", "rgba(20,12,6,0.35)", 0.8);
  }

  function drawHair(ctx, hx, hy, r, style, color) {
    ctx.fillStyle = color;
    if (style === "cropped") {
      ctx.beginPath();
      ctx.ellipse(hx, hy - r * 0.55, r * 1.0, r * 0.4, 0, Math.PI, Math.PI * 2, true);
      ctx.fill();
      return;
    }
    ctx.beginPath();
    ctx.ellipse(hx - r * 0.06, hy - r * 0.2, r * 1.1, r * 0.92, -0.12, Math.PI * 1.05, Math.PI * 1.95);
    ctx.fill();
    if (style === "tied") {
      ctx.beginPath();
      ctx.arc(hx + r * 0.92, hy + r * 0.02, r * 0.4, 0, Math.PI * 2);
      ctx.fill();
    }
    if (style === "long") {
      ctx.beginPath();
      ctx.ellipse(hx - r * 0.92, hy + r * 0.5, r * 0.36, r * 0.9, 0.2, 0, Math.PI * 2);
      ctx.fill();
      ctx.beginPath();
      ctx.ellipse(hx + r * 0.7, hy + r * 0.65, r * 0.34, r * 1.0, -0.15, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  // Chunky ¾ paperdoll. AD layers win; otherwise a readable 2007-era doll.
  function drawPaperdoll(ctx, cx, cy, s, looks, highlight) {
    const l = looks || defaultLooks();
    const S = spr();
    const keys = S && S.paperdollKeys(l);
    const u = s / 10;
    const fw = 56 * u;
    const fh = 84 * u;
    if (S && keys) {
      const skinLayer = S.ready(keys.skin) ? keys.skin : keys.bodyMask;
      const tunicLayer = S.ready(keys.tunic) ? keys.tunic : keys.tunicMask;
      const drewSkin = S.ready(keys.skin)
        ? S.blitFeet(ctx, keys.skin, cx, cy, fw, fh)
        : S.blitTint(ctx, skinLayer, cx, cy, fw, fh, keys.skinColor);
      const drewTunic = S.ready(keys.tunic)
        ? S.blitFeet(ctx, keys.tunic, cx, cy, fw, fh)
        : S.blitTint(ctx, tunicLayer, cx, cy, fw, fh, keys.tunicColor);
      const drewHair = S.blitTint(ctx, keys.hair, cx, cy, fw, fh, keys.hairColor);
      if (drewSkin && (drewTunic || drewHair)) {
        if (highlight) {
          ctx.strokeStyle = "rgba(255,244,176,0.75)";
          ctx.lineWidth = Math.max(1.2, u);
          ctx.beginPath();
          ctx.ellipse(cx, cy + 1 * u, 12 * u, 4.2 * u, 0, 0, Math.PI * 2);
          ctx.stroke();
        }
        return;
      }
    }

    const skin = (S && S.SKIN[l.skin]) || "#d4a574";
    const top = (S && S.TOP[l.top]) || "#4a6a32";
    const hair = (S && S.HAIR[l.hairColor]) || "#3d2412";
    ctx.fillStyle = "rgba(20,12,6,0.3)";
    ctx.beginPath();
    ctx.ellipse(cx, cy + 1 * u, 11 * u, 4 * u, 0, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = shadeHex(skin, 0.78);
    ctx.fillRect(cx - 6 * u, cy - 9 * u, 4.4 * u, 11 * u);
    ctx.fillStyle = shadeHex(skin, 0.92);
    ctx.fillRect(cx + 1.4 * u, cy - 8 * u, 4.8 * u, 12 * u);
    ctx.fillStyle = shadeHex(top, 0.75);
    ctx.fillRect(cx - 8 * u, cy - 21 * u, 16 * u, 14 * u);
    ctx.fillStyle = top;
    ctx.fillRect(cx - 7 * u, cy - 22 * u, 14.4 * u, 13 * u);
    ctx.fillStyle = shadeHex(top, 0.55);
    ctx.fillRect(cx - 6 * u, cy - 10.5 * u, 12 * u, 2 * u);
    ctx.fillStyle = shadeHex(skin, 0.8);
    ctx.fillRect(cx - 9.2 * u, cy - 20 * u, 3 * u, 9 * u);
    ctx.fillStyle = skin;
    ctx.fillRect(cx + 6.6 * u, cy - 19 * u, 3.2 * u, 10 * u);
    if (highlight) {
      ctx.strokeStyle = "rgba(255,244,176,0.7)";
      ctx.lineWidth = Math.max(1, u);
      ctx.beginPath();
      ctx.ellipse(cx, cy + 1 * u, 12 * u, 4.4 * u, 0, 0, Math.PI * 2);
      ctx.stroke();
    }
    const hx = cx + 0.5 * u;
    const hy = cy - 28 * u;
    const hr = 6.8 * u;
    ctx.fillStyle = skin;
    ctx.beginPath();
    ctx.arc(hx, hy, hr, 0, Math.PI * 2);
    ctx.fill();
    drawHair(ctx, hx, hy, hr, l.hair || "short", hair);
    ctx.fillStyle = "#2a160c";
    ctx.fillRect(hx - 2.6 * u, hy - 1.2 * u, 1.6 * u, 1.8 * u);
    ctx.fillRect(hx + 1.2 * u, hy - 1.4 * u, 1.6 * u, 1.8 * u);
  }

  function drawHostileFallback(ctx, cx, cy, s, name) {
    const big = /brambleback/i.test(name || "");
    const u = s / 10;
    ctx.fillStyle = "rgba(20,12,6,0.3)";
    ctx.beginPath();
    ctx.ellipse(cx, cy + 1 * u, (big ? 16 : 11) * u, (big ? 6 : 4) * u, 0, 0, Math.PI * 2);
    ctx.fill();
    if (big) {
      ctx.fillStyle = "#3a2a18";
      ctx.fillRect(cx - 16 * u, cy - 22 * u, 30 * u, 18 * u);
      ctx.fillStyle = "#4a3420";
      ctx.fillRect(cx + 8 * u, cy - 28 * u, 14 * u, 12 * u);
      ctx.fillStyle = "#5a2a18";
      ctx.fillRect(cx + 4 * u, cy - 36 * u, 4 * u, 10 * u);
      ctx.fillRect(cx - 4 * u, cy - 32 * u, 3 * u, 8 * u);
      return;
    }
    ctx.fillStyle = "#3a4a22";
    ctx.fillRect(cx - 8 * u, cy - 20 * u, 16 * u, 18 * u);
    ctx.fillStyle = "#4a5a28";
    ctx.beginPath();
    ctx.arc(cx, cy - 24 * u, 6.4 * u, 0, Math.PI * 2);
    ctx.fill();
    ctx.fillStyle = "#5a2a18";
    ctx.fillRect(cx - 1.4 * u, cy - 36 * u, 2.8 * u, 10 * u);
    ctx.fillRect(cx - 7 * u, cy - 30 * u, 2.4 * u, 8 * u);
    ctx.fillRect(cx + 5 * u, cy - 30 * u, 2.4 * u, 8 * u);
  }

  function drawNameplate(ctx, text, x, y, color) {
    ctx.font = "bold 12px Georgia, serif";
    ctx.textAlign = "center";
    const w = ctx.measureText(text).width;
    ctx.fillStyle = "rgba(42,22,12,0.62)";
    ctx.fillRect(x - w / 2 - 5, y - 12, w + 10, 15);
    ctx.fillStyle = color;
    ctx.fillText(text, x, y);
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

  function lerp(a, b, u) { return a + (b - a) * u; }

  function posOf(prevPos, id, x, y, u) {
    const p = prevPos && prevPos[id];
    if (!p) return { x, y };
    return { x: lerp(p.px, x, u), y: lerp(p.py, y, u) };
  }

  function spriteBox(kind, extra) {
    switch (kind) {
      case "tree": return { w: 56, h: 88 };
      case "stump": return { w: 40, h: 28 };
      case "bramble":
      case "hazel": return { w: 48, h: 52 };
      case "millstone": return { w: 52, h: 48 };
      case "oak_chest": return { w: 48, h: 44 };
      case "kiln": return { w: 52, h: 72 };
      case "anvil": return { w: 48, h: 40 };
      case "hearth": return { w: 52, h: 64 };
      case "stile": return { w: 72, h: 56 };
      case "house": return { w: 192, h: 192 };
      case "pedlar_stall": return { w: 56, h: 64 };
      case "ore": return { w: 44, h: 36 };
      case "wall": return { w: 56, h: 40 };
      case "figure": return { w: 36, h: 72 };
      case "hostile": return { w: extra && extra.big ? 48 : 36, h: extra && extra.big ? 80 : 72 };
      default: return { w: 32, h: 32 };
    }
  }

  function hitTile(sx, sy, cam, map, nodes, figures) {
    const I = iso();
    const ground = I.screenToTile(sx, sy, cam);
    const hits = [];
    const consider = (tx, ty, kind, extra) => {
      const spec = spriteBox(kind, extra);
      const feet = I.tileFeet(tx, ty, cam);
      if (sx >= feet.x - spec.w / 2 && sx <= feet.x + spec.w / 2 &&
          sy >= feet.y - spec.h && sy <= feet.y + 6) {
        hits.push({ x: tx, y: ty, depth: tx + ty, h: spec.h });
      }
    };
    for (const n of nodes || []) {
      const kind = n.kind === "fire" && n.id === "fire-1" ? "hearth"
        : n.kind === "mill" ? "millstone"
        : n.kind === "chest" ? "oak_chest"
        : n.kind === "bush" ? "bramble"
        : n.kind === "copper" || n.kind === "tin" ? "ore"
        : n.kind === "tree" && !n.ready ? "stump"
        : n.kind;
      consider(n.x, n.y, kind, n);
    }
    consider(8, 8, "stile");
    {
      const H = I.HOUSE;
      const spec = spriteBox("house");
      const south = I.toScreen(H.x + H.w, H.y + H.h, cam);
      if (sx >= south.x - spec.w / 2 && sx <= south.x + spec.w / 2 &&
          sy >= south.y - spec.h && sy <= south.y + 6) {
        hits.push({ x: H.idX, y: H.idY, depth: H.idX + H.idY, h: spec.h });
      }
    }
    consider(10, 5, "pedlar_stall");
    for (const f of figures || []) {
      const e = f.e || f;
      consider(e.x, e.y, f.kind === "npc" && e.hostile ? "hostile" : "figure", {
        big: /brambleback/i.test(e.name || ""),
      });
    }
    if (hits.length) {
      hits.sort((a, b) => (b.depth - a.depth) || (b.h - a.h));
      return { x: Math.round(hits[0].x), y: Math.round(hits[0].y) };
    }
    if (!map) return null;
    if (ground.x < 0 || ground.y < 0 || ground.x >= map.w || ground.y >= map.h) return null;
    return ground;
  }

  function drawWorld(ctx, canvas, view) {
    const I = iso();
    const S = spr();
    const m = view.map;
    if (!m || !I) {
      ctx.fillStyle = "#1c140c";
      ctx.fillRect(0, 0, canvas.width, canvas.height);
      ctx.fillStyle = "#f3e2c7";
      ctx.font = "16px Georgia, serif";
      ctx.fillText("Waiting at the stile…", 24, 40);
      return { cam: null, focus: null };
    }

    const now = view.now || 0;
    const u = Math.min(1, (now - (view.lastTickAt || 0)) / (view.tickMs || 600));
    const flicker = 0.5 + 0.5 * Math.sin(now / 90);

    let target = { x: 8.5, y: 8.5 };
    if (view.you) {
      const p = posOf(view.prevPos, view.you.id, view.you.x, view.you.y, u);
      const prev = view.prevPos && view.prevPos[view.you.id];
      const dx = prev ? p.x - prev.px : 0;
      const dy = prev ? p.y - prev.py : 0;
      target = { x: p.x + 0.5 + dx * 0.35, y: p.y + 0.5 + dy * 0.35 };
    }
    const focus = I.chase(view.camFocus, target, view.snap ? 1 : 0.14);
    const cam = I.cameraFollow(focus.x, focus.y, canvas.width, canvas.height);

    ctx.imageSmoothingEnabled = false;
    ctx.fillStyle = "#14100c";
    ctx.fillRect(0, 0, canvas.width, canvas.height);

    const spentTree = {};
    for (const n of view.nodes || []) {
      if (n.kind === "tree" && !n.ready) spentTree[n.x + "," + n.y] = true;
    }

    const ground = [];
    for (let y = 0; y < m.h; y++) {
      const row = m.tiles[y] || "";
      for (let x = 0; x < m.w; x++) {
        const feet = I.tileFeet(x, y, cam);
        if (!I.inView(feet.x, feet.y, canvas.width, canvas.height, 110)) continue;
        ground.push({ x, y, g: row[x] || "." });
      }
    }
    ground.sort((a, b) => (a.x + a.y) - (b.x + b.y) || a.y - b.y || a.x - b.x);
    for (const t of ground) {
      const scars = t.x >= 27;
      const key = S ? S.tileKey(t.g === "T" ? "." : t.g, scars) : "";
      drawDiamond(ctx, t.x, t.y, cam, groundFill(t.g, t.x, t.y, scars), key, flicker);
    }

    const sprites = [];
    sprites.push({ x: 8, y: 8, depth: 16, kind: "stile" });
    sprites.push({ x: I.HOUSE.x, y: I.HOUSE.y, depth: I.HOUSE.x + I.HOUSE.y + I.HOUSE.w + I.HOUSE.h - 2, kind: "house" });
    sprites.push({ x: 10, y: 5, depth: 15, kind: "pedlar_stall" });

    for (let y = 0; y < m.h; y++) {
      const row = m.tiles[y] || "";
      for (let x = 0; x < m.w; x++) {
        const g = row[x] || ".";
        if (g === "#") sprites.push({ x, y, depth: x + y, kind: "wall" });
        if (g === "T") sprites.push({ x, y, depth: x + y, kind: "tree", ready: !spentTree[x + "," + y] });
      }
    }

    for (const n of view.nodes || []) {
      sprites.push({ x: n.x, y: n.y, depth: n.x + n.y, kind: "node", node: n });
    }
    for (const g of view.ground || []) {
      sprites.push({ x: g.x, y: g.y, depth: g.x + g.y + 0.2, kind: "pile", pile: g });
    }

    const figures = [];
    for (const n of view.npcs || []) figures.push({ kind: "npc", e: n });
    for (const p of view.players || []) figures.push({ kind: "pl", e: p });
    if (view.you) figures.push({ kind: "you", e: view.you });
    for (const f of figures) {
      const p = posOf(view.prevPos, f.e.id, f.e.x, f.e.y, u);
      sprites.push({ x: p.x, y: p.y, depth: p.x + p.y + 0.4, kind: "figure", fig: f });
    }

    sprites.sort((a, b) => a.depth - b.depth || a.y - b.y || a.x - b.x);
    const targetId = view.you && view.you.target;

    for (const s of sprites) {
      const feet = I.tileFeet(s.x, s.y, cam);
      if (!I.inView(feet.x, feet.y, canvas.width, canvas.height, 140)) continue;

      if (s.kind === "wall") {
        if (!(S && S.ready("terrain/wall"))) raiseWall(ctx, s.x, s.y, cam);
        continue;
      }
      if (s.kind === "house") {
        const H = I.HOUSE;
        if (!(blitPlate(ctx, "buildings/house", H.x, H.y, cam, H.w, H.h) ||
              blitPlate(ctx, "props/house", H.x, H.y, cam, H.w, H.h))) {
          const south = I.toScreen(H.x + H.w, H.y + H.h, cam);
          ctx.fillStyle = "#6b4423";
          ctx.fillRect(south.x - 40, south.y - 70, 80, 54);
          ctx.fillStyle = "#8a5a28";
          ctx.beginPath();
          ctx.moveTo(south.x - 46, south.y - 68);
          ctx.lineTo(south.x, south.y - 98);
          ctx.lineTo(south.x + 46, south.y - 68);
          ctx.fill();
          ctx.fillStyle = "#2a160c";
          ctx.fillRect(south.x - 8, south.y - 44, 14, 20);
        }
        continue;
      }
      if (s.kind === "stile") {
        blitPlate(ctx, "props/stile", s.x, s.y, cam);
        continue;
      }
      if (s.kind === "pedlar_stall") {
        blitPlate(ctx, "props/pedlar_stall", s.x, s.y, cam);
        continue;
      }
      if (s.kind === "tree") {
        if (s.ready === false) {
          if (!blitPlate(ctx, "props/stump", s.x, s.y, cam)) {
            ctx.fillStyle = "#6b3e1a";
            ctx.beginPath();
            ctx.ellipse(feet.x, feet.y - 4, 10, 5, 0, 0, Math.PI * 2);
            ctx.fill();
          }
        } else if (!blitPlate(ctx, "props/tree", s.x, s.y, cam)) {
          ctx.fillStyle = "#6b3e1a";
          ctx.fillRect(feet.x - 4, feet.y - 36, 8, 36);
          ctx.fillStyle = "#245218";
          ctx.beginPath();
          ctx.arc(feet.x, feet.y - 48, 20, 0, Math.PI * 2);
          ctx.fill();
        }
        continue;
      }
      if (s.kind === "pile") {
        const g = s.pile;
        ctx.fillStyle = "rgba(0,0,0,0.25)";
        ctx.beginPath();
        ctx.ellipse(feet.x, feet.y, 12, 5, 0, 0, Math.PI * 2);
        ctx.fill();
        ctx.fillStyle = "#6b4423";
        ctx.beginPath();
        ctx.moveTo(feet.x - 9, feet.y);
        ctx.lineTo(feet.x - 6, feet.y - 14);
        ctx.lineTo(feet.x + 6, feet.y - 14);
        ctx.lineTo(feet.x + 9, feet.y);
        ctx.fill();
        if (g.mine) {
          ctx.strokeStyle = "rgba(250,225,150," + (0.45 + 0.3 * flicker) + ")";
          ctx.beginPath();
          ctx.ellipse(feet.x, feet.y, 14, 6, 0, 0, Math.PI * 2);
          ctx.stroke();
        }
        if (g.coins > 0) {
          ctx.fillStyle = "hsl(45,80%," + Math.floor(50 + 12 * flicker) + "%)";
          ctx.beginPath();
          ctx.arc(feet.x, feet.y - 10, 3, 0, Math.PI * 2);
          ctx.fill();
        }
        continue;
      }
      if (s.kind === "node") {
        const n = s.node;
        const key = S && S.nodeKey(n);
        // Map glyph T already drew the ready tree or the stump.
        if (n.kind === "tree") continue;
        // fire-1 is the hearth inside the 2×2 cottage. Drawing the
        // plate here parks the flames on the thatch; the house hit
        // still reports (4, 5) so cooking clicks keep working.
        if (n.id === "fire-1") continue;
        const drew = key && blitPlate(ctx, key, n.x, n.y, cam);
        if (!drew) {
          if (n.kind === "bush") {
            ctx.fillStyle = n.ready ? "#2f5a22" : "#3a4a30";
            ctx.beginPath();
            ctx.ellipse(feet.x, feet.y - 8, 16, 10, 0, 0, Math.PI * 2);
            ctx.fill();
            if (n.ready) {
              ctx.fillStyle = "#bb3333";
              ctx.fillRect(feet.x - 6, feet.y - 12, 4, 4);
            }
          } else if (n.kind === "hazel") {
            ctx.fillStyle = "#6b3e1a";
            ctx.fillRect(feet.x - 2, feet.y - 26, 4, 24);
            ctx.fillStyle = n.ready ? "#3d5a22" : "#3a4a30";
            ctx.beginPath();
            ctx.ellipse(feet.x, feet.y - 28, 13, 11, 0, 0, Math.PI * 2);
            ctx.fill();
          } else if (n.kind === "mill") {
            ctx.fillStyle = "#8a8070";
            ctx.beginPath();
            ctx.ellipse(feet.x, feet.y - 8, 16, 7, 0, 0, Math.PI * 2);
            ctx.fill();
          } else if (n.kind === "chest") {
            ctx.fillStyle = "#6b3e1a";
            ctx.fillRect(feet.x - 14, feet.y - 22, 28, 18);
            ctx.fillStyle = "#c4a36a";
            ctx.fillRect(feet.x - 3, feet.y - 14, 6, 5);
          } else if (n.kind === "kiln") {
            ctx.fillStyle = "#3a2a22";
            ctx.fillRect(feet.x - 12, feet.y - 44, 24, 40);
            ctx.fillStyle = "rgba(255," + Math.floor(90 + 70 * flicker) + ",20,0.95)";
            ctx.fillRect(feet.x - 6, feet.y - 28, 12, 12);
          } else if (n.kind === "anvil") {
            ctx.fillStyle = "#8a8a8a";
            ctx.fillRect(feet.x - 16, feet.y - 26, 32, 8);
            ctx.fillStyle = "#2a2a2a";
            ctx.fillRect(feet.x - 6, feet.y - 18, 12, 14);
          } else if (n.kind === "copper" || n.kind === "tin") {
            ctx.fillStyle = n.ready ? "#6a5340" : "#4a4035";
            ctx.beginPath();
            ctx.moveTo(feet.x - 14, feet.y);
            ctx.lineTo(feet.x - 6, feet.y - 16);
            ctx.lineTo(feet.x + 12, feet.y);
            ctx.fill();
            if (n.ready) {
              ctx.fillStyle = n.kind === "copper" ? "#c46a32" : "#c8c4b0";
              ctx.fillRect(feet.x - 2, feet.y - 8, 4, 4);
            }
          } else if (n.kind === "fire") {
            ctx.fillStyle = "#5a3a18";
            ctx.fillRect(feet.x - 12, feet.y - 6, 24, 5);
            ctx.fillStyle = "rgba(255," + Math.floor(120 + 80 * flicker) + ",20,0.95)";
            ctx.beginPath();
            ctx.moveTo(feet.x, feet.y - 28);
            ctx.lineTo(feet.x - 10, feet.y - 4);
            ctx.lineTo(feet.x + 10, feet.y - 4);
            ctx.fill();
          } else if (n.kind === "tree" && !n.ready) {
            ctx.fillStyle = "#6b3e1a";
            ctx.beginPath();
            ctx.ellipse(feet.x, feet.y - 4, 10, 5, 0, 0, Math.PI * 2);
            ctx.fill();
          }
        }
        if (n.kind === "fire" && n.burns > 0) {
          const life = Math.max(0, Math.min(1, n.burns / 150));
          ctx.fillStyle = "rgba(40,24,12,0.75)";
          ctx.fillRect(feet.x - 14, feet.y + 2, 28, 3);
          ctx.fillStyle = "hsl(28,80%,55%)";
          ctx.fillRect(feet.x - 14, feet.y + 2, 28 * life, 3);
        }
        continue;
      }
      if (s.kind === "figure") {
        const f = s.fig;
        const e = f.e;
        const hostile = f.kind === "npc" && e.hostile;
        if (e.id && e.id === targetId) {
          ctx.strokeStyle = "rgba(190,50,30,0.85)";
          ctx.lineWidth = 2;
          ctx.beginPath();
          ctx.ellipse(feet.x, feet.y + 2, 14, 6, 0, 0, Math.PI * 2);
          ctx.stroke();
        }
        const nkey = S && S.npcKey(e);
        const drewNpc = nkey && S.blitFeet(ctx, nkey, feet.x, feet.y, 64, 96);
        if (!drewNpc) {
          if (hostile) drawHostileFallback(ctx, feet.x, feet.y, 14, e.name);
          else drawPaperdoll(ctx, feet.x, feet.y, 14, looksOf(e), f.kind === "you");
        } else if (f.kind === "you") {
          ctx.strokeStyle = "rgba(255,244,176,0.7)";
          ctx.beginPath();
          ctx.ellipse(feet.x, feet.y + 2, 12, 4.4, 0, 0, Math.PI * 2);
          ctx.stroke();
        }
        const ink = f.kind === "you" ? "#fff4b0" : hostile ? "#f0c8a0" : "#f3e2c7";
        drawNameplate(ctx, e.name || "?", feet.x, feet.y - 52, ink);
        if (e.maxHp > 0 && (f.kind === "you" || hostile)) {
          const hp = e.hp == null ? e.maxHp : e.hp;
          const bw = 22;
          const bx = feet.x - bw / 2;
          const by = feet.y - 62;
          ctx.fillStyle = "#2a160c";
          ctx.fillRect(bx - 1, by - 1, bw + 2, 5);
          ctx.fillStyle = hostile ? "#c44" : "#6a3";
          ctx.fillRect(bx, by, bw * Math.max(0, Math.min(1, hp / e.maxHp)), 3);
        }
        if (e.action && e.action !== "idle" && e.action !== "walk") {
          ctx.fillStyle = "#fff4b0";
          ctx.font = "11px Georgia, serif";
          ctx.textAlign = "center";
          ctx.fillText(actionVoice(e.action), feet.x, feet.y + 16);
        }
      }
    }

    return { cam, focus };
  }

  root.HollowmereArt = {
    drawWorld,
    drawPaperdoll,
    hitTile,
    looksOf,
    defaultLooks,
    actionVoice,
  };
  if (typeof module !== "undefined" && module.exports) {
    module.exports = root.HollowmereArt;
  }
})(typeof globalThis !== "undefined" ? globalThis : this);
