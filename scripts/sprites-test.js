#!/usr/bin/env node
// AD pack keys for the ¾ painter. No camera here.
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

assert(spr.tileKey("P", false) === "terrain/path", "path tile key");
assert(spr.tileKey("P", true) === "terrain/scar", "scar path tile key");
assert(spr.tileKey("~", false) === "terrain/water", "water tile key");
assert(spr.tileKey("#", false) === "terrain/wall", "wall tile key");
assert(spr.tileKey(".", false) === "terrain/grass", "grass tile key");
assert(spr.tileKey(".", true) === "terrain/scar", "scar grass tile key");
assert(spr.tileKey("C", false) === "terrain/scar", "copper sits on scar ground");

assert(spr.nodeKey({ kind: "bush", ready: true }) === "gather/bramble", "ready bush");
assert(spr.nodeKey({ kind: "bush", ready: false }) === "gather/bramble", "spent bush keeps the AD plate");
assert(spr.nodeKey({ kind: "hazel", ready: true }) === "gather/hazel", "hazel");
assert(spr.nodeKey({ kind: "mill" }) === "props/millstone", "millstone");
assert(spr.nodeKey({ kind: "chest" }) === "props/oak_chest", "oak chest");
assert(spr.nodeKey({ kind: "kiln" }) === "props/kiln", "kiln");
assert(spr.nodeKey({ kind: "anvil" }) === "props/anvil", "anvil");
assert(spr.nodeKey({ kind: "copper" }) === "gather/ore_copper", "copper");
assert(spr.nodeKey({ kind: "tin" }) === "gather/ore_tin", "tin");
assert(spr.nodeKey({ kind: "fire", id: "fire-1" }) === "props/hearth", "hearth is fire-1");
assert(spr.nodeKey({ kind: "fire", id: "fire-9" }) === "", "campfire stays a canvas pile");
assert(spr.nodeKey({ kind: "tree", ready: true }) === "", "ready trees stay on the map tile");
assert(spr.nodeKey({ kind: "tree", ready: false }) === "props/stump", "spent tree is a stump");

assert(spr.npcKey({ name: "Thornkin", hostile: true }) === "hostiles/thornkin", "thornkin");
assert(spr.npcKey({ name: "Brambleback", hostile: true }) === "hostiles/brambleback", "brambleback");
assert(spr.npcKey({ name: "Marta" }) === "npcs/marta", "Marta sprite");
assert(spr.npcKey({ name: "Old Fen" }) === "npcs/old_fen", "Old Fen sprite");
assert(spr.npcKey({ name: "Pip" }) === "npcs/pip", "Pip sprite");
assert(spr.npcKey({ name: "Wend the Pedlar" }) === "npcs/wend", "Wend sprite");

const doll = spr.paperdollKeys({ skin: "tan", hair: "short", hairColor: "umber", top: "moss" });
assert(doll.skin === "paperdoll/skin_tan", "paperdoll skin");
assert(doll.tunic === "paperdoll/tunic_moss", "paperdoll tunic");
assert(doll.hair === "paperdoll/hair_short_mask", "paperdoll hair mask");
assert(doll.bodyMask === "paperdoll/body_mask", "body mask");
assert(doll.hairColor === "#3d2412", "umber hair tint");

assert(spr.MANIFEST["props/stile"], "stile is in the drop list");
assert(spr.MANIFEST["props/oak_chest"], "oak chest is in the drop list");
assert(spr.MANIFEST["buildings/house"], "house is in the drop list");
assert(spr.MANIFEST["gather/bramble"], "bramble is in the drop list");
assert(spr.MANIFEST["npcs/marta"], "Marta is in the drop list");
assert(spr.MANIFEST["paperdoll/skin_tan"], "paperdoll skin is in the drop list");
assert(spr.MANIFEST["terrain/grass"], "grass is in the drop list");

if (failed) {
  console.error(failed + " failed");
  process.exit(1);
}
console.log("SPRITES PASS");
