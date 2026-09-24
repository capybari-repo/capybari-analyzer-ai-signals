# Methodology: AI-Generation Signals (experimental)

## Principles

1. **Indicators, not proof.** People write generic copy too. Findings are phrased as indicators, and phrase-based ones are low confidence.
2. **Use ≠ risk.** AI assistants and builders are reported as information (zero score penalty). Only signs of *unreviewed* output lower the score.
3. **Transparent rules.** Every phrase is listed in `rules/phrases.yaml`.

## Website

Visible text = page markup without scripts, styles, SVG and tags, with entities decoded.

| Rule | Trigger | Severity | Confidence |
|---|---|---|---|
| `ai-boilerplate-density` | ≥ 3 distinct stock phrases **and** ≥ 5 per 1,000 words | low; medium at ≥ 6 distinct and ≥ 15 per 1,000 | low |
| `placeholder-content` | any placeholder fragment | medium | high |
| `template-leftover` | default theme/builder text | low | medium |
| `ai-builder` | AI site builder detected by `web-tech` | info | high |

## Repository

| Rule | Trigger | Severity |
|---|---|---|
| `ai-assistant-config` | `.cursorrules`, `.cursor/rules/`, `CLAUDE.md`, `AGENTS.md`, `.github/copilot-instructions.md`, `.windsurfrules`, `.clinerules`, `.aider.conf.yml` | info |
| `ai-builder` | Lovable / Bolt.new / v0 markers in `package.json`, `README.md` or `index.html` | info |
| `scaffold-comments` | scaffolding comments in source files | low; medium at ≥ 5 |

## Score

The standard scoring method, in dimension `ai-signals` ("AI Dependability"), always shown with low confidence because the capability is experimental.
