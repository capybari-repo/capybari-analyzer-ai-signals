# capybari-analyzer-ai-signals (experimental)

**Capybari Source Intelligence: AI-Generation Signals: does this look like unreviewed AI output?**

This powers the **AI Slop Check**. It separates AI *use* (fine, and reported as information) from signs of *unreviewed* output (the actual risk):

| Target | Signals |
|---|---|
| Website | the front page **and up to 5 linked pages**: density of 67 stock AI-marketing phrases, live placeholder content (lorem ipsum, "Your Company"), default template leftovers, AI site-builder fingerprints in the markup (Lovable, v0, Bolt.new, Framer, Wix, Durable, 10Web, Hostinger AI, Base44). Sites with under 150 words of visible text are reported as **not assessable** instead of scoring 100 |
| Repository | AI assistant configuration (Cursor, Claude Code, Copilot, Windsurf, Cline, Aider; informational only), AI builder markers (Lovable, Bolt.new, v0), scaffolding comments left in source ("in a real application you would…", "mock data for now") |

It produces an **AI Dependability** score (100 = no indicators found), always with **low confidence** and a **basis** line stating what was checked. Every finding is tagged `experimental`.

No AI model is used, and nothing is sent anywhere.

| | |
|---|---|
| Uses | `web-snapshot`, `technologies` (websites); `inventory` (repositories) |
| Scores | AI Dependability (experimental) |
| Rules | [`rules/phrases.yaml`](rules/phrases.yaml) |

## License

Apache-2.0
