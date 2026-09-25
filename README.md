# capybari-analyzer-ai-signals (experimental)

**Capybari Source Intelligence: AI-Generation Signals: does this look like unreviewed AI output?**

This powers the **AI Slop Check**. It separates AI *use* (fine, and reported as information) from signs of *unreviewed* output (the actual risk):

| Target | Signals |
|---|---|
| Website | the front page **and up to 5 linked pages**: density of 67 stock AI-marketing phrases, live placeholder content (lorem ipsum, "Your Company"), default template leftovers, AI site-builder fingerprints in the markup (Lovable, v0, Bolt.new, Framer, Wix, Durable, 10Web, Hostinger AI, Base44). Generator defaults left in the page head ("Vite + React + TS", "Lovable Generated Project"). Also measures **Build Depth** signs of effort (content pages, specific facts, finished metadata, trust pages, extra craft). Sites under 100 words with no unambiguous sign are reported as **not assessable**; the stock-phrase check needs 150 words |
| Repository | AI assistant configuration (Cursor, Claude Code, Copilot, Windsurf, Cline, Aider; informational only), AI builder markers (Lovable, Bolt.new, v0), scaffolding comments left in source ("in a real application you would…", "mock data for now"), **swallowed errors** (empty `catch {}`, `except: pass`, `if err != nil {}`) and **placeholder configuration** (`"YOUR_API_KEY_HERE"`, `"changeme"`, `api.example.com`) in shipping code |

Its findings feed the **AI Slop Score**, a composite meter (0 = clean, 100 = pure slop) computed by `capybari-core` together with evidence from the security, dependency and code-health capabilities. See the [scoring methodology](https://github.com/capybari-repo/capybari-docs/blob/main/methodology/scoring.md#ai-slop). Every finding is tagged `experimental`.

No AI model is used, and nothing is sent anywhere.

| | |
|---|---|
| Uses | `web-snapshot`, `technologies` (websites); `inventory` (repositories) |
| Scores | feeds the AI Slop Score (composite, computed in capybari-core) |
| Rules | [`rules/phrases.yaml`](rules/phrases.yaml) |

## License

Apache-2.0
