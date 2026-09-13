#!/usr/bin/env node
// ¾ / 2:1 projection + chase camera. Gameplay x/y stay integer tiles.
"use strict";

const path = require("path");
const iso = require(path.join(__dirname, "..", "client", "iso.js"));
const art = require(path.join(__dirname, "..", "client", "art.js"));
const spr = require(path.join(__dirname, "..", "client", "sprites.js"));

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

assert(iso.TW === 96 && iso.TH === 48, "2:1 diamond is 96×48");

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
assert(feet.x === stileCenter.x && feet.y === stileCenter.y, "tileFeet is the diamond centre (no extra offset)");
const stileHit = art.hitTile(feet.x, feet.y - 10, cam, map, [], []);
assert(stileHit && stileHit.x === 8 && stileHit.y === 8, "stile AABB frames spawn (8,8)");

const houseHit = art.hitTile(
  iso.tileFeet(4, 5, cam).x,
  iso.tileFeet(4, 5, cam).y - 20,
  cam, map, [], []
);
assert(houseHit && houseHit.x === 4 && houseHit.y === 5, "cottage footprint at (4,5)");

// --- grid alignment -------------------------------------------------------
assert(iso.HOUSE.x === 3 && iso.HOUSE.y === 4 && iso.HOUSE.w === 2 && iso.HOUSE.h === 2,
  "cottage is a 2×2 snapped to the first H (3,4)");
assert(iso.HOUSE.idX === 4 && iso.HOUSE.idY === 5, "cottage click reports the hearth tile");

const hearthNode = [{ id: "fire-1", kind: "fire", x: 4, y: 5 }];
const hearthFeet = iso.tileFeet(4, 5, cam);
const hearthClick = art.hitTile(hearthFeet.x, hearthFeet.y - 20, cam, map, hearthNode, []);
assert(hearthClick && hearthClick.x === 4 && hearthClick.y === 5, "hearth node click stays on (4,5)");

const originCam = { x: 0, y: 0 };
const c00 = iso.tileCenter(0, 0, originCam);
const f00 = iso.tileFeet(0, 0, originCam);
assert(c00.x === f00.x && c00.y === f00.y, "tileFeet === tileCenter");
near(c00.x, iso.project(0.5, 0.5).x, 1e-9, "foot x is iso project of tile centre");
near(c00.y, iso.project(0.5, 0.5).y, 1e-9, "foot y is iso project of tile centre");

const dia = iso.diamond(0, 0, originCam);
near(dia[0].x, 0, 1e-9, "diamond north x");
near(dia[2].y - dia[0].y, iso.TH, 1e-9, "diamond height is TH");
near(dia[1].x - dia[3].x, iso.TW, 1e-9, "diamond width is TW");
near(c00.x, (dia[1].x + dia[3].x) / 2, 1e-9, "tile centre sits on the diamond mid-x");
near(c00.y, (dia[0].y + dia[2].y) / 2, 1e-9, "tile centre sits on the diamond mid-y");

assert(Number.isInteger(cam.x) && Number.isInteger(cam.y), "chase camera snaps to integer pixels");

const houseDia = iso.footprintDiamond(iso.HOUSE.x, iso.HOUSE.y, iso.HOUSE.w, iso.HOUSE.h, originCam);
near(houseDia[1].x - houseDia[3].x, iso.TW * 2, 1e-9, "cottage 2×2 diamond is 2·TW wide");
near(houseDia[2].y - houseDia[0].y, iso.TH * 2, 1e-9, "cottage 2×2 diamond is 2·TH tall");
const houseMid = iso.footprintCenter(iso.HOUSE.x, iso.HOUSE.y, iso.HOUSE.w, iso.HOUSE.h, originCam);
near(houseMid.x, iso.toScreen(4, 5, originCam).x, 1e-9, "cottage centre is the shared (4,5) corner");

// Terrain cube: padded 64×64 grass plants its top face on the diamond.
const grass = spr.BOUNDS["terrain/grass"];
const cube = spr.destAtTopDiamond(dia[3].x, dia[0].y, iso.TW, 64, 64, grass);
const scaleG = iso.TW / grass.w;
near(cube.x + grass.x * scaleG, dia[3].x, 1e-9, "grass cube left meets diamond west");
near(cube.y + grass.y * scaleG, dia[0].y, 1e-9, "grass cube top meets diamond north");
near(grass.w * scaleG, iso.TW, 1e-9, "grass top face is one tile wide");

// Prop plate: stile ground diamond (opaque width × ½, on the opaque bottom)
// lands on the spawn cell diamond.
const stileB = spr.BOUNDS["props/stile"];
const plate = spr.destAtGroundDiamond(dia[3].x, dia[0].y, iso.TW, iso.TH, 96, 96, stileB);
const scaleS = iso.TW / stileB.w;
const plateSouth = plate.y + (stileB.y + stileB.h) * scaleS;
const plateLeft = plate.x + stileB.x * scaleS;
near(plateLeft, dia[3].x, 1e-9, "stile plate left meets diamond west");
near(plateSouth, dia[2].y, 1e-9, "stile plate south meets diamond south");

// Figure foot: opaque bottom-centre maps to tile centre.
const marta = spr.BOUNDS["npcs/marta"];
const doll = spr.destAtFeet(c00.x, c00.y, 64, 96, 64, 96, marta);
near(doll.x + (marta.x + marta.w / 2), c00.x, 1e-9, "Marta opaque foot x is tile centre");
near(doll.y + (marta.y + marta.h), c00.y, 1e-9, "Marta opaque foot y is tile centre");

const houseB = spr.BOUNDS["buildings/house"];
const cottage = spr.destAtGroundDiamond(
  houseDia[3].x, houseDia[0].y, iso.TW * 2, iso.TH * 2, 192, 192, houseB
);
const scaleH = (iso.TW * 2) / houseB.w;
near(cottage.x + houseB.x * scaleH, houseDia[3].x, 1e-9, "cottage left meets 2×2 west");
near(cottage.y + (houseB.y + houseB.h) * scaleH, houseDia[2].y, 1e-9, "cottage south meets 2×2 south");

const miss = art.hitTile(-400, -400, cam, map, [], []);
assert(miss === null, "off-map click is null");

if (failed) {
  console.error(failed + " failed");
  process.exit(1);
}
console.log("ISO PASS");
