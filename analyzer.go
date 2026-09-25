// Package aisignals implements the experimental AI-Generation Signals
// capability. It reports indicators, never proof, and keeps AI *use* (which
// is fine) separate from signs of *unreviewed* output (which is the risk).
package aisignals

import (
	"context"
	_ "embed"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/capybari-repo/capybari-core/analyzer"
	"github.com/capybari-repo/capybari-core/facts"
	"github.com/capybari-repo/capybari-core/finding"
	"github.com/capybari-repo/capybari-core/fsutil"
	"github.com/capybari-repo/capybari-core/webtext"
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
	PlaceholderConfig []string `yaml:"placeholder_config"`
	Builders          []struct {
		Name    string `yaml:"name"`
		Pattern string `yaml:"pattern"`
	} `yaml:"builders"`
}

type builderRule struct {
	name string
	re   *regexp.Regexp
}

// MinWords is the least visible text needed to assess a website: below it
// there is too little copy for the indicators to mean anything.
const MinWords = 150

type pattern struct {
	raw string
	re  *regexp.Regexp
}

var builderRules []builderRule

var rules = func() map[string][]pattern {
	var ps phraseSet
	if err := yaml.Unmarshal(phrasesYAML, &ps); err != nil {
		panic(err)
	}
	for _, b := range ps.Builders {
		builderRules = append(builderRules, builderRule{b.Name, regexp.MustCompile(b.Pattern)})
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
		"placeholder-config": compile(ps.PlaceholderConfig),
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
	if in.Target.Kind == analyzer.TargetWebsite {
		var ws facts.WebSnapshot
		if ok, _ := in.Evidence.Get(facts.KeyWebSnapshot, &ws); !ok {
			return false, "no page content was fetched"
		}
		if words := totalWords(&ws); words < MinWords {
			return false, fmt.Sprintf("not enough content to assess: only %d words of visible text across %d page(s) (at least %d needed); pages rendered entirely by JavaScript show little text to a scanner", words, 1+len(ws.Pages), MinWords)
		}
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

type page struct {
	url, text, html, title string
}

func pagesOf(ws *facts.WebSnapshot) []page {
	ps := []page{{url: ws.FinalURL, text: webtext.Visible(ws.Body), html: ws.Body, title: ws.Title}}
	for _, p := range ws.Pages {
		ps = append(ps, page{url: p.URL, text: p.Text, html: p.HTML, title: p.Title})
	}
	return ps
}

func totalWords(ws *facts.WebSnapshot) int {
	n := 0
	for _, p := range pagesOf(ws) {
		n += webtext.Words(p.text)
	}
	return n
}

type match struct {
	phrase, url, sample string
	count               int
}

// find counts pattern matches across pages; one match per phrase keeps the
// first page and sample where it appeared.
func find(ps []page, pats []pattern, withTitle bool) (total int, ms []match) {
	for _, p := range pats {
		var m *match
		for _, pg := range ps {
			text := pg.text
			if withTitle {
				text += " " + pg.title
			}
			locs := p.re.FindAllStringIndex(text, -1)
			if len(locs) == 0 {
				continue
			}
			if m == nil {
				s := text[max(0, locs[0][0]-40):min(len(text), locs[0][1]+40)]
				m = &match{phrase: p.raw, url: pg.url, sample: "…" + strings.TrimSpace(s) + "…"}
			}
			m.count += len(locs)
		}
		if m != nil {
			ms = append(ms, *m)
			total += m.count
		}
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].count > ms[j].count })
	return total, ms
}

// evidence picks the strongest matches but makes sure every page that
// contributed is represented, so readers can see the whole site was read.
func evidence(ms []match, n int) []finding.Evidence {
	chosen := map[int]bool{}
	pages := map[string]bool{}
	for i, m := range ms {
		if !pages[m.url] {
			pages[m.url] = true
			chosen[i] = true
		}
	}
	for i := range ms {
		if len(chosen) >= n {
			break
		}
		chosen[i] = true
	}
	var ev []finding.Evidence
	for i, m := range ms {
		if chosen[i] {
			ev = append(ev, finding.Evidence{Location: finding.Location{URL: m.url}, Snippet: m.sample, Detail: fmt.Sprintf("%q ×%d", m.phrase, m.count)})
		}
	}
	return ev
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func website(ws *facts.WebSnapshot, tech *facts.Technologies) ([]finding.Finding, string) {
	ps := pagesOf(ws)
	words := 0
	for _, p := range ps {
		words += webtext.Words(p.text)
	}
	var out []finding.Finding

	n, ms := find(ps, rules["boilerplate"], false)
	density := float64(n) * 1000 / float64(max(words, 1))
	distinct := len(ms)
	if distinct >= 3 && density >= 5 {
		// Tiers: a sprinkle of stock phrases is weak evidence; many distinct
		// ones at extreme density is characteristic of unedited generated copy.
		sev, conf := finding.Low, finding.ConfidenceLow
		switch {
		case distinct >= 10 && density >= 30:
			sev, conf = finding.High, finding.ConfidenceMedium
		case distinct >= 6 && density >= 15:
			sev = finding.Medium
		}
		out = append(out, finding.Finding{
			Dimension: finding.DimAISignals, Category: "ai-boilerplate", Severity: sev, Confidence: conf,
			Title:                 fmt.Sprintf("Copy uses %d stock phrases typical of unedited AI output", distinct),
			Description:           fmt.Sprintf("%d occurrences of %d distinct boilerplate phrases in %d words across %d page(s) (%.1f per 1,000 words). Generic copy like this often means AI-drafted text went live without editing, and says little that is specific to the business.", n, distinct, words, len(ps), density),
			Evidence:              evidence(ms, 6),
			Rule:                  &finding.Rule{ID: "ai-boilerplate-density"},
			Remediation:           &finding.Remediation{Summary: "Rewrite key pages with specific, verifiable statements about the product, customers and results.", Automatable: false},
			FalsePositiveGuidance: "Human marketing copy uses these phrases too. Treat this as a prompt to review the text, not as proof of AI use.",
		})
	}
	pn, pm := find(ps, rules["placeholders"], false)
	if pn > 0 {
		out = append(out, finding.Finding{
			Dimension: finding.DimAISignals, Category: "placeholder-content", Severity: finding.High, Confidence: finding.ConfidenceHigh,
			Title:       "Placeholder content is live on the site",
			Description: fmt.Sprintf("%d placeholder fragment(s), such as lorem ipsum, 'Your Company' or dummy contact details, are visible to visitors.", pn),
			Evidence:    evidence(pm, 5),
			Rule:        &finding.Rule{ID: "placeholder-content"},
			Remediation: &finding.Remediation{Summary: "Replace every placeholder with real content before publishing.", Automatable: false},
		})
	}
	ln, lm := find(ps, rules["leftovers"], true)
	if ln > 0 {
		out = append(out, finding.Finding{
			Dimension: finding.DimAISignals, Category: "template-leftover", Severity: finding.Low, Confidence: finding.ConfidenceMedium,
			Title:       "Default template content left in place",
			Evidence:    evidence(lm, 3),
			Rule:        &finding.Rule{ID: "template-leftover"},
			Remediation: &finding.Remediation{Summary: "Remove the theme or builder's default pages and text.", Automatable: false},
		})
	}

	// AI site builders: from the page markup itself, plus web-tech if it ran.
	builders := map[string]string{} // name -> evidence
	for _, b := range builderRules {
		for _, p := range ps {
			if m := b.re.FindString(p.html); m != "" {
				builders[b.name] = p.url + " — " + truncate(m, 60)
				break
			}
		}
	}
	for _, t := range tech.Items {
		if _, seen := builders[t.Name]; !seen && aiBuilders[t.Name] {
			builders[t.Name] = strings.Join(t.Evidence, "; ")
		}
	}
	names := make([]string, 0, len(builders))
	for n := range builders {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		out = append(out, finding.Finding{
			Dimension: finding.DimAISignals, Category: "ai-builder", Severity: finding.Info, Confidence: finding.ConfidenceHigh,
			Title:       "Built with " + n,
			Description: fmt.Sprintf("%s generates or assists with site content and code. That is not a problem in itself; review generated pages for accuracy and maintainability.", n),
			Evidence:    []finding.Evidence{{Location: finding.Location{URL: ws.FinalURL}, Detail: builders[n]}},
			Component:   n,
			Rule:        &finding.Rule{ID: "ai-builder"},
		})
	}

	count := func(n int, what string) string {
		if n == 0 {
			return "no " + what
		}
		return fmt.Sprintf("%d %s", n, what)
	}
	builderPart := "no AI builder detected"
	if len(names) > 0 {
		builderPart = "built with " + strings.Join(names, ", ")
	}
	summary := fmt.Sprintf("Checked %d page%s, %d words: %d of %d stock phrases found (%.1f per 1,000 words); %s; %s; %s",
		len(ps), plural(len(ps), "", "s"), words, distinct, len(rules["boilerplate"]), density,
		count(pn, "placeholders"), count(ln, "template leftovers"), builderPart)
	return out, summary
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// swallowed matches error handlers that silently discard the error.
var swallowed = map[string]*regexp.Regexp{
	"brace":   regexp.MustCompile(`catch\s*(?:\([^)]*\))?\s*\{\s*\}`),
	"promise": regexp.MustCompile(`\.catch\(\s*(?:\(\s*\w*\s*\)|\w+)\s*=>\s*(?:\{\s*\}|null|undefined)\s*\)`),
	"python":  regexp.MustCompile(`(?m)^[ \t]*except[^:\n]*:[ \t]*(?:\n[ \t]*)?pass[ \t]*$`),
	"go":      regexp.MustCompile(`if\s+err\s*!=\s*nil\s*\{\s*\}`),
}

var swallowedFor = map[string][]string{
	"JavaScript": {"brace", "promise"}, "TypeScript": {"brace", "promise"}, "Vue": {"brace", "promise"}, "Svelte": {"brace", "promise"},
	"Java": {"brace"}, "Kotlin": {"brace"}, "C#": {"brace"}, "PHP": {"brace"}, "Dart": {"brace"}, "Swift": {"brace"}, "Scala": {"brace"},
	"Python": {"python"}, "Go": {"go"},
}

// isComment reports whether a line is (part of) a comment.
func isComment(line string) bool {
	t := strings.TrimSpace(line)
	for _, p := range []string{"//", "*", "/*", "#", "<!--", "--", ";"} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// densitySeverity grades a count by its density in shipping source: four
// scaffolding comments in 25 lines is far worse than four in 50,000.
func densitySeverity(count, lines int) finding.Severity {
	perK := float64(count) * 1000 / float64(max(lines, 1))
	switch {
	case count >= 10 || count >= 3 && perK >= 5:
		return finding.High
	case count >= 3 || perK >= 1:
		return finding.Medium
	}
	return finding.Low
}

// shipping reports whether a file is production code or configuration (not
// tests, examples, docs or example env files).
func shipping(f facts.File) bool {
	if facts.PathContext(f.Path) != facts.ContextProduction || f.Kind == facts.KindTest || f.Kind == facts.KindDocs {
		return false
	}
	base := strings.ToLower(path.Base(f.Path))
	return !strings.Contains(base, "example") && !strings.Contains(base, "sample") && !strings.Contains(base, "template")
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
	var scaffold, swallowEv, placeholderEv []finding.Evidence
	scaffoldCount, swallowCount, placeholderCount, shippingLines := 0, 0, 0, 0
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
		if (f.Kind == facts.KindConfig || f.Kind == facts.KindSource) && shipping(f) && f.Size <= fsutil.DefaultMaxRead {
			if b, _, err := fsutil.ReadFile(root, f.Path, fsutil.DefaultMaxRead); err == nil {
				fsutil.Lines(b, func(n int, line string) bool {
					if isComment(line) {
						return true // documentation examples are not configuration
					}
					for _, p := range rules["placeholder-config"] {
						if p.re.MatchString(line) {
							placeholderCount++
							if len(placeholderEv) < 10 {
								placeholderEv = append(placeholderEv, finding.Evidence{Location: finding.Location{Path: f.Path, StartLine: n}, Snippet: truncate(strings.TrimSpace(line), 120)})
							}
							break
						}
					}
					return true
				})
			}
		}
		if f.Kind != facts.KindSource || f.Size > fsutil.DefaultMaxRead {
			continue
		}
		b, _, err := fsutil.ReadFile(root, f.Path, fsutil.DefaultMaxRead)
		if err != nil {
			continue
		}
		if shipping(f) {
			shippingLines += f.Lines
			for _, key := range swallowedFor[f.Language] {
				for _, loc := range swallowed[key].FindAllIndex(b, -1) {
					swallowCount++
					if len(swallowEv) < 10 {
						line := 1 + strings.Count(string(b[:loc[0]]), "\n")
						swallowEv = append(swallowEv, finding.Evidence{Location: finding.Location{Path: f.Path, StartLine: line}, Snippet: truncate(strings.Join(strings.Fields(string(b[loc[0]:loc[1]])), " "), 120)})
					}
				}
			}
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
		sev := densitySeverity(scaffoldCount, shippingLines)
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
	if swallowCount > 0 {
		sev := densitySeverity(swallowCount, shippingLines)
		out = append(out, finding.Finding{
			Dimension: finding.DimMaintainability, Category: "swallowed-errors", Severity: sev, Confidence: finding.ConfidenceMedium,
			Title:                 fmt.Sprintf("%d error handler(s) silently discard errors", swallowCount),
			Description:           "Empty catch/except blocks and ignored error checks hide failures: the program continues in a broken state and nobody is told. Generated code often adds them to make examples run.",
			Evidence:              swallowEv,
			Rule:                  &finding.Rule{ID: "swallowed-errors"},
			Remediation:           &finding.Remediation{Summary: "Handle each error (retry, return it, or report it to the user) and at least log it with context.", Automatable: false},
			FalsePositiveGuidance: "Deliberately ignored errors should carry a comment explaining why; this check cannot see intent.",
		})
	}
	if placeholderCount > 0 {
		out = append(out, finding.Finding{
			Dimension: finding.DimAISignals, Category: "placeholder-config", Severity: densitySeverity(placeholderCount, shippingLines), Confidence: finding.ConfidenceHigh,
			Title:       fmt.Sprintf("%d placeholder value(s) left in code or configuration", placeholderCount),
			Description: "Values such as \"YOUR_API_KEY_HERE\", \"changeme\" or api.example.com in shipping code mean configuration was never completed, or a default credential is live.",
			Evidence:    placeholderEv,
			Rule:        &finding.Rule{ID: "placeholder-config"},
			Remediation: &finding.Remediation{Summary: "Replace placeholders with real configuration loaded from the environment or a secret manager; never ship default credentials.", Automatable: false},
		})
	}
	return out, fmt.Sprintf("%d AI assistant config(s), %d builder marker(s), %d scaffolding comment(s), %d swallowed error(s), %d placeholder value(s)", len(assistants), len(builders), scaffoldCount, swallowCount, placeholderCount), nil
}
