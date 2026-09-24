// Package aisignals implements the experimental AI-Generation Signals
// capability. It reports indicators, never proof, and keeps AI *use* (which
// is fine) separate from signs of *unreviewed* output (which is the risk).
package aisignals

import (
	"context"
	_ "embed"
	"fmt"
	"html"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/capybari-repo/capybari-core/analyzer"
	"github.com/capybari-repo/capybari-core/facts"
	"github.com/capybari-repo/capybari-core/finding"
	"github.com/capybari-repo/capybari-core/fsutil"
)

//go:embed capability.yaml
var capabilityYAML []byte

//go:embed rules/phrases.yaml
var phrasesYAML []byte

var capability = analyzer.MustParseCapability(capabilityYAML)

type phraseSet struct {
	Boilerplate       []string `yaml:"boilerplate"`
	Placeholders      []string `yaml:"placeholders"`
	TemplateLeftovers []string `yaml:"template_leftovers"`
	ScaffoldComments  []string `yaml:"scaffold_comments"`
}

type pattern struct {
	raw string
	re  *regexp.Regexp
}

var rules = func() map[string][]pattern {
	var ps phraseSet
	if err := yaml.Unmarshal(phrasesYAML, &ps); err != nil {
		panic(err)
	}
	compile := func(list []string) []pattern {
		var out []pattern
		for _, p := range list {
			prefix := `(?i)`
			if c := p[0]; (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				prefix += `\b` // whole-word start only where a word character begins the phrase
			}
			out = append(out, pattern{p, regexp.MustCompile(prefix + p)})
		}
		return out
	}
	return map[string][]pattern{
		"boilerplate": compile(ps.Boilerplate), "placeholders": compile(ps.Placeholders),
		"leftovers": compile(ps.TemplateLeftovers), "scaffold": compile(ps.ScaffoldComments),
	}
}()

var aiBuilders = map[string]bool{"Lovable": true, "v0": true, "Bolt.new": true, "Durable": true, "Framer": true, "Wix": true}

// Analyzer implements the capability.
type Analyzer struct{}

// New returns the capability.
func New() *Analyzer { return &Analyzer{} }

// Capability implements analyzer.Analyzer.
func (*Analyzer) Capability() analyzer.Capability { return capability }

// Applies declines when there is nothing to read.
func (*Analyzer) Applies(in *analyzer.Input) (bool, string) {
	if in.Target.Kind == analyzer.TargetWebsite && !in.Evidence.Has(facts.KeyWebSnapshot) {
		return false, "no page content was fetched"
	}
	if in.Target.Kind == analyzer.TargetRepository && !in.Evidence.Has(facts.KeyInventory) {
		return false, "no inventory"
	}
	return true, ""
}

// Analyze implements analyzer.Analyzer.
func (*Analyzer) Analyze(ctx context.Context, in *analyzer.Input) (*analyzer.Result, error) {
	var tech facts.Technologies
	in.Evidence.Get(facts.KeyTechnologies, &tech)
	var fs []finding.Finding
	var summary string
	if in.Target.Kind == analyzer.TargetWebsite {
		var ws facts.WebSnapshot
		if _, err := in.Evidence.Get(facts.KeyWebSnapshot, &ws); err != nil {
			return nil, err
		}
		fs, summary = website(&ws, &tech)
	} else {
		var inv facts.Inventory
		if _, err := in.Evidence.Get(facts.KeyInventory, &inv); err != nil {
			return nil, err
		}
		var err error
		fs, summary, err = repository(ctx, in.Target.Root, &inv)
		if err != nil {
			return nil, err
		}
	}
	return &analyzer.Result{
		Findings: fs,
		Summary:  summary,
		Limitations: []string{
			"Experimental heuristics. These are indicators of unreviewed AI-generated output, not proof of AI use, and not a measure of quality on their own.",
		},
	}, nil
}

var (
	scriptStyle = regexp.MustCompile(`(?is)<(script|style|noscript|svg)[^>]*>.*?</(script|style|noscript|svg)>`)
	tags        = regexp.MustCompile(`(?s)<[^>]+>`)
	spaces      = regexp.MustCompile(`\s+`)
)

// visibleText approximates the text a visitor reads.
func visibleText(body string) string {
	t := scriptStyle.ReplaceAllString(body, " ")
	t = tags.ReplaceAllString(t, " ")
	t = html.UnescapeString(t)
	return strings.TrimSpace(spaces.ReplaceAllString(t, " "))
}

type match struct {
	phrase string
	count  int
	sample string
}

func find(text string, ps []pattern) (total int, ms []match) {
	for _, p := range ps {
		locs := p.re.FindAllStringIndex(text, -1)
		if len(locs) == 0 {
			continue
		}
		s := text[max(0, locs[0][0]-40):min(len(text), locs[0][1]+40)]
		ms = append(ms, match{p.raw, len(locs), "…" + strings.TrimSpace(s) + "…"})
		total += len(locs)
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].count > ms[j].count })
	return total, ms
}

func evidence(url string, ms []match, n int) []finding.Evidence {
	var ev []finding.Evidence
	for i, m := range ms {
		if i == n {
			break
		}
		ev = append(ev, finding.Evidence{Location: finding.Location{URL: url}, Snippet: m.sample, Detail: fmt.Sprintf("%q ×%d", m.phrase, m.count)})
	}
	return ev
}

func website(ws *facts.WebSnapshot, tech *facts.Technologies) ([]finding.Finding, string) {
	text := visibleText(ws.Body)
	words := len(strings.Fields(text))
	var out []finding.Finding

	n, ms := find(text, rules["boilerplate"])
	density := 0.0
	if words > 0 {
		density = float64(n) * 1000 / float64(words)
	}
	distinct := len(ms)
	if distinct >= 3 && density >= 5 {
		sev := finding.Low
		if distinct >= 6 && density >= 15 {
			sev = finding.Medium
		}
		out = append(out, finding.Finding{
			Dimension: finding.DimAISignals, Category: "ai-boilerplate", Severity: sev, Confidence: finding.ConfidenceLow,
			Title:                 fmt.Sprintf("Copy uses %d stock phrases typical of unedited AI output", distinct),
			Description:           fmt.Sprintf("%d occurrences of %d distinct boilerplate phrases in %d words (%.1f per 1,000 words). Generic copy like this often means AI-drafted text went live without editing, and says little that is specific to the business.", n, distinct, words, density),
			Evidence:              evidence(ws.FinalURL, ms, 5),
			Rule:                  &finding.Rule{ID: "ai-boilerplate-density"},
			Remediation:           &finding.Remediation{Summary: "Rewrite key pages with specific, verifiable statements about the product, customers and results.", Automatable: false},
			FalsePositiveGuidance: "Human marketing copy uses these phrases too. Treat this as a prompt to review the text, not as proof of AI use.",
		})
	}
	if pn, pm := find(text, rules["placeholders"]); pn > 0 {
		out = append(out, finding.Finding{
			Dimension: finding.DimAISignals, Category: "placeholder-content", Severity: finding.Medium, Confidence: finding.ConfidenceHigh,
			Title:       "Placeholder content is live on the page",
			Description: fmt.Sprintf("%d placeholder fragment(s), such as lorem ipsum, 'Your Company' or dummy contact details, are visible to visitors.", pn),
			Evidence:    evidence(ws.FinalURL, pm, 5),
			Rule:        &finding.Rule{ID: "placeholder-content"},
			Remediation: &finding.Remediation{Summary: "Replace every placeholder with real content before publishing.", Automatable: false},
		})
	}
	if ln, lm := find(text+" "+ws.Title, rules["leftovers"]); ln > 0 {
		out = append(out, finding.Finding{
			Dimension: finding.DimAISignals, Category: "template-leftover", Severity: finding.Low, Confidence: finding.ConfidenceMedium,
			Title:       "Default template content left in place",
			Evidence:    evidence(ws.FinalURL, lm, 3),
			Rule:        &finding.Rule{ID: "template-leftover"},
			Remediation: &finding.Remediation{Summary: "Remove the theme or builder's default pages and text.", Automatable: false},
		})
	}
	for _, t := range tech.Items {
		if aiBuilders[t.Name] {
			out = append(out, finding.Finding{
				Dimension: finding.DimAISignals, Category: "ai-builder", Severity: finding.Info, Confidence: finding.ConfidenceHigh,
				Title:       "Built with " + t.Name,
				Description: fmt.Sprintf("%s generates or assists with site content and code. That is not a problem in itself; review generated pages for accuracy and maintainability.", t.Name),
				Evidence:    []finding.Evidence{{Location: finding.Location{URL: ws.FinalURL}, Detail: strings.Join(t.Evidence, "; ")}},
				Component:   t.Name,
				Rule:        &finding.Rule{ID: "ai-builder"},
			})
		}
	}
	return out, fmt.Sprintf("%d words; %d boilerplate phrase(s) (%.1f/1,000 words); %d finding(s)", words, n, density, len(out))
}

var assistantFiles = map[string]string{
	".cursorrules": "Cursor", ".windsurfrules": "Windsurf", ".clinerules": "Cline", "claude.md": "Claude Code",
	"agents.md": "AI coding agents", ".github/copilot-instructions.md": "GitHub Copilot", ".aider.conf.yml": "Aider",
}

var builderMarkers = []struct {
	name string
	re   *regexp.Regexp
}{
	{"Lovable", regexp.MustCompile(`(?i)lovable-tagger|gptengineer|lovable\.dev`)},
	{"Bolt.new", regexp.MustCompile(`(?i)bolt\.new|stackblitz/bolt`)},
	{"v0", regexp.MustCompile(`(?i)v0\.dev|generated by v0`)},
}

func repository(ctx context.Context, root string, inv *facts.Inventory) ([]finding.Finding, string, error) {
	var out []finding.Finding
	assistants := map[string][]string{}
	for _, f := range inv.Files {
		p := strings.ToLower(f.Path)
		if name, ok := assistantFiles[p]; ok {
			assistants[name] = append(assistants[name], f.Path)
		}
		if strings.HasPrefix(p, ".cursor/rules/") {
			assistants["Cursor"] = append(assistants["Cursor"], f.Path)
		}
	}
	names := make([]string, 0, len(assistants))
	for n := range assistants {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		var ev []finding.Evidence
		for _, p := range assistants[n] {
			ev = append(ev, finding.Evidence{Location: finding.Location{Path: p}})
		}
		out = append(out, finding.Finding{
			Dimension: finding.DimAISignals, Category: "ai-assistant-config", Severity: finding.Info, Confidence: finding.ConfidenceHigh,
			Title:       n + " is configured for this repository",
			Description: "The team uses an AI coding assistant. That is normal practice; the configuration documents conventions the assistant follows.",
			Evidence:    ev, Component: n,
			Rule: &finding.Rule{ID: "ai-assistant-config"},
		})
	}

	builders := map[string]string{}
	var scaffold []finding.Evidence
	scaffoldCount := 0
	for _, f := range inv.Files {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		base := strings.ToLower(path.Base(f.Path))
		if base == "package.json" || base == "readme.md" || base == "index.html" {
			if b, _, err := fsutil.ReadFile(root, f.Path, 256<<10); err == nil {
				for _, m := range builderMarkers {
					if _, seen := builders[m.name]; !seen && m.re.Match(b) {
						builders[m.name] = f.Path
					}
				}
			}
		}
		if f.Kind != facts.KindSource || f.Size > fsutil.DefaultMaxRead {
			continue
		}
		b, _, err := fsutil.ReadFile(root, f.Path, fsutil.DefaultMaxRead)
		if err != nil {
			continue
		}
		fsutil.Lines(b, func(n int, line string) bool {
			for _, p := range rules["scaffold"] {
				if p.re.MatchString(line) {
					scaffoldCount++
					if len(scaffold) < 10 {
						scaffold = append(scaffold, finding.Evidence{Location: finding.Location{Path: f.Path, StartLine: n}, Snippet: strings.TrimSpace(line)})
					}
					break
				}
			}
			return true
		})
	}
	bnames := make([]string, 0, len(builders))
	for n := range builders {
		bnames = append(bnames, n)
	}
	sort.Strings(bnames)
	for _, n := range bnames {
		out = append(out, finding.Finding{
			Dimension: finding.DimAISignals, Category: "ai-builder", Severity: finding.Info, Confidence: finding.ConfidenceHigh,
			Title:       "Project generated with " + n,
			Description: "Generated projects work, but they often need hardening (auth, validation, error handling, tests) before production.",
			Evidence:    []finding.Evidence{{Location: finding.Location{Path: builders[n]}}},
			Component:   n,
			Rule:        &finding.Rule{ID: "ai-builder"},
		})
	}
	if scaffoldCount > 0 {
		sev := finding.Low
		if scaffoldCount >= 5 {
			sev = finding.Medium
		}
		out = append(out, finding.Finding{
			Dimension: finding.DimAISignals, Category: "scaffold-code", Severity: sev, Confidence: finding.ConfidenceMedium,
			Title:                 fmt.Sprintf("%d scaffolding comment(s) left in source code", scaffoldCount),
			Description:           "Comments such as 'in a real application you would…' or 'mock data for now' mark code that was generated or sketched as an example and may never have been completed.",
			Evidence:              scaffold,
			Rule:                  &finding.Rule{ID: "scaffold-comments"},
			Remediation:           &finding.Remediation{Summary: "Review each marked location: implement the real behaviour (validation, persistence, auth) or remove the example code.", Automatable: false},
			FalsePositiveGuidance: "Example and documentation code legitimately contains such comments.",
		})
	}
	return out, fmt.Sprintf("%d AI assistant config(s), %d builder marker(s), %d scaffolding comment(s)", len(assistants), len(builders), scaffoldCount), nil
}
