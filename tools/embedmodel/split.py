"""Splits the embedding model into parts for the mindmap-models repository.

jsDelivr, the mirror the app falls back to when GitHub is out of reach,
serves files of a repository up to 20 MB, so the model goes in parts of
15 MB, with manifest.json giving each part's size and SHA-256 and the
whole's, which the app checks.

    python split.py mindmap-embed.bin OUTDIR VERSION
"""

import hashlib
import json
import os
import sys

src, out, version = sys.argv[1], sys.argv[2], int(sys.argv[3])
PART = 15 * 1024 * 1024
data = open(src, "rb").read()
os.makedirs(out, exist_ok=True)
for f in os.listdir(out):
    if f.startswith("mindmap-embed.bin."):
        os.remove(os.path.join(out, f))
parts = []
for i in range(0, len(data), PART):
    chunk = data[i : i + PART]
    name = f"mindmap-embed.bin.{i // PART:03d}"
    open(os.path.join(out, name), "wb").write(chunk)
    parts.append({"file": name, "size": len(chunk), "sha256": hashlib.sha256(chunk).hexdigest()})
manifest = {
    "name": "potion-multilingual-128M（中日英，256 维）",
    "version": version,
    "size": len(data),
    "sha256": hashlib.sha256(data).hexdigest(),
    "parts": parts,
}
json.dump(manifest, open(os.path.join(out, "manifest.json"), "w"), ensure_ascii=False, indent=2)
print(json.dumps(manifest, ensure_ascii=False, indent=2))
