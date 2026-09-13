#!/usr/bin/env node
// ¾ / 2:1 projection + chase camera. Gameplay x/y stay integer tiles.
"use strict";

const path = require("path");
const iso = require(path.join(__dirname, "..", "client", "iso.js"));
const art = require(path.join(__dirname, "..", "client", "art.js"));

let failed = 0;
function assert(cond, msg) {
  if (!cond) {
    failed++;
    console.error("FAIL", msg);
  } else {
    console.log("ok  ", msg);
  }
}

function near(a, b, eps, msg) {
  assert(Math.abs(a - b) < (eps == null ? 1e-9 : eps), msg + " (" + a + " vs " + b + ")");
}

assert(iso.TW === 72 && iso.TH === 36, "2:1 diamond is 72×36");

const origin = iso.project(0, 0);
assert(origin.x === 0 && origin.y === 0, "origin projects to 0,0");

const p88 = iso.project(8, 8);
assert(p88.x === 0, "stile tile (8,8) sits on the iso vertical");
assert(p88.y === (8 + 8) * (iso.TH / 2), "stile depth is (x+y)*TH/2");

const back = iso.unproject(p88.x, p88.y);
near(back.x, 8, 1e-9, "unproject x of stile");
near(back.y, 8, 1e-9, "unproject y of stile");

const far = iso.project(39, 31);
const farBack = iso.unproject(far.x, far.y);
near(farBack.x, 39, 1e-9, "SE corner unproject x");
near(farBack.y, 31, 1e-9, "SE corner unproject y");

const snapped = iso.chase(null, { x: 10, y: 12 }, 0.14);
assert(snapped.x === 10 && snapped.y === 12, "chase with no previous focus snaps");
const stepped = iso.chase({ x: 0, y: 0 }, { x: 10, y: 10 }, 0.14);
near(stepped.x, 1.4, 1e-9, "chase eases x by k");
near(stepped.y, 1.4, 1e-9, "chase eases y by k");
const hard = iso.chase({ x: 0, y: 0 }, { x: 8.5, y: 8.5 }, 1);
near(hard.x, 8.5, 1e-9, "k=1 snaps onto the player");

const viewW = 1200;
const viewH = 960;
const cam = iso.cameraFollow(8.5, 8.5, viewW, viewH);
const stileCenter = iso.tileCenter(8, 8, cam);
const stileTile = iso.screenToTile(stileCenter.x, stileCenter.y, cam);
assert(stileTile.x === 8 && stileTile.y === 8, "screenToTile at stile centre is (8,8)");

// Chase camera keeps the player near the middle of the canvas.
// A top-down stretch would put tile 8 at 8/40 of the width (~240px).
near(stileCenter.x, viewW / 2, 8, "stile centre is horizontally chased");
assert(stileCenter.y > viewH * 0.4 && stileCenter.y < viewH * 0.75, "stile centre sits in the chased band");
assert(Math.abs(stileCenter.x - (8 / 40) * viewW) > 200, "not a top-down stretch of the 40-wide map");

const southEdge = iso.tileCenter(8, 31, cam);
assert(!iso.inView(southEdge.x, southEdge.y, viewW, viewH, 90), "south map edge is off-camera from the stile");
const scars = iso.tileCenter(35, 8, cam);
assert(!iso.inView(scars.x, scars.y, viewW, viewH, 90), "eastern scars are off-camera from the stile");

const map = { w: 40, h: 32 };
const ground = art.hitTile(stileCenter.x, stileCenter.y, cam, map, [], []);
assert(ground && ground.x === 8 && ground.y === 8, "hitTile on empty stile tile is the ground");

const feet = iso.tileFeet(8, 8, cam);
const stileHit = art.hitTile(feet.x, feet.y - 10, cam, map, [], []);
assert(stileHit && stileHit.x === 8 && stileHit.y === 8, "stile AABB frames spawn (8,8)");

const houseHit = art.hitTile(
  iso.tileFeet(4, 5, cam).x,
  iso.tileFeet(4, 5, cam).y - 20,
  cam, map, [], []
);
assert(houseHit && houseHit.x === 4 && houseHit.y === 5, "cottage footprint at (4,5)");

const miss = art.hitTile(-400, -400, cam, map, [], []);
assert(miss === null, "off-map click is null");

if (failed) {
  console.error(failed + " failed");
  process.exit(1);
}
console.log("ISO PASS");
