# capybari-analyzer-ai-signals (experimental)

**Capybari Source Intelligence: AI-Generation Signals: does this look like unreviewed AI output?**

This powers the **AI Slop Check**. It separates AI *use* (fine, and reported as information) from signs of *unreviewed* output (the actual risk):

| Target | Signals |
|---|---|
| Website | density of stock AI-marketing phrases in the visible text, live placeholder content (lorem ipsum, "Your Company"), default template leftovers, AI site builders (via `web-tech`) |
| Repository | AI assistant configuration (Cursor, Claude Code, Copilot, Windsurf, Cline, Aider; informational only), AI builder markers (Lovable, Bolt.new, v0), scaffolding comments left in source ("in a real application you would…", "mock data for now") |

It produces an **AI Dependability** score (100 = no indicators), always with **low confidence**, and every finding is tagged `experimental`.

No AI model is used, and nothing is sent anywhere.

| | |
|---|---|
| Uses | `web-snapshot`, `technologies` (websites); `inventory` (repositories) |
| Scores | AI Dependability (experimental) |
| Rules | [`rules/phrases.yaml`](rules/phrases.yaml) |

## License

Apache-2.0
