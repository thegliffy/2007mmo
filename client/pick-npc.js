// Canvas click targeting for Hollowmere. Shared by the browser client
// and scripts/pick-npc-test.js (node).
//
// On a 28×32 map a tile is small; hostiles also wander. Clicking the
// beast's sprite often lands on a neighbor tile. Pick the nearest living
// NPC within Chebyshev ≤ maxDist (default 1 = the 3×3 around the click).
// Prefer hostiles when a villager and a beast are both in range.
(function (root) {
  function chebyshev(ax, ay, bx, by) {
    return Math.max(Math.abs(ax - bx), Math.abs(ay - by));
  }

  function npcIsLiving(n) {
    if (!n) return false;
    if (n.maxHp > 0) return (n.hp == null ? n.maxHp : n.hp) > 0;
    return true;
  }

  function pickNearestLivingNPC(npcs, tile, maxDist) {
    if (!tile || !npcs || !npcs.length) return null;
    if (maxDist == null) maxDist = 1;
    let best = null;
    let bestDist = Infinity;
    let bestHostile = false;
    for (const n of npcs) {
      if (!npcIsLiving(n)) continue;
      const d = chebyshev(n.x, n.y, tile.x, tile.y);
      if (d > maxDist) continue;
      const hostile = !!n.hostile;
      if (!best) {
        best = n;
        bestDist = d;
        bestHostile = hostile;
        continue;
      }
      if (hostile && !bestHostile) {
        best = n;
        bestDist = d;
        bestHostile = true;
        continue;
      }
      if (hostile === bestHostile && d < bestDist) {
        best = n;
        bestDist = d;
      }
    }
    return best;
  }

  function resolveCanvasClick(npcs, nodes, tile) {
    if (!tile) return null;
    const npc = pickNearestLivingNPC(npcs, tile, 1);
    if (npc) {
      return { t: npc.hostile ? "attack" : "interact", id: npc.id };
    }
    const node = (nodes || []).find((n) => n.x === tile.x && n.y === tile.y);
    if (node) return { t: "interact", id: node.id };
    return { t: "move", x: tile.x, y: tile.y };
  }

  const api = { chebyshev, npcIsLiving, pickNearestLivingNPC, resolveCanvasClick };
  root.HollowmerePick = api;
  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
})(typeof globalThis !== "undefined" ? globalThis : this);
