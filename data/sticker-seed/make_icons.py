"""Generate the third-party verifier badge sticker set from the genuine
Telegram verification-badge Lottie sources.

Major uses the real Telegram "Badge" sticker (already #007AFF).
Hold uses the real Telegram "Hold adaptive" sticker, recolored from white to
#007AFF so both marks render blue.

Both documents carry `text_color: true`, which tells the official client to
paint the badge with the badge's `premiumFg` colour instead of the baked
artwork colour (only the artwork alpha is used as a mask):

  * with a profile background (gradient/solid) the client switches the badge
    to `infoColoredBotVerifyBadge`, whose `premiumFg` is
    `groupCallVideoSubTextFg` (#ffffffc0, alpha 192) -> a translucent white
    badge that stays readable over the background;
  * without a background the badge keeps `infoPeerBadge`, whose `premiumFg`
    is `profileVerifiedCheckBg` (windowBgActive, opaque blue) -> a solid blue
    badge.

The client colorizes the rendered frame with that `premiumFg` and keeps the
artwork's alpha (Lottie::Icon::paint -> style::colorizeImage). The artwork is
kept fully opaque (fill opacity 100): over a profile background the client's
`premiumFg` is itself translucent white (#ffffffc0, alpha 192), so the mark
stays readable and the background shows through; without a background the
opaque blue `premiumFg` renders a solid mark. Baking a lower opacity would
make the mark translucent in both cases, which is not wanted. The final hue
still comes from the client's `premiumFg`, which the server cannot override.

Because the tint is applied at paint time, the baked fill colour is
irrelevant to the rendered result; it is kept for previews and for clients
that ignore `text_color`.

The raw `.tgs` sources live next to this script in `verifiericons-src/` so the
set is reproducible without a Telegram Desktop cache.

Document ids are bumped whenever the artwork (or its attributes) changes so
clients that cached a previous rendition under the old document id download
the new one instead of serving stale bytes/metadata from their local cache.
The set `hash` is bumped for the same reason: the server seed importer skips
a set whose hash is unchanged.
"""

import gzip
import json
import os
import zlib

BASE = os.path.dirname(os.path.abspath(__file__))
SRC = os.path.join(BASE, "verifiericons-src")
SET_DIR = os.path.join(BASE, "telegram_stickers_export", "verifiericons")
STICKERS = os.path.join(SET_DIR, "stickers")
os.makedirs(STICKERS, exist_ok=True)

BLUE = [0, 0.478431402468, 1, 1]  # #007AFF, the exact value Telegram ships for "Badge"

# Fill opacity baked into the artwork (0..100). The client colorizes the
# rendered frame with `premiumFg` but keeps the artwork alpha, so this is the
# artwork's own transparency. 100 keeps the mark solid; the "semi-transparent
# over a background" look comes from the client's translucent white premiumFg
# (#ffffffc0), not from this value.
MARK_OPACITY = 100


def load_tgs(path):
    data = open(path, "rb").read()
    for wbits in (31, 47):
        try:
            return json.loads(zlib.decompress(data, wbits))
        except Exception:
            pass
    raise RuntimeError("cannot decompress %s" % path)


def recolor(node, color):
    """Rewrite every solid fill color so the whole badge renders in `color`."""
    if isinstance(node, dict):
        if node.get("ty") == "fl" and isinstance(node.get("c", {}).get("k"), list):
            node["c"]["k"] = list(color)
        for value in node.values():
            recolor(value, color)
    elif isinstance(node, list):
        for value in node:
            recolor(value, color)


def set_opacity(node, opacity):
    """Force every fill/stroke opacity to `opacity` (0..100) so the artwork is
    semi-transparent; the client keeps this alpha when it tints the badge."""
    if isinstance(node, dict):
        if node.get("ty") in ("fl", "st") and isinstance(node.get("o"), dict):
            node["o"]["k"] = opacity
        for value in node.values():
            set_opacity(value, opacity)
    elif isinstance(node, list):
        for value in node:
            set_opacity(value, opacity)


# (label, doc_id, access_hash, source file, recolor target)
ITEMS = [
    ("Major", 7800000000000000025, 7800000000000000125, "Badge.tgs", BLUE),
    ("Hold", 7800000000000000026, 7800000000000000126, "Hold_adaptive.tgs", BLUE),
]

SET_ID = 7800000000000000000
SET_ACCESS_HASH = 7800000000000000099

docs = []
previews = []
for name, doc_id, access_hash, source, color in ITEMS:
    lottie = load_tgs(os.path.join(SRC, source))
    recolor(lottie, color)
    set_opacity(lottie, MARK_OPACITY)
    previews.append((name, lottie))
    raw = json.dumps(lottie, separators=(",", ":")).encode()
    packed = gzip.compress(raw, mtime=0)
    with open(os.path.join(STICKERS, f"{doc_id}.tgs"), "wb") as fh:
        fh.write(packed)
    docs.append(
        {
            "id": doc_id,
            "access_hash": access_hash,
            "file_reference": "",
            "date": "2026-11-12T00:00:00Z",
            "mime_type": "application/x-tgsticker",
            "size": len(packed),
            "dc_id": 2,
            "attributes": [
                {"_": "DocumentAttributeImageSize", "w": 512, "h": 512},
                {
                    "_": "DocumentAttributeCustomEmoji",
                    "alt": "\u2705" if name == "Major" else "\u23f3",
                    "free": False,
                    "text_color": True,
                    "stickerset": {"id": SET_ID, "access_hash": SET_ACCESS_HASH},
                },
                {"_": "DocumentAttributeFilename", "file_name": f"{name}.tgs"},
            ],
            "thumbs": [],
        }
    )

set_info = {
    "input_set_type": "inputStickerSetID",
    "result": {
        "set": {
            "id": SET_ID,
            "access_hash": SET_ACCESS_HASH,
            "title": "Verifier marks",
            "short_name": "verifiermarks",
            "count": len(docs),
            # Bump whenever the document set changes: the seed importer skips a
            # set whose hash is unchanged.
            "hash": 78000005,
            "archived": False,
            "official": False,
            "masks": False,
            "emojis": True,
            "thumbs": [],
            "thumb_dc_id": 2,
            "thumb_version": 0,
            "packs": [
                {"emoticon": "\u2705", "documents": [docs[0]["id"]]},
                {"emoticon": "\u23f3", "documents": [docs[1]["id"]]},
            ],
        },
        "packs": [
            {"emoticon": "\u2705", "documents": [docs[0]["id"]]},
            {"emoticon": "\u23f3", "documents": [docs[1]["id"]]},
        ],
        "documents": docs,
    },
}

with open(os.path.join(SET_DIR, "set_info.json"), "w", encoding="utf-8") as fh:
    json.dump(set_info, fh, ensure_ascii=False, indent=2)

# A self-contained preview of exactly what ships in the seed, so the visual
# reference can never drift from the generated .tgs files.
preview_items = json.dumps([lottie for _, lottie in previews], separators=(",", ":"))
preview_boxes = "".join(
    f'<div><div class="b" id="c{i}"></div><p>{name}</p></div>'
    for i, (name, _) in enumerate(previews)
)
preview_html = (
    '<!doctype html><html><head><meta charset="utf-8"><style>'
    "body{background:#111;color:#ddd;font-family:sans-serif;display:flex;gap:40px;padding:30px}"
    "div.b{width:220px;height:220px;background:#222;border-radius:12px}</style></head><body>\n"
    '<script src="https://unpkg.com/lottie-web@5.12.2/build/player/lottie.min.js"></script>\n'
    f"{preview_boxes}\n"
    "<script>\n"
    f"const items = {preview_items};\n"
    "items.forEach((a,i)=>{lottie.loadAnimation({container:document.getElementById('c'+i),renderer:'svg',loop:true,autoplay:true,animationData:a});});\n"
    "</script></body></html>\n"
)
with open(os.path.join(BASE, "icons-preview.html"), "w", encoding="utf-8") as fh:
    fh.write(preview_html)

print("set:", SET_DIR)
print("preview:", os.path.join(BASE, "icons-preview.html"))
for name, doc_id, _, source, _ in ITEMS:
    p = os.path.join(STICKERS, f"{doc_id}.tgs")
    print(name, doc_id, os.path.getsize(p), "bytes  <-", source)
