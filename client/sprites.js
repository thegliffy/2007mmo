// AD production pack loader. Paths match client/assets/manifest.json.
// Each entry tries the AD file first, then a legacy placeholder if one
// still lives under tiles/ or the old props/ names. Missing files stay
// unloaded so the ¾ painter can draw a chunky canvas fallback.
(function (root) {
  const PACK = [
    { key: "terrain/grass", src: "assets/terrain/grass.png", fallback: "assets/tiles/grass.png" },
    { key: "terrain/path", src: "assets/terrain/path.png", fallback: "assets/tiles/path.png" },
    { key: "terrain/water", src: "assets/terrain/water.png", fallback: "assets/tiles/water.png" },
    { key: "terrain/scar", src: "assets/terrain/scar.png", fallback: "assets/tiles/grass-scar.png" },
    { key: "terrain/wall", src: "assets/terrain/wall.png", fallback: "assets/tiles/wall.png" },
    { key: "buildings/house", src: "assets/buildings/house.png", fallback: "assets/props/house.png" },
    { key: "props/house", src: "assets/props/house.png" },
    { key: "props/hearth", src: "assets/props/hearth.png" },
    { key: "props/stile", src: "assets/props/stile.png" },
    { key: "props/millstone", src: "assets/props/millstone.png", fallback: "assets/props/mill.png" },
    { key: "props/oak_chest", src: "assets/props/oak_chest.png", fallback: "assets/props/chest.png" },
    { key: "props/kiln", src: "assets/props/kiln.png" },
    { key: "props/anvil", src: "assets/props/anvil.png" },
    { key: "props/pedlar_stall", src: "assets/props/pedlar_stall.png" },
    { key: "props/tree", src: "assets/props/tree.png" },
    { key: "props/stump", src: "assets/props/stump.png" },
    { key: "gather/bramble", src: "assets/gather/bramble.png", fallback: "assets/props/bush.png" },
    { key: "gather/hazel", src: "assets/gather/hazel.png", fallback: "assets/props/hazel.png" },
    { key: "gather/ore_copper", src: "assets/gather/ore_copper.png", fallback: "assets/props/copper.png" },
    { key: "gather/ore_tin", src: "assets/gather/ore_tin.png", fallback: "assets/props/tin.png" },
    { key: "hostiles/thornkin", src: "assets/hostiles/thornkin.png" },
    { key: "hostiles/brambleback", src: "assets/hostiles/brambleback.png" },
    { key: "npcs/marta", src: "assets/npcs/marta.png" },
    { key: "npcs/old_fen", src: "assets/npcs/old_fen.png" },
    { key: "npcs/pip", src: "assets/npcs/pip.png" },
    { key: "npcs/wend", src: "assets/npcs/wend.png" },
    { key: "paperdoll/skin_fair", src: "assets/paperdoll/skin_fair.png" },
    { key: "paperdoll/skin_tan", src: "assets/paperdoll/skin_tan.png" },
    { key: "paperdoll/skin_olive", src: "assets/paperdoll/skin_olive.png" },
    { key: "paperdoll/skin_deep", src: "assets/paperdoll/skin_deep.png" },
    { key: "paperdoll/tunic_moss", src: "assets/paperdoll/tunic_moss.png" },
    { key: "paperdoll/tunic_clay", src: "assets/paperdoll/tunic_clay.png" },
    { key: "paperdoll/tunic_ink", src: "assets/paperdoll/tunic_ink.png" },
    { key: "paperdoll/tunic_cream", src: "assets/paperdoll/tunic_cream.png" },
    { key: "paperdoll/tunic_berry", src: "assets/paperdoll/tunic_berry.png" },
    { key: "paperdoll/hair_cropped_mask", src: "assets/paperdoll/hair_cropped_mask.png" },
    { key: "paperdoll/hair_short_mask", src: "assets/paperdoll/hair_short_mask.png" },
    { key: "paperdoll/hair_tied_mask", src: "assets/paperdoll/hair_tied_mask.png" },
    { key: "paperdoll/hair_long_mask", src: "assets/paperdoll/hair_long_mask.png" },
    { key: "paperdoll/body_mask", src: "assets/paperdoll/body_mask.png" },
    { key: "paperdoll/tunic_mask", src: "assets/paperdoll/tunic_mask.png" },
    { key: "paperdoll/preview_default_looks", src: "assets/paperdoll/preview_default_looks.png" },
  ];

  const MANIFEST = {};
  PACK.forEach((p) => { MANIFEST[p.key] = p.src; });

  const images = {};
  const measured = {};

  // Opaque bbox in source pixels. Seeded from the AD pack so the first
  // paint (and node tests) plant plates before a canvas measure runs.
  // load() overwrites these from the decoded bitmap when a document exists.
  const BOUNDS = {
    "terrain/grass": { x: 1, y: 11, w: 62, h: 42 },
    "terrain/path": { x: 1, y: 11, w: 61, h: 41 },
    "terrain/water": { x: 1, y: 11, w: 62, h: 42 },
    "terrain/scar": { x: 1, y: 10, w: 62, h: 44 },
    "terrain/wall": { x: 1, y: 6, w: 61, h: 52 },
    "buildings/house": { x: 2, y: 18, w: 188, h: 154 },
    "props/house": { x: 1, y: 9, w: 94, h: 77 },
    "props/hearth": { x: 2, y: 1, w: 92, h: 93 },
    "props/stile": { x: 1, y: 12, w: 93, h: 71 },
    "props/millstone": { x: 1, y: 2, w: 93, h: 91 },
    "props/oak_chest": { x: 1, y: 6, w: 94, h: 83 },
    "props/kiln": { x: 1, y: 7, w: 94, h: 82 },
    "props/anvil": { x: 1, y: 12, w: 93, h: 71 },
    "props/pedlar_stall": { x: 7, y: 1, w: 80, h: 94 },
    "props/tree": { x: 8, y: 1, w: 80, h: 94 },
    "props/stump": { x: 1, y: 7, w: 94, h: 82 },
    "gather/bramble": { x: 1, y: 11, w: 94, h: 74 },
    "gather/hazel": { x: 1, y: 12, w: 94, h: 73 },
    "gather/ore_copper": { x: 1, y: 9, w: 94, h: 78 },
    "gather/ore_tin": { x: 3, y: 11, w: 92, h: 74 },
    "hostiles/thornkin": { x: 1, y: 28, w: 42, h: 39 },
    "hostiles/brambleback": { x: 1, y: 15, w: 62, h: 65 },
    "npcs/marta": { x: 3, y: 1, w: 58, h: 93 },
    "npcs/old_fen": { x: 1, y: 7, w: 62, h: 81 },
    "npcs/pip": { x: 2, y: 1, w: 59, h: 94 },
    "npcs/wend": { x: 1, y: 9, w: 62, h: 76 },
    "paperdoll/skin_tan": { x: 24, y: 1, w: 80, h: 190 },
    "paperdoll/skin_fair": { x: 24, y: 1, w: 80, h: 190 },
    "paperdoll/skin_olive": { x: 24, y: 1, w: 80, h: 190 },
    "paperdoll/skin_deep": { x: 24, y: 1, w: 80, h: 190 },
    "paperdoll/body_mask": { x: 24, y: 1, w: 80, h: 190 },
  };

  function boundsOf(key, imgW, imgH) {
    const b = measured[key] || BOUNDS[key];
    if (b && b.w > 0 && b.h > 0) return b;
    const w = imgW || 0, h = imgH || 0;
    return { x: 0, y: 0, w: w, h: h };
  }

  function measureImage(img) {
    if (typeof document === "undefined" || !img || !img.naturalWidth) return null;
    const c = document.createElement("canvas");
    c.width = img.naturalWidth;
    c.height = img.naturalHeight;
    const g = c.getContext("2d");
    if (!g) return null;
    g.drawImage(img, 0, 0);
    let data;
    try { data = g.getImageData(0, 0, c.width, c.height).data; }
    catch { return null; }
    let minx = c.width, miny = c.height, maxx = -1, maxy = -1;
    for (let y = 0; y < c.height; y++) {
      for (let x = 0; x < c.width; x++) {
        if (data[(y * c.width + x) * 4 + 3] > 16) {
          if (x < minx) minx = x;
          if (y < miny) miny = y;
          if (x > maxx) maxx = x;
          if (y > maxy) maxy = y;
        }
      }
    }
    if (maxx < 0) return { x: 0, y: 0, w: img.naturalWidth, h: img.naturalHeight };
    return { x: minx, y: miny, w: maxx - minx + 1, h: maxy - miny + 1 };
  }

  function loadOne(key, src, fallback) {
    const img = new Image();
    img.onload = () => {
      images[key] = img;
      const b = measureImage(img);
      if (b) measured[key] = b;
    };
    img.onerror = () => {
      if (fallback && fallback !== src) loadOne(key, fallback, null);
      else images[key] = null;
    };
    img.src = src;
  }

  function load() {
    PACK.forEach((p) => {
      if (images[p.key]) return;
      loadOne(p.key, p.src, p.fallback || null);
    });
  }

  function ready(key) {
    const im = images[key];
    return im && im.naturalWidth ? im : null;
  }

  function blit(ctx, key, x, y, w, h) {
    const im = ready(key);
    if (!im) return false;
    ctx.imageSmoothingEnabled = false;
    ctx.drawImage(im, Math.round(x), Math.round(y), Math.round(w), Math.round(h));
    return true;
  }

  // Terrain cube: opaque top-left is the north-west of the top face.
  // Scale that face to tileW and sit it on the logical diamond.
  function destAtTopDiamond(left, north, tileW, imgW, imgH, bounds) {
    const b = bounds && bounds.w ? bounds : { x: 0, y: 0, w: imgW, h: imgH };
    const scale = tileW / Math.max(1, b.w);
    return {
      x: left - b.x * scale,
      y: north - b.y * scale,
      w: imgW * scale,
      h: imgH * scale,
    };
  }

  // ¾ prop plate: a 2:1 ground diamond of width = opaque width sits on
  // the opaque bottom. Scale that diamond onto the tile diamond
  // (left, north) · (tileW × tileH).
  function destAtGroundDiamond(left, north, tileW, tileH, imgW, imgH, bounds) {
    const b = bounds && bounds.w ? bounds : { x: 0, y: 0, w: imgW, h: imgH };
    const scale = tileW / Math.max(1, b.w);
    const srcDiamondNorth = (b.y + b.h) - b.w / 2;
    return {
      x: left - b.x * scale,
      y: north - srcDiamondNorth * scale,
      w: imgW * scale,
      h: imgH * scale,
    };
  }

  // Figure / paperdoll: opaque bottom-centre maps to the world foot.
  function destAtFeet(feetX, feetY, destW, destH, imgW, imgH, bounds) {
    const b = bounds && bounds.w ? bounds : { x: 0, y: 0, w: imgW || destW, h: imgH || destH };
    const iw = imgW || destW, ih = imgH || destH;
    const sx = destW / Math.max(1, iw);
    const sy = destH / Math.max(1, ih);
    return {
      x: feetX - (b.x + b.w / 2) * sx,
      y: feetY - (b.y + b.h) * sy,
      w: destW,
      h: destH,
    };
  }

  function blitFeet(ctx, key, feetX, feetY, w, h) {
    const im = ready(key);
    if (!im) return false;
    const d = destAtFeet(feetX, feetY, w, h, im.naturalWidth, im.naturalHeight, boundsOf(key, im.naturalWidth, im.naturalHeight));
    return blit(ctx, key, d.x, d.y, d.w, d.h);
  }

  const scratch = typeof document !== "undefined" ? document.createElement("canvas") : null;

  function blitTint(ctx, key, feetX, feetY, w, h, color) {
    const im = ready(key);
    if (!im || !scratch) return false;
    const d = destAtFeet(feetX, feetY, w, h, im.naturalWidth, im.naturalHeight, boundsOf(key, im.naturalWidth, im.naturalHeight));
    scratch.width = Math.max(1, Math.ceil(d.w));
    scratch.height = Math.max(1, Math.ceil(d.h));
    const s = scratch.getContext("2d");
    s.clearRect(0, 0, scratch.width, scratch.height);
    s.drawImage(im, 0, 0, scratch.width, scratch.height);
    if (color) {
      s.globalCompositeOperation = "source-in";
      s.fillStyle = color;
      s.fillRect(0, 0, scratch.width, scratch.height);
    }
    ctx.drawImage(scratch, Math.round(d.x), Math.round(d.y));
    return true;
  }

  function tileKey(glyph, scars) {
    switch (glyph) {
      case "~": return "terrain/water";
      case "P": return scars ? "terrain/scar" : "terrain/path";
      case "#": return "terrain/wall";
      case "C":
      case "N":
      case "K":
      case "A": return "terrain/scar";
      default: return scars ? "terrain/scar" : "terrain/grass";
    }
  }

  function nodeKey(n) {
    if (!n) return "";
    if (n.kind === "bush") return "gather/bramble";
    if (n.kind === "hazel") return "gather/hazel";
    if (n.kind === "tree") return n.ready ? "" : "props/stump";
    if (n.kind === "mill") return "props/millstone";
    if (n.kind === "chest") return "props/oak_chest";
    if (n.kind === "kiln") return "props/kiln";
    if (n.kind === "anvil") return "props/anvil";
    if (n.kind === "copper") return "gather/ore_copper";
    if (n.kind === "tin") return "gather/ore_tin";
    if (n.kind === "fire") return n.id === "fire-1" ? "props/hearth" : "";
    return "";
  }

  function npcKey(e) {
    if (!e) return "";
    const name = (e.name || "").toLowerCase();
    if (name.indexOf("brambleback") >= 0) return "hostiles/brambleback";
    if (e.hostile) return "hostiles/thornkin";
    if (name === "marta") return "npcs/marta";
    if (name === "old fen") return "npcs/old_fen";
    if (name === "pip") return "npcs/pip";
    if (name.indexOf("wend") >= 0) return "npcs/wend";
    return "";
  }

  const HAIR = { umber: "#3d2412", straw: "#d4b46a", soot: "#1c140c", russet: "#8a3a1c", snow: "#e8e0d0" };
  const SKIN = { fair: "#f0d2b0", tan: "#d4a574", olive: "#b08a58", deep: "#6b4226" };
  const TOP = { moss: "#4a6a32", clay: "#a85a32", ink: "#2a3a5a", cream: "#e8d4a8", berry: "#7a2a40" };

  function paperdollKeys(looks) {
    const l = looks || {};
    return {
      skin: "paperdoll/skin_" + (l.skin || "tan"),
      tunic: "paperdoll/tunic_" + (l.top || "moss"),
      hair: "paperdoll/hair_" + (l.hair || "short") + "_mask",
      bodyMask: "paperdoll/body_mask",
      tunicMask: "paperdoll/tunic_mask",
      hairColor: HAIR[l.hairColor] || "#3d2412",
      skinColor: SKIN[l.skin] || "#d4a574",
      tunicColor: TOP[l.top] || "#4a6a32",
    };
  }

  const api = {
    PACK,
    MANIFEST,
    BOUNDS,
    load,
    ready,
    boundsOf,
    destAtTopDiamond,
    destAtGroundDiamond,
    destAtFeet,
    blit,
    blitFeet,
    blitTint,
    tileKey,
    nodeKey,
    npcKey,
    paperdollKeys,
    HAIR,
    SKIN,
    TOP,
  };
  root.HollowmereSprites = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
})(typeof globalThis !== "undefined" ? globalThis : this);
