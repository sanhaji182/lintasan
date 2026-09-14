# Lintasan Jalur / Interchange identity

## Rationale

Three incoming routes bend and converge into one continuous exit. The shared vertical and lower-right stroke subtly forms an **L**, so the mark reads as Lintasan before it reads as a generic network diagram. Rounded terminals keep the silhouette clear at favicon scale, while electric blue → cyan communicates a fast, modern control plane.

The pale-blue light tile and deep-slate dark tile preserve contrast without changing the route geometry. The UI renders the same hand-authored paths through `LogoMark.svelte`; standalone files remain portable assets with accessible `<title>` and `<desc>` elements.

## Asset inventory

- `frontend/static/lintasan-mark.svg` — symbol, light
- `frontend/static/lintasan-mark-dark.svg` — symbol, dark
- `frontend/static/lintasan-wordmark.svg` — horizontal wordmark, light
- `frontend/static/lintasan-wordmark-dark.svg` — horizontal wordmark, dark
- `frontend/static/favicon.svg`, `favicon.ico`, `favicon-16.png`, `favicon-32.png`, `favicon-48.png`
- `frontend/static/favicon-192.png` — PWA icon
- `frontend/static/apple-touch-icon.png` — 180px touch icon
- `frontend/static/lintasan-mark-512.png` — transparent-share/app master export
- `frontend/static/site.webmanifest` — install metadata
- `scripts/export_brand_assets.py` — deterministic SVG → PNG/ICO exporter (CairoSVG 2.9.1, Pillow 12.3.0)

## Usage

Keep clear space around the symbol equal to one route stroke (7/64 of its size). Do not recolor individual routes, add nodes/sparkles, rotate the mark, or place the light tile on a similarly pale blue surface without a boundary.
