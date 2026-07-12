# documents/assets/

Public marketing assets — README Hero region, docs site landing, blog.

## Files

- `demo.cast` — asciinema v2 recording script (1-line terminal demo).
  Plays back the exact 5-second sequence referenced in the README Hero
  block. To use it:

  ```bash
  brew install asciinema agg   # asciinema-cli + agg (gif renderer)
  asciinema play demo.cast
  agg demo.cast demo.gif      # produces a high-fidelity GIF
  ```

  Drop the resulting `demo.gif` here.

## Recording a fresh demo

The fastest path on macOS to record a real terminal GIF:

```bash
# Option A: Kap (records a screen region, exports GIF)
brew install --cask kap
# Open Kap, record the terminal window, export.

# Option B: ttyrec + ttygif (text-only, smaller files)
brew install ttyrec gifski
ttyrec -e hnsx\ new\ my-cs\ ...   # do the demo
ttygif ttyrecord                  # render to GIF

# Option C: QuickTime + ffmpeg
brew install ffmpeg
# Record with QuickTime Player → File > New Screen Recording
# Export to MOV, then:
ffmpeg -i in.mov -vf "fps=15,scale=720:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" out.gif
```

The README expects `demo.gif` at this exact path:

```
documents/assets/demo.gif
```

Hard cap on file size: **3 MB**. If your GIF is larger, see
"Optimizing" below.

## Optimizing

```bash
# Lossy resize for the README Hero
ffmpeg -i in.gif -vf "fps=12,scale=720:-1:flags=lanczos" -loop 0 out.gif
gifsicle -O3 out.gif -o out.gif     # re-compress in-place
```

If you need smaller, drop fps to 10 or scale to 600px wide. The Hero
region caps at 720px so going wider is wasted.

## What's NOT in this directory

- `docs/assets/` is the internal mirror (also empty for now). Don't
  add public assets there.
- Project logo / OG image / favicon live in `website/docs/public/`.