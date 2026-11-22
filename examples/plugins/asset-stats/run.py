import json
import sys

req = json.load(sys.stdin)
assets = req.get("assets", [])
add_tags = {}
total_mp = 0.0

for a in assets:
    w = a.get("width", 0)
    h = a.get("height", 0)
    total_mp += (w * h) / 1_000_000.0
    if w > 0 and h > 0:
        if w > h:
            tag = "Landscape"
        elif h > w:
            tag = "Portrait"
        else:
            tag = "Square"
        add_tags[a["id"]] = [tag]

print(json.dumps({
    "ok": True,
    "toast": f"Tagged {len(add_tags)} assets by aspect ratio ({total_mp:.1f} MP total)",
    "add_tags": add_tags,
    "reload": True
}))
