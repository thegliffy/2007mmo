#!/usr/bin/env python3
"""Paint the paperdoll garment masks (jerkin, sword) on the 128x192 doll
canvas, matching the real layer geometry measured from the pack:

  skin/body  alpha bbox (28,20)-(100,180): head 20..52, torso 56..104
             (arms are the side columns x=28..52 / x=76..100), legs 108..180
  tunic      alpha bbox (40,56)-(88,128)  — torso + hip skirt
  hair       alpha bbox (40,8)-(88,52)
  foot anchor (64,192)

Output is a white-alpha mask the client tints with the item's WornColor
via source-in."""
from PIL import Image, ImageDraw

W, H = 128, 192

# --- Jerkin: the tunic silhouette, but sleeveless — the arm columns stay
# bare skin. Tunic covers x40..88; arms are x28..52 and x76..100, so the
# overlap is the outer 12px of each side. Punch those columns out below
# the shoulder line (y=56) down to the hem.
tunic = Image.open("client/assets/paperdoll/tunic_mask.png").convert("RGBA")
ta = tunic.split()[3]
jerkin = ta.copy()
jd = ImageDraw.Draw(jerkin)
# sleeveless: clear the outer arm columns, keep the inner torso edge
jd.rectangle([28, 56, 51, 128], fill=0)   # viewer-left arm column
jd.rectangle([77, 56, 100, 128], fill=0)  # viewer-right arm column
# a raised hem so the skirt reads as leather, not cloth: trim 6px off
jd.rectangle([0, 122, W, H], fill=0)
# shoulder capes stay (y56..64 full width) — that is the chunky plate look.
jerkin_rgba = Image.new("RGBA", (W, H), (255, 255, 255, 0))
jerkin_rgba.putalpha(jerkin)
jerkin_rgba.save("client/assets/paperdoll/jerkin_mask.png")

# --- Sword: held in the viewer-right hand (grip near x=88, y=104 — the
# right arm column ends at y=104). Blade points down-outward, chunky
# 2007 style, entirely OUTSIDE the body silhouette (x>=92) so it never
# occludes the arm.
img = Image.new("RGBA", (W, H), (0, 0, 0, 0))
d = ImageDraw.Draw(img)
# blade: tapered quad from the guard down past the hip
d.polygon([(94, 96), (102, 94), (112, 148), (106, 152)], fill=(255, 255, 255, 255))
# tip
d.polygon([(106, 152), (112, 148), (114, 160)], fill=(255, 255, 255, 255))
# crossguard across the grip
d.polygon([(90, 92), (106, 88), (108, 96), (92, 100)], fill=(255, 255, 255, 255))
# grip up to the pommel (hand at ~x92,y88)
d.polygon([(94, 80), (100, 78), (102, 90), (96, 92)], fill=(255, 255, 255, 255))
d.ellipse([92, 72, 102, 82], fill=(255, 255, 255, 255))
img.save("client/assets/paperdoll/sword_mask.png")

# --- Preview on a dark ground so every layer has contrast.
skin = Image.open("client/assets/paperdoll/skin_tan.png").convert("RGBA")
tun = Image.open("client/assets/paperdoll/tunic_moss.png").convert("RGBA")
hair = Image.open("client/assets/paperdoll/hair_short_mask.png").convert("RGBA")
# tint hair umber for the preview
hair_umber = hair.copy()
hcol = Image.new("RGBA", hair.size, (61, 36, 18, 255))
hcol.putalpha(hair.split()[3])

prev = Image.new("RGBA", (W, H), (24, 20, 14, 255))
prev.alpha_composite(skin)
prev.alpha_composite(tun)
prev.alpha_composite(hair_umber)
prev.alpha_composite(jerkin_rgba)   # brown tint baked for preview only
jerkin_brown = Image.new("RGBA", (W, H), (138, 90, 42, 255))
jerkin_brown.putalpha(jerkin)
# redo: skin/tunic/hair, then tinted jerkin, then tinted sword
prev = Image.new("RGBA", (W, H), (24, 20, 14, 255))
prev.alpha_composite(skin)
prev.alpha_composite(tun)
prev.alpha_composite(hair_umber)
prev.alpha_composite(jerkin_brown)
sword_steel = Image.new("RGBA", (W, H), (154, 164, 174, 255))
sword_steel.putalpha(img.split()[3])
prev.alpha_composite(sword_steel)
prev.save("client/assets/paperdoll/preview_worn_gear.png")
print("wrote jerkin_mask.png, sword_mask.png, preview_worn_gear.png")
