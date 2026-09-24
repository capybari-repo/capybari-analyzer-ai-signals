package aisignals_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aisignals "github.com/capybari/capybari-analyzer-ai-signals"
	"github.com/capybari/capybari-core/analyzer"
	"github.com/capybari/capybari-core/analyzertest"
	"github.com/capybari/capybari-core/finding"
	"github.com/capybari/capybari-schemas"
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

func TestWebsiteFixture(t *testing.T) {
	body, _ := os.ReadFile("../capybari-fixtures/static-site/index.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(body)
	}))
	defer srv.Close()
	r := analyzertest.Run(t, aisignals.New(), analyzertest.Website(srv.URL), analyzertest.Options{Online: true})
	got := byCategory(r.Findings)
	b, ok := got["ai-boilerplate"]
	if !ok || b.Confidence != finding.ConfidenceLow {
		t.Fatalf("boilerplate: %+v", r.Findings)
	}
	if p, ok := got["placeholder-content"]; !ok || !strings.Contains(p.Evidence[0].Detail+p.Evidence[1].Detail, "lorem ipsum") {
		t.Fatalf("placeholders: %+v", p)
	}
	s := r.Scores[0]
	if s.ID != finding.DimAISignals || s.Confidence != finding.ConfidenceLow || s.Value >= 100 {
		t.Fatalf("score: %+v", s)
	}
	for _, f := range r.Findings {
		if !contains(f.Tags, "experimental") {
			t.Fatalf("finding not tagged experimental: %+v", f)
		}
	}
}

func TestCleanWebsite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><h1>Harbour Street Bakery</h1><p>Sourdough baked at 5am every day since 1998. Open Tuesday to Sunday, 7am to 2pm. Call 0161 496 0000 to order a celebration cake with two days' notice.</p><p>Whether you're a regular or new, say hello.</p></body></html>`))
	}))
	defer srv.Close()
	r := analyzertest.Run(t, aisignals.New(), analyzertest.Website(srv.URL), analyzertest.Options{Online: true})
	if len(r.Findings) != 0 {
		t.Fatalf("specific human copy flagged: %+v", r.Findings)
	}
}

func TestRepository(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"CLAUDE.md":    "# conventions\n",
		"package.json": `{"devDependencies":{"lovable-tagger":"^1.0.0"}}`,
		"src/api.ts":   "export function pay() {\n  // In a real application, you would call the payment provider here\n  return mockData;\n}\n// Mock data for now\nconst mockData = {};\n",
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
	sc := got["scaffold-code"]
	if sc.Severity != finding.Low || len(sc.Evidence) != 2 || sc.Evidence[0].StartLine != 2 {
		t.Fatalf("scaffold: %+v", sc)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
