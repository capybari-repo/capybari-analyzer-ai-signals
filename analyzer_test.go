package aisignals_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aisignals "github.com/capybari-repo/capybari-analyzer-ai-signals"
	"github.com/capybari-repo/capybari-core/analyzer"
	"github.com/capybari-repo/capybari-core/analyzertest"
	"github.com/capybari-repo/capybari-core/builtin"
	"github.com/capybari-repo/capybari-core/engine"
	"github.com/capybari-repo/capybari-core/finding"
	"github.com/capybari-repo/capybari-core/report"
	"github.com/capybari-repo/capybari-schemas"
	"gopkg.in/yaml.v3"
)

func TestCapabilityMetadata(t *testing.T) {
	b, err := os.ReadFile("capability.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analyzer.ParseCapability(b); err != nil {
		t.Fatal(err)
	}
	var doc any
	yaml.Unmarshal(b, &doc)
	if err := schemas.ValidateValue("capability.schema.json", doc); err != nil {
		t.Fatal(err)
	}
}

func byCategory(fs []finding.Finding) map[string]finding.Finding {
	m := map[string]finding.Finding{}
	for _, f := range fs {
		m[f.Category] = f
	}
	return m
}

// serveDir serves static HTML files from a directory.
func serveDir(t *testing.T, dir string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.FileServer(http.Dir(dir)))
}

func serveHTML(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(body))
	}))
}

// runAny runs ai-signals through the engine without requiring status ok.
func runAny(t *testing.T, target analyzer.Target) *report.Report {
	t.Helper()
	reg := engine.NewRegistry()
	if err := reg.Register(append(builtin.All(), aisignals.New())...); err != nil {
		t.Fatal(err)
	}
	e := engine.New(engine.Config{Registry: reg, Tool: report.Tool{Name: "t", Version: "0"}})
	r, _, err := e.Analyze(context.Background(), target, engine.Selection{Only: []string{"ai-signals"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMultiPageFixture(t *testing.T) {
	srv := serveDir(t, "../capybari-fixtures/static-site")
	defer srv.Close()
	r := analyzertest.Run(t, aisignals.New(), analyzertest.Website(srv.URL), analyzertest.Options{Online: true})
	got := byCategory(r.Findings)

	b, ok := got["ai-boilerplate"]
	if !ok || b.Severity != finding.High || b.Confidence != finding.ConfidenceMedium {
		t.Fatalf("boilerplate: %+v", r.Findings)
	}
	var fromAbout bool
	for _, e := range b.Evidence {
		fromAbout = fromAbout || strings.HasSuffix(e.URL, "/about.html")
	}
	if !fromAbout {
		t.Fatalf("evidence should include the linked about page: %+v", b.Evidence)
	}
	p := got["placeholder-content"]
	if len(p.Evidence) < 2 {
		t.Fatalf("placeholders from both pages expected: %+v", p.Evidence)
	}
	if bl := got["ai-builder"]; bl.Component != "Lovable" || !strings.Contains(bl.Evidence[0].Detail, "about.html") {
		t.Fatalf("Lovable badge on the about page not detected: %+v", bl)
	}

	s := slop(t, r)
	// Website groups: AI-generation signs (assessed) + security (web-security did
	// not run in this focused test, so not assessed).
	if !s.IsHigherWorse() || s.Value < 25 || s.Label == "Low unfinished risk" || s.Confidence != finding.ConfidenceLow {
		t.Fatalf("sloppy site must read as moderate/high slop: %+v", s)
	}
	joined := strings.Join(s.Basis, "|")
	if !strings.Contains(joined, "Checked 2 pages") || !strings.Contains(joined, "Not assessed (run the full scan): Security shortcuts") {
		t.Fatalf("basis must say what was checked and what was not: %v", s.Basis)
	}
}

func slop(t *testing.T, r *report.Report) report.Score {
	t.Helper()
	for _, s := range r.Scores {
		if s.ID == "ai-slop" {
			return s
		}
	}
	t.Fatalf("no AI Slop Score in %+v", r.Scores)
	return report.Score{}
}

func TestSpecificHumanCopyScores100WithBasis(t *testing.T) {
	copy := strings.Repeat(`<p>Harbour Street Bakery has baked sourdough at 5am every day since 1998, using flour milled twelve miles away in Stoke Row.
Opening hours are Tuesday to Sunday, 7am to 2pm; on Mondays we deliver to the four cafés on Mill Lane.
Celebration cakes need two days' notice: call 0161 496 0000 and ask for Priya. Whether you're a regular or new, say hello.</p>`, 3)
	srv := serveHTML(`<html><body><h1>Harbour Street Bakery</h1>` + copy + `</body></html>`)
	defer srv.Close()
	r := analyzertest.Run(t, aisignals.New(), analyzertest.Website(srv.URL), analyzertest.Options{Online: true})
	if len(r.Findings) != 0 {
		t.Fatalf("specific human copy flagged: %+v", r.Findings)
	}
	s := slop(t, r)
	if s.Value != 0 || s.Label != "Low unfinished risk" || !strings.Contains(strings.Join(s.Basis, "|"), "no placeholders; no template leftovers; no AI builder detected") {
		t.Fatalf("clean copy must read 0 = no slop, with an explicit basis: %+v", s)
	}
}

func TestTooLittleTextIsNotAssessed(t *testing.T) {
	srv := serveHTML(`<html><head><script src="/app.js"></script></head><body><div id="root"></div><noscript>Enable JavaScript</noscript></body></html>`)
	defer srv.Close()
	r := runAny(t, analyzertest.Website(srv.URL))
	run, _ := r.Capability("ai-signals")
	if run.Status != report.StatusNotApplicable || !strings.Contains(run.Reason, "not enough content to assess") {
		t.Fatalf("status %s: %s", run.Status, run.Reason)
	}
	for _, s := range r.Scores {
		if s.ID == "ai-slop" {
			t.Fatalf("no AI Slop Score may be shown without content: %+v", s)
		}
	}
}

func TestRepository(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"CLAUDE.md":          "# conventions\n",
		"package.json":       `{"devDependencies":{"lovable-tagger":"^1.0.0"}}`,
		"src/api.ts":         "export function pay() {\n  // In a real application, you would call the payment provider here\n  return mockData;\n}\n// Mock data for now\nconst mockData = {};\n",
		"src/client.ts":      "/**\n * Example: fetch('https://api.example.com/users')\n */\nconst yourApiKey = process.env.KEY;\nconst cfg = { apiKey: \"YOUR_API_KEY_HERE\", url: \"https://api.example.com/v1\" };\nfetch(u).catch(() => {});\ntry { run(); } catch (e) {}\n",
		"app/jobs.py":        "def sync():\n    try:\n        push()\n    except Exception:\n        pass\n",
		"main.go":            "package main\n\nfunc main() {\n\terr := run()\n\tif err != nil {\n\t}\n}\n",
		"tests/fake_test.py": "try:\n    x()\nexcept Exception:\n    pass\n",
		".env.example":       "API_KEY=\"YOUR_API_KEY_HERE\"\n",
	}
	for p, b := range files {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755)
		os.WriteFile(filepath.Join(dir, p), []byte(b), 0o644)
	}
	r := analyzertest.Run(t, aisignals.New(), analyzertest.Repo(t, dir), analyzertest.Options{})
	got := byCategory(r.Findings)
	if got["ai-assistant-config"].Severity != finding.Info || got["ai-builder"].Component != "Lovable" {
		t.Fatalf("assistant/builder: %+v", r.Findings)
	}
	sw := got["swallowed-errors"]
	if len(sw.Evidence) != 4 {
		t.Fatalf("swallowed errors (2 JS, 1 Python, 1 Go; tests excluded): %+v", sw.Evidence)
	}
	pc := got["placeholder-config"]
	if len(pc.Evidence) != 1 || pc.Evidence[0].Path != "src/client.ts" || pc.Evidence[0].StartLine != 5 {
		t.Fatalf("placeholder config (identifier, doc comment and .env.example must not count): %+v", pc.Evidence)
	}
	s := slop(t, r)
	if s.Value == 0 || !s.IsHigherWorse() {
		t.Fatalf("repository slop: %+v", s)
	}
	sc := got["scaffold-code"]
	// Two scaffolding comments in ~20 lines of shipping code: dense, so medium.
	if sc.Severity != finding.Medium || len(sc.Evidence) != 2 || sc.Evidence[0].StartLine != 2 {
		t.Fatalf("scaffold: %+v", sc)
	}
}

func TestDensityScalesSeverity(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("export const v = 1;\n", 5000)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src/big.ts"), []byte(big+"// Mock data for now\n"), 0o644)
	r := analyzertest.Run(t, aisignals.New(), analyzertest.Repo(t, dir), analyzertest.Options{})
	if sc := byCategory(r.Findings)["scaffold-code"]; sc.Severity != finding.Low {
		t.Fatalf("one comment in 5,000 lines should be low: %+v", sc)
	}
}

func depth(r *report.Report) *report.Score {
	for _, s := range r.Scores {
		if s.ID == "build-depth" {
			return &s
		}
	}
	return nil
}

// A one-prompt page is short, but its generator defaults need no amount of
// text to recognise: it must be assessed, flagged and read as shallow.
func TestShortPageWithGeneratorDefaults(t *testing.T) {
	srv := serveHTML(`<!doctype html><html lang="en"><head><title>Vite + React + TS</title><link rel="icon" type="image/svg+xml" href="/vite.svg">
<meta name="description" content="Lovable Generated Project"></head><body><div id="root"><h1>Welcome</h1><p>Build faster with us.</p></div></body></html>`)
	defer srv.Close()
	r := runAny(t, analyzertest.Website(srv.URL))
	run, _ := r.Capability("ai-signals")
	if run.Status != report.StatusOK || !strings.Contains(run.Summary, "stock-phrase check skipped") {
		t.Fatalf("short page with defaults must be assessed, copy not judged: %s %s %s", run.Status, run.Reason, run.Summary)
	}
	f, ok := byCategory(r.Findings)["template-leftover"]
	if !ok || f.Rule.ID != "scaffold-defaults" || len(f.Evidence) != 3 {
		t.Fatalf("generator defaults: %+v", r.Findings)
	}
	d := depth(r)
	if d == nil || d.Value >= 35 || d.Label != "Thin build" || d.IsHigherWorse() {
		t.Fatalf("one-prompt page must read as a shallow build: %+v", d)
	}
}

// Build Depth must rank a crafted site well above a generic one, whatever
// tools built either.
func TestBuildDepthRanksCraftAboveGeneric(t *testing.T) {
	generic := serveHTML(`<html><head><title>Home</title></head><body><h1>Welcome</h1><p>` +
		strings.Repeat("Unlock your potential with our cutting-edge, seamless solutions that empower your journey and elevate your experience. ", 12) + `</p></body></html>`)
	defer generic.Close()
	crafted := serveHTML(`<!doctype html><html lang="en"><head><title>Tallyroom — café rotas</title><meta name="viewport" content="width=device-width">
<meta name="description" content="Build next week's café rota from staff availability in minutes."><meta property="og:title" content="Tallyroom"><meta property="og:image" content="/og.png">
<link rel="icon" href="/favicon.svg"><link rel="canonical" href="/"><link rel="alternate" hreflang="de" href="/de/"><link rel="manifest" href="/site.webmanifest">
<script type="application/ld+json">{"@type":"SoftwareApplication"}</script></head><body><nav><a href="/about">About</a> <a href="/privacy">Privacy</a> <a href="mailto:hello@tallyroom.co.uk">Email us</a></nav><main>
<p>Tallyroom builds next week's rota in about 4 minutes. It knows that Priya can't open on Tuesdays and that you need 2 baristas between 8 and 10.</p>
<p>We started at Harbour Street Bakery in Leith. Today 214 cafés in Scotland use it, most with 4 to 15 staff, for £19 a month.</p>
<p>It flags rotas that break the 11 hours of rest between shifts, and exports hours to Xero every 14 days.</p>
<p>Tallyroom is not a till or an HR system. If you run more than 3 sites, a larger tool will suit you better, and Aisha in support will tell you so.</p>
<form><label for="e">Email</label><input id="e" type="email"><button type="submit">Start</button></form></main></body></html>`)
	defer crafted.Close()
	g, c := depth(runAny(t, analyzertest.Website(generic.URL))), depth(runAny(t, analyzertest.Website(crafted.URL)))
	if g == nil || c == nil {
		t.Fatalf("both sites must get Build Depth: %+v %+v", g, c)
	}
	if g.Value >= 35 || c.Value < 65 || c.Value-g.Value < 40 {
		t.Fatalf("crafted %d vs generic %d: expected deep vs shallow", c.Value, g.Value)
	}
	if len(c.Components) != 6 || c.Components[0].Max != 15 {
		t.Fatalf("components with max points expected: %+v", c.Components)
	}
}
