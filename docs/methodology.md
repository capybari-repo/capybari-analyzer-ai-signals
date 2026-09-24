# Methodology: AI-Generation Signals (experimental)

## Principles

1. **Indicators, not proof.** People write generic copy too. Findings are phrased as indicators, and phrase-based ones are low confidence.
2. **Use ≠ risk.** AI assistants and builders are reported as information (zero score penalty). Only signs of *unreviewed* output lower the score.
3. **Transparent rules.** Every phrase is listed in `rules/phrases.yaml`.

## Website

**What is read:** the front page plus up to 5 same-site pages linked from it, fetched by the Website Snapshot (politely: one at a time, `robots.txt` honoured, never login/logout/admin/cart/file links). Visible text is the markup without scripts, styles, SVG and templates, with entities decoded.

**Not enough content:** if all pages together contain fewer than **150 words** of visible text, the capability reports **not assessable** and no AI Dependability score is shown. A near-empty page, or a JavaScript app that renders its text in the browser, must not score a perfect 100 simply because there was nothing to read.

| Rule | Trigger | Severity | Confidence |
|---|---|---|---|
| `ai-boilerplate-density` | ≥ 3 distinct stock phrases (of 67) **and** ≥ 5 per 1,000 words | low | low |
| | ≥ 6 distinct and ≥ 15 per 1,000 words | medium | low |
| | ≥ 10 distinct and ≥ 30 per 1,000 words | high | medium |
| `placeholder-content` | any placeholder fragment (lorem ipsum, "Your Company", 555 numbers, example@example.com) | high: visible broken content | high |
| `template-leftover` | default theme/builder text | low | medium |
| `ai-builder` | builder fingerprint in the markup of any page (generator tags, badges, asset hosts: Lovable, v0, Bolt.new, Framer, Wix, Durable, 10Web, Hostinger AI, Base44) or detected by `web-tech` | info | high |

Evidence always includes at least one sample from every page that contributed.

**Basis:** the result always states what was checked, e.g. *"Checked 3 pages, 1,240 words: 0 of 67 stock phrases found (0.0 per 1,000 words); no placeholders; no template leftovers; no AI builder detected."* The same line appears on the score card, so 100 is never an unexplained number.

## Repository

| Rule | Trigger | Severity |
|---|---|---|
| `ai-assistant-config` | `.cursorrules`, `.cursor/rules/`, `CLAUDE.md`, `AGENTS.md`, `.github/copilot-instructions.md`, `.windsurfrules`, `.clinerules`, `.aider.conf.yml` | info |
| `ai-builder` | Lovable / Bolt.new / v0 markers in `package.json`, `README.md` or `index.html` | info |
| `scaffold-comments` | scaffolding comments in source files | low; medium at ≥ 5 |

## Score

The standard scoring method, in dimension `ai-signals` ("AI Dependability"), always shown with low confidence because the capability is experimental.
