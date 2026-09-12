#!/usr/bin/env node
// Client-level tests for canvas click targeting (Chebyshev ≤ 1).
"use strict";

const path = require("path");
const pick = require(path.join(__dirname, "..", "client", "pick-npc.js"));

let failed = 0;
function assert(cond, msg) {
  if (!cond) {
    failed++;
    console.error("FAIL", msg);
  } else {
    console.log("ok  ", msg);
  }
}

const thorn = { id: "npc-thornkin-1", name: "Thornkin", x: 10, y: 22, hostile: true, hp: 6, maxHp: 6 };
const thornAdj = { id: "npc-thornkin-2", name: "Thornkin", x: 11, y: 22, hostile: true, hp: 6, maxHp: 6 };
const marta = { id: "npc-marta", name: "Marta", x: 10, y: 22, hostile: false };
const dead = { id: "npc-dead", name: "Thornkin", x: 10, y: 22, hostile: true, hp: 0, maxHp: 6 };
const bush = { id: "bush-1", kind: "bush", x: 10, y: 22 };

assert(pick.chebyshev(10, 22, 11, 23) === 1, "diagonal neighbor is Chebyshev 1");
assert(pick.chebyshev(10, 22, 12, 22) === 2, "two tiles east is Chebyshev 2");

assert(pick.pickNearestLivingNPC([thorn], { x: 10, y: 22 }, 1) === thorn, "exact tile still hits");
assert(pick.pickNearestLivingNPC([thorn], { x: 11, y: 22 }, 1) === thorn, "click one tile east still hits");
assert(pick.pickNearestLivingNPC([thorn], { x: 9, y: 21 }, 1) === thorn, "click diagonal still hits");
assert(pick.pickNearestLivingNPC([thorn], { x: 12, y: 22 }, 1) === null, "two tiles away is a miss");
assert(pick.pickNearestLivingNPC([dead], { x: 10, y: 22 }, 1) === null, "dead hostile is ignored");
assert(pick.pickNearestLivingNPC([marta], { x: 11, y: 22 }, 1) === marta, "living villager still selectable");

const mixed = pick.pickNearestLivingNPC([marta, thornAdj], { x: 10, y: 22 }, 1);
assert(mixed === thornAdj, "prefer in-range hostile over a closer villager");

const nearerHostile = pick.pickNearestLivingNPC([thorn, thornAdj], { x: 10, y: 22 }, 1);
assert(nearerHostile === thorn, "among hostiles, prefer the closer one");

const intentNear = pick.resolveCanvasClick([thorn], [bush], { x: 11, y: 22 });
assert(intentNear && intentNear.t === "attack" && intentNear.id === thorn.id, "near click sends attack");

const intentFriend = pick.resolveCanvasClick([marta], [], { x: 10, y: 23 });
assert(intentFriend && intentFriend.t === "interact" && intentFriend.id === marta.id, "near villager sends interact");

const intentNode = pick.resolveCanvasClick([], [bush], { x: 10, y: 22 });
assert(intentNode && intentNode.t === "interact" && intentNode.id === bush.id, "no NPC keeps node interact");

const intentMove = pick.resolveCanvasClick([thorn], [bush], { x: 16, y: 26 });
assert(intentMove && intentMove.t === "move" && intentMove.x === 16 && intentMove.y === 26, "far click stays move");

if (failed) {
  console.error(failed + " failed");
  process.exit(1);
}
console.log("PICK NPC PASS");
