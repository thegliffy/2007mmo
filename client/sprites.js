// Thin AD sprite swap for the existing top-down hamlet.
// No camera math. If a PNG is present it is drawn in the tile;
// if not, the caller keeps the canvas fallback.
(function (root) {
  const MANIFEST = {
    "tiles/grass": "assets/tiles/grass.png",
    "tiles/grass-scar": "assets/tiles/grass-scar.png",
    "tiles/path": "assets/tiles/path.png",
    "tiles/path-scar": "assets/tiles/path-scar.png",
    "tiles/water": "assets/tiles/water.png",
    "tiles/wall": "assets/tiles/wall.png",
    "tiles/house": "assets/tiles/house.png",
    "tiles/earth": "assets/tiles/earth.png",
    "props/stile": "assets/props/stile.png",
    "props/hearth": "assets/props/hearth.png",
    "props/fire": "assets/props/fire.png",
    "props/mill": "assets/props/mill.png",
    "props/chest": "assets/props/chest.png",
    "props/kiln": "assets/props/kiln.png",
    "props/anvil": "assets/props/anvil.png",
    "props/bush": "assets/props/bush.png",
    "props/bush-spent": "assets/props/bush-spent.png",
    "props/hazel": "assets/props/hazel.png",
    "props/hazel-spent": "assets/props/hazel-spent.png",
    "props/tree": "assets/props/tree.png",
    "props/stump": "assets/props/stump.png",
    "props/copper": "assets/props/copper.png",
    "props/tin": "assets/props/tin.png",
    "hostiles/thornkin": "assets/hostiles/thornkin.png",
    "hostiles/brambleback": "assets/hostiles/brambleback.png",
  };

  const images = {};

  function load() {
    Object.keys(MANIFEST).forEach((key) => {
      if (images[key]) return;
      const img = new Image();
      img.onload = () => { images[key] = img; };
      img.onerror = () => { images[key] = null; };
      img.src = MANIFEST[key];
    });
  }

  function ready(key) {
    const im = images[key];
    return im && im.naturalWidth ? im : null;
  }

  function tile(ctx, key, px, py, tw, th) {
    const im = ready(key);
    if (!im) return false;
    ctx.drawImage(im, px, py, tw, th);
    return true;
  }

  function prop(ctx, key, px, py, tw, th) {
    const im = ready(key);
    if (!im) return false;
    ctx.drawImage(im, px, py + th - th * 1.2, tw, th * 1.2);
    return true;
  }

  function figure(ctx, key, cx, cy, tw, th) {
    const im = ready(key);
    if (!im) return false;
    const w = tw * 0.9;
    const h = th * 1.15;
    ctx.drawImage(im, cx - w / 2, cy - h * 0.72, w, h);
    return true;
  }

  function tileKey(glyph, scars) {
    switch (glyph) {
      case "~": return "tiles/water";
      case "P": return scars ? "tiles/path-scar" : "tiles/path";
      case "H":
      case "*": return "tiles/house";
      case "#": return "tiles/wall";
      case "C":
      case "N":
      case "K":
      case "A": return "tiles/earth";
      case "T": return scars ? "tiles/grass-scar" : "tiles/grass";
      default: return scars ? "tiles/grass-scar" : "tiles/grass";
    }
  }

  function nodeKey(n) {
    if (!n) return "";
    if (n.kind === "bush") return n.ready ? "props/bush" : "props/bush-spent";
    if (n.kind === "hazel") return n.ready ? "props/hazel" : "props/hazel-spent";
    if (n.kind === "tree") return n.ready ? "" : "props/stump";
    if (n.kind === "mill") return "props/mill";
    if (n.kind === "chest") return "props/chest";
    if (n.kind === "kiln") return "props/kiln";
    if (n.kind === "anvil") return "props/anvil";
    if (n.kind === "copper") return "props/copper";
    if (n.kind === "tin") return "props/tin";
    if (n.kind === "fire") return n.id === "fire-1" ? "props/hearth" : "props/fire";
    return "";
  }

  function npcKey(e) {
    if (!e) return "";
    if (/brambleback/i.test(e.name || "")) return "hostiles/brambleback";
    if (e.hostile) return "hostiles/thornkin";
    return "";
  }

  const api = { load, tile, prop, figure, tileKey, nodeKey, npcKey, MANIFEST };
  root.HollowmereSprites = api;
  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
})(typeof globalThis !== "undefined" ? globalThis : this);
