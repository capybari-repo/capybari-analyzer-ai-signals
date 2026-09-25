# Methodology: AI-Generation Signals (experimental)

## Principles

1. **Indicators, not proof.** People write generic copy too. Findings are phrased as indicators, and phrase-based ones are low confidence.
2. **Use ≠ risk.** AI assistants and builders are reported as information (zero score penalty). Only signs of *unreviewed* output lower the score.
3. **Transparent rules.** Every phrase is listed in `rules/phrases.yaml`.

## Website

**What is read:** the front page plus up to 5 same-site pages linked from it, fetched by the Website Snapshot (politely: one at a time, `robots.txt` honoured, never login/logout/admin/cart/file links). Visible text is the markup without scripts, styles, SVG and templates, with entities decoded.

**Not enough content:** the stock-phrase density check needs **150 words** of visible text across all pages. Between **100 and 149 words** only the unambiguous checks run (placeholders, template leftovers, generator defaults, builders). Below 100 words the capability reports **not assessable**, unless an unambiguous sign is present: a one-prompt page with a default "Vite + React + TS" title needs no amount of text to recognise, whereas a near-empty page or a JavaScript app behind a login must not score as clean simply because there was nothing to read.

| Rule | Trigger | Severity | Confidence |
|---|---|---|---|
| `ai-boilerplate-density` | ≥ 3 distinct stock phrases (of 67) **and** ≥ 5 per 1,000 words | low | low |
| | ≥ 6 distinct and ≥ 15 per 1,000 words | medium | low |
| | ≥ 10 distinct and ≥ 30 per 1,000 words | high | medium |
| `placeholder-content` | any placeholder fragment (lorem ipsum, "Your Company", 555 numbers, example@example.com) | high: visible broken content | high |
| `template-leftover` | default theme/builder text | low | medium |
| `scaffold-defaults` (category `template-leftover`) | project generator defaults in the page head: default title ("Vite + React + TS", "React App", "Create Next App", `vite_react_shadcn_ts`…), the generator's default description ("Lovable Generated Project"…), the default Vite favicon, a builder's default share image | medium | high |
| `ai-builder` | builder fingerprint in the markup of any page (generator tags, badges, asset hosts: Lovable, v0, Bolt.new, Framer, Wix, Durable, 10Web, Hostinger AI, Base44) or detected by `web-tech` | info | high |

Evidence always includes at least one sample from every page that contributed.

**Basis:** the result always states what was checked, e.g. *"Checked 3 pages, 1,240 words: 0 of 67 stock phrases found (0.0 per 1,000 words); no placeholders; no template leftovers; no AI builder detected."* The same line appears on the score card, so 100 is never an unexplained number.

## Repository

| Rule | Trigger | Severity |
|---|---|---|
| `ai-assistant-config` | `.cursorrules`, `.cursor/rules/`, `CLAUDE.md`, `AGENTS.md`, `.github/copilot-instructions.md`, `.windsurfrules`, `.clinerules`, `.aider.conf.yml` | info |
| `ai-builder` | Lovable / Bolt.new / v0 markers in `package.json`, `README.md` or `index.html` | info |
| `scaffold-comments` | scaffolding comments in source files | by density* |
| `swallowed-errors` | empty `catch {}` / `.catch(() => {})` (JS/TS, Java, Kotlin, C#, PHP, Dart, Swift, Scala), `except …: pass` (Python), `if err != nil {}` (Go), in shipping code only | by density*, medium confidence |
| `placeholder-config` | quoted placeholder values (`"YOUR_API_KEY_HERE"`, `"<your-token>"`, `"changeme"`, `"replace-me"`, `"sk-xxxxxxxx"`) or `api.example.com` URLs in shipping source/config, **excluding comment lines** (documentation examples) | by density*, high confidence |

\* **Density severity:** high when ≥ 10 occurrences, or ≥ 3 at ≥ 5 per 1,000 shipping source lines; medium when ≥ 3, or ≥ 1 per 1,000 lines; otherwise low. Four scaffolding comments in 25 lines is far worse than four in 50,000.

"Shipping code" excludes tests, fixtures, examples, docs and files named `*example*`, `*sample*` and `*template*` (such as `.env.example`).

## Score

This capability no longer produces a score of its own. Its findings feed the **AI Slop Score**, a composite meter computed by capybari-core (0 = clean, 100 = pure slop). For websites it also publishes `site-depth` evidence (signs of effort: content pages, specific figures and names, finished metadata, trust pages, extra craft), which capybari-core turns into the **Build Depth** score (0 = shallow, 100 = deep build). See the [scoring methodology](https://github.com/capybari-repo/capybari-docs/blob/main/methodology/scoring.md#ai-slop).
