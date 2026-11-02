# MY LOCAL NAME · icon candidates

5 baroque app-icon directions, drawn as flat vector on a 1024 × 1024 canvas. The mark reads first; ornament is secondary.

## Palette & construction

- Background: `#140A11` (ink-plum), full bleed, no rounded corners.
- Cream mark: `#F7E9E4`.
- Accent: `#FF5D8F` (bright pink) or `#D61F5C` (deep rose).
- 2–3 colour families only; no gradients, text, shadows, masks or baked frames.
- Content stays inside the 824 px keyline, mark covers roughly 57–74% of the canvas.
- Source SVGs live in `vectors/`; 1024 previews in `preview/`; review sheet in `candidates_5up.png`.

## The 5 candidates

| # | Name | Idea |
| --- | --- | --- |
| 01 | Medallion | Baroque cartouche framing a pink mDNS signal |
| 02 | Crown | Three-point crown with pink jewels |
| 03 | Shell | Rococo scallop shell with cream rim and hinge |
| 04 | Beacon | Stepped lighthouse/obelisk with signal arcs |
| 05 | Wax seal | Deep-rose scalloped seal with cream signal |

Review sheet: `candidates_5up.png`. 48 / 16 / grayscale audit: `../../build/icon_concepts/final_audit.png`.

## AI concept drafts

The first pass used `zhipu/cogview-4-250304` at 1024 × 1024. Those drafts are kept in `ai_drafts/` with their prompts. They are **not** final icons: the provider adds an “AI生成” watermark and tends to add frames/scenes, so they were used only as Route H references and then redrawn as SVG. Do not ship them.

```yaml
model: zhipu/cogview-4-250304
size: 1024x1024
route: A (concept) -> H (vector redraw)
```

## Validate / preview

```bash
python ~/.pi/agent/skills/my-app-icon/scripts/validate_svg.py vectors/01_medallion.svg --grid 1024 --strict
python ~/.pi/agent/skills/my-app-icon/scripts/export_preview.py vectors/01_medallion.svg --out preview --preview-only
```

All five vectors pass `validate_svg.py --grid 1024 --strict` with no warnings.

## File map

```
vectors/        final SVG sources (01–05)
preview/        1024 PNG previews
candidates_5up.png
ai_drafts/      watermarked AI concepts + prompts (reference only)
alternates/     earlier keyhole experiment
```
