#!/usr/bin/env python3
"""Deterministically export Lintasan's hand-authored SVG mark to PNG/ICO."""
from pathlib import Path
from io import BytesIO
import cairosvg
from PIL import Image

ROOT = Path(__file__).resolve().parents[1]
STATIC = ROOT / "frontend" / "static"
SOURCE = STATIC / "lintasan-mark.svg"

sizes = {
    "favicon-16.png": 16,
    "favicon-32.png": 32,
    "favicon-48.png": 48,
    "apple-touch-icon.png": 180,
    "favicon-192.png": 192,
    "lintasan-mark-512.png": 512,
    "favicon.png": 32,
}

images: dict[int, Image.Image] = {}
for filename, size in sizes.items():
    rendered = cairosvg.svg2png(url=str(SOURCE), output_width=size, output_height=size)
    if not isinstance(rendered, bytes):
        raise RuntimeError(f"SVG export returned no bytes for {filename}")
    (STATIC / filename).write_bytes(rendered)
    images[size] = Image.open(BytesIO(rendered)).convert("RGBA")

images[48].save(
    STATIC / "favicon.ico",
    format="ICO",
    sizes=[(16, 16), (32, 32), (48, 48)],
    append_images=[images[32], images[16]],
)

# Browsers conventionally look for this exact name.
(STATIC / "favicon.svg").write_bytes(SOURCE.read_bytes())
print(f"exported {len(sizes) + 2} brand assets from {SOURCE.relative_to(ROOT)}")
