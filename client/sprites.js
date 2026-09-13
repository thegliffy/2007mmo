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

  function loadOne(key, src, fallback) {
    const img = new Image();
    img.onload = () => { images[key] = img; };
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

  function blitFeet(ctx, key, feetX, feetY, w, h) {
    return blit(ctx, key, feetX - w / 2, feetY - h, w, h);
  }

  const scratch = typeof document !== "undefined" ? document.createElement("canvas") : null;

  function blitTint(ctx, key, feetX, feetY, w, h, color) {
    const im = ready(key);
    if (!im || !scratch) return false;
    scratch.width = Math.max(1, Math.ceil(w));
    scratch.height = Math.max(1, Math.ceil(h));
    const s = scratch.getContext("2d");
    s.clearRect(0, 0, scratch.width, scratch.height);
    s.drawImage(im, 0, 0, scratch.width, scratch.height);
    if (color) {
      s.globalCompositeOperation = "source-in";
      s.fillStyle = color;
      s.fillRect(0, 0, scratch.width, scratch.height);
    }
    ctx.drawImage(scratch, feetX - w / 2, feetY - h);
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
    load,
    ready,
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
