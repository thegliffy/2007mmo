#!/usr/bin/env node
// Sprite-swap map only. No camera, no compositor.
"use strict";

const path = require("path");
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

assert(spr.tileKey("P", false) === "tiles/path", "path tile key");
assert(spr.tileKey("P", true) === "tiles/path-scar", "scar path tile key");
assert(spr.tileKey("~", false) === "tiles/water", "water tile key");
assert(spr.nodeKey({ kind: "bush", ready: true }) === "props/bush", "ready bush");
assert(spr.nodeKey({ kind: "bush", ready: false }) === "props/bush-spent", "spent bush");
assert(spr.nodeKey({ kind: "fire", id: "fire-1" }) === "props/hearth", "hearth is fire-1");
assert(spr.nodeKey({ kind: "fire", id: "fire-9" }) === "props/fire", "campfire");
assert(spr.nodeKey({ kind: "tree", ready: true }) === "", "ready trees stay on the map tile");
assert(spr.nodeKey({ kind: "tree", ready: false }) === "props/stump", "spent tree is a stump");
assert(spr.npcKey({ name: "Thornkin", hostile: true }) === "hostiles/thornkin", "thornkin");
assert(spr.npcKey({ name: "Brambleback", hostile: true }) === "hostiles/brambleback", "brambleback");
assert(spr.npcKey({ name: "Marta" }) === "", "villagers stay on the paperdoll");
assert(spr.MANIFEST["props/stile"], "stile is in the drop list");
assert(spr.MANIFEST["props/chest"], "chest is in the drop list");

if (failed) {
  console.error(failed + " failed");
  process.exit(1);
}
console.log("SPRITES PASS");
