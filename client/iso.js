// ¾ / 2:1 hamlet projection + chase camera.
// Gameplay stays on integer tile (x, y). This file only maps those
// onto the canvas and back. Shared with scripts/iso-test.js (node).
//
// ANCHOR CONVENTION
// -----------------
// Every cell is a 2:1 diamond. For integer tile (tx, ty):
//
//   north  = project(tx,     ty)
//   east   = project(tx + 1, ty)
//   south  = project(tx + 1, ty + 1)   // diamond bottom of the cell
//   west   = project(tx,     ty + 1)
//   centre = project(tx + 0.5, ty + 0.5)
//
// tileFeet === tileCenter === centre. That is the world foot point
// for a 1×1 object: the iso project of the tile centre.
//
//   • Figures (player, NPCs, hostiles): the sprite's opaque
//     bottom-centre (the authored feet) maps onto that foot point.
//   • ¾ prop / gather / building plates: the plate's ground diamond
//     (opaque width × half that, sitting on the opaque bottom) is
//     scaled onto this cell's diamond, so the object sits on the
//     same grid as the terrain.
//   • Terrain cubes: the cube's top face (opaque top × opaque width)
//     is the walking diamond — it is scaled onto this cell's diamond.
//   • The cottage is a 2×2 plate. NW tile is the first H (3, 4);
//     the hearth at (4, 5) is the SE cell. Same diamond rule on the
//     2×2 cluster (width 2·TW, height 2·TH).
//
// Camera offsets are snapped to integer pixels so tiles and plates
// cannot drift a half-pixel apart while the chase eases.
(function (root) {
  const TW = 96;
  const TH = 48;

  // Cottage 2×2, snapped to the hamlet H-block. idX/idY is the
  // hearth tile — what a click on the roof reports.
  const HOUSE = { x: 3, y: 4, w: 2, h: 2, idX: 4, idY: 5 };

  function project(x, y) {
    return {
      x: (x - y) * (TW / 2),
      y: (x + y) * (TH / 2),
    };
  }

  function unproject(sx, sy) {
    const u = sx / (TW / 2);
    const v = sy / (TH / 2);
    return { x: (u + v) / 2, y: (v - u) / 2 };
  }

  function cameraFollow(fx, fy, viewW, viewH) {
    const p = project(fx, fy);
    return {
      x: Math.round(viewW / 2 - p.x),
      y: Math.round(viewH * 0.58 - p.y),
    };
  }

  // Smooth chase: ease the focus toward the player. k is 0..1 per frame.
  function chase(prev, target, k) {
    const t = k == null ? 0.14 : k;
    if (!prev || !Number.isFinite(prev.x)) return { x: target.x, y: target.y };
    return {
      x: prev.x + (target.x - prev.x) * t,
      y: prev.y + (target.y - prev.y) * t,
    };
  }

  function toScreen(x, y, cam) {
    const p = project(x, y);
    return { x: p.x + (cam && cam.x || 0), y: p.y + (cam && cam.y || 0) };
  }

  function fromScreen(sx, sy, cam) {
    return unproject(sx - (cam && cam.x || 0), sy - (cam && cam.y || 0));
  }

  function screenToTile(sx, sy, cam) {
    const w = fromScreen(sx, sy, cam);
    return { x: Math.floor(w.x), y: Math.floor(w.y) };
  }

  function diamond(tx, ty, cam) {
    return [
      toScreen(tx, ty, cam),
      toScreen(tx + 1, ty, cam),
      toScreen(tx + 1, ty + 1, cam),
      toScreen(tx, ty + 1, cam),
    ];
  }

  function tileCenter(tx, ty, cam) {
    return toScreen(tx + 0.5, ty + 0.5, cam);
  }

  // World foot of a 1×1 object: the centre of that tile's diamond.
  function tileFeet(tx, ty, cam) {
    return tileCenter(tx, ty, cam);
  }

  function footprintDiamond(ox, oy, w, h, cam) {
    return [
      toScreen(ox, oy, cam),
      toScreen(ox + w, oy, cam),
      toScreen(ox + w, oy + h, cam),
      toScreen(ox, oy + h, cam),
    ];
  }

  function footprintCenter(ox, oy, w, h, cam) {
    return toScreen(ox + w / 2, oy + h / 2, cam);
  }

  function inView(sx, sy, viewW, viewH, pad) {
    const p = pad == null ? 120 : pad;
    return sx > -p && sy > -p && sx < viewW + p && sy < viewH + p;
  }

  const api = {
    TW,
    TH,
    HOUSE,
    project,
    unproject,
    cameraFollow,
    chase,
    toScreen,
    fromScreen,
    screenToTile,
    diamond,
    tileCenter,
    tileFeet,
    footprintDiamond,
    footprintCenter,
    inView,
  };
  root.HollowmereIso = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
})(typeof globalThis !== "undefined" ? globalThis : this);
