// ¾ / 2:1 hamlet projection + chase camera.
// Gameplay stays on integer tile (x, y). This file only maps those
// onto the canvas and back. Shared with scripts/iso-test.js (node).
(function (root) {
  const TW = 72;
  const TH = 36;

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
      x: viewW / 2 - p.x,
      y: viewH * 0.58 - p.y,
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

  function tileFeet(tx, ty, cam) {
    const c = tileCenter(tx, ty, cam);
    return { x: c.x, y: c.y + TH * 0.2 };
  }

  function inView(sx, sy, viewW, viewH, pad) {
    const p = pad == null ? 120 : pad;
    return sx > -p && sy > -p && sx < viewW + p && sy < viewH + p;
  }

  const api = {
    TW,
    TH,
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
    inView,
  };
  root.HollowmereIso = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
})(typeof globalThis !== "undefined" ? globalThis : this);
