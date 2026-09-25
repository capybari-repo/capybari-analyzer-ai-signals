package aisignals

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/capybari-repo/capybari-core/facts"
	"github.com/capybari-repo/capybari-core/webtext"
)

// Build Depth credits signs of effort, read only from the pages already
// fetched. Groups and their maximum points (total 100):
//
//	breadth 15, specificity 20, originality 20, finish 20, trust 10, craft 15
//
// Documented in capybari-docs/methodology/scoring.md#build-depth.

// contentPageWords is the visible text a page needs to count as a content
// page rather than a stub.
const contentPageWords = 80

var (
	reNumber     = regexp.MustCompile(`[£$€¥]\s?\d[\d,.]*|\b\d[\d,.]*\s?(?:%|percent|minutes?|hours?|days?|weeks?|months?|years?|km|miles?|kg)\b|\b\d{2,}[\d,.]*\b`)
	reProper     = regexp.MustCompile(`[a-z0-9,;] ([A-Z][a-z]{2,}(?: [A-Z][a-z]{2,})*)`)
	reTitle      = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reMetaDesc   = regexp.MustCompile(`(?i)<meta[^>]+name="description"[^>]+content="([^"]{20,})"|<meta[^>]+content="([^"]{20,})"[^>]+name="description"`)
	reOGTitle    = regexp.MustCompile(`(?i)<meta[^>]+property="og:title"`)
	reOGImage    = regexp.MustCompile(`(?i)<meta[^>]+property="og:image"`)
	reIcon       = regexp.MustCompile(`(?i)<link[^>]+rel="(?:shortcut )?icon"[^>]*>`)
	reLang       = regexp.MustCompile(`(?i)<html[^>]+lang="[a-z]{2}`)
	reViewport   = regexp.MustCompile(`(?i)<meta[^>]+name="viewport"`)
	reCanonical  = regexp.MustCompile(`(?i)<link[^>]+rel="canonical"`)
	reAnchor     = regexp.MustCompile(`(?is)<a\b[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	reJSONLD     = regexp.MustCompile(`(?i)<script[^>]+type="application/ld\+json"`)
	reHreflang   = regexp.MustCompile(`(?i)<link[^>]+hreflang="`)
	reManifest   = regexp.MustCompile(`(?i)<link[^>]+rel="manifest"`)
	reImg        = regexp.MustCompile(`(?i)<img\b[^>]*>`)
	reAlt        = regexp.MustCompile(`(?i)\balt="[^"]+"`)
	reInput      = regexp.MustCompile(`(?i)<(?:input|select|textarea)\b[^>]*>`)
	reInputID    = regexp.MustCompile(`(?i)\bid="([^"]+)"`)
	reHidden     = regexp.MustCompile(`(?i)type="(?:hidden|submit|button)"`)
	reAria       = regexp.MustCompile(`(?i)aria-label(?:ledby)?="`)
	reLabelFor   = regexp.MustCompile(`(?i)<label[^>]+for="([^"]+)"`)
	reLandmarks  = regexp.MustCompile(`(?i)<(?:main|nav|header|footer)\b`)
	rePrivacy    = regexp.MustCompile(`(?i)privacy|terms|legal|imprint|impressum|cookie policy|datenschutz|gizlilik|kvkk`)
	reAbout      = regexp.MustCompile(`(?i)\babout\b|\bteam\b|our story|who we are|hakk[ıi]m[ıi]zda|über uns`)
	reContact    = regexp.MustCompile(`(?i)mailto:[^"]+|tel:\+?\d[\d \-]{5,}`)
	reStopProper = regexp.MustCompile(`^(?:The|This|That|These|Our|Your|We|You|It|And|But|Or|For|With|Get|Start|Learn|Read|Try|See|Join|Contact|About|Home|Pricing|Privacy|Terms|Features|Blog|Sign|Log|Book|Free|New|All|More|Why|How|What|When|Where|Who)$`)
)

type depthBuilder struct {
	d facts.SiteDepth
}

func (b *depthBuilder) add(id, group, name string, earned, maxPts float64, detail string) {
	earned = min(maxPts, max(0, earned))
	b.d.Checks = append(b.d.Checks, facts.DepthCheck{ID: id, Group: group, Name: name, Earned: float64(int(earned*10+0.5)) / 10, Max: maxPts, Detail: detail})
}

func yes(ok bool, pts float64) float64 {
	if ok {
		return pts
	}
	return 0
}

// siteDepth measures effort across the fetched pages. density is the stock
// phrase rate per 1,000 words; copyJudged is false when there was too little
// text to judge it; unfinished counts placeholders, leftovers and defaults.
func siteDepth(ws *facts.WebSnapshot, ps []page, words int, density float64, copyJudged bool, unfinished int) *facts.SiteDepth {
	b := &depthBuilder{d: facts.SiteDepth{Pages: len(ps), Words: words}}
	all := ""
	for _, p := range ps {
		all += p.html + "\n"
	}
	front := ps[0].html

	// Breadth: several pages with real content, each with its own title.
	content, titles := 0, map[string]bool{}
	for _, p := range ps {
		if webtext.Words(p.text) >= contentPageWords {
			content++
		}
		if t := strings.TrimSpace(p.title); t != "" {
			titles[strings.ToLower(t)] = true
		}
	}
	b.add("content-pages", "breadth", "content pages", []float64{0, 3, 8, 12, 15}[min(content, 4)], 15, fmt.Sprintf("%d of %d page(s) with %d+ words", content, len(ps), contentPageWords))

	// Specificity: concrete figures and named people, places and products.
	text := ""
	for _, p := range ps {
		text += p.text + "\n"
	}
	nums := map[string]bool{}
	for _, m := range reNumber.FindAllString(text, -1) {
		nums[strings.TrimSpace(m)] = true
	}
	perK := float64(len(nums)) * 1000 / float64(max(words, 1))
	b.add("figures", "specificity", "concrete figures", float64(min(len(nums), 12))*min(1, perK/8), 12, fmt.Sprintf("%d distinct (prices, counts, durations)", len(nums)))
	names := map[string]bool{}
	for _, m := range reProper.FindAllStringSubmatch(text, -1) {
		if !reStopProper.MatchString(strings.Fields(m[1])[0]) {
			names[m[1]] = true
		}
	}
	b.add("named-specifics", "specificity", "named specifics", float64(len(names))/2, 8, fmt.Sprintf("%d distinct names of people, places or products", len(names)))

	// Originality: copy written for this site, nothing left unfinished.
	switch {
	case !copyJudged:
		b.add("own-words", "originality", "own wording", 0, 12, "too little text to judge")
	case density < 3:
		b.add("own-words", "originality", "own wording", 12, 12, fmt.Sprintf("%.1f stock phrases per 1,000 words", density))
	case density < 8:
		b.add("own-words", "originality", "own wording", 8, 12, fmt.Sprintf("%.1f stock phrases per 1,000 words", density))
	case density < 15:
		b.add("own-words", "originality", "own wording", 4, 12, fmt.Sprintf("%.1f stock phrases per 1,000 words", density))
	default:
		b.add("own-words", "originality", "own wording", 0, 12, fmt.Sprintf("%.1f stock phrases per 1,000 words", density))
	}
	b.add("nothing-unfinished", "originality", "no placeholders or defaults", yes(unfinished == 0, 8), 8, fmt.Sprintf("%d found", unfinished))

	// Finishing touches: what browsers, search engines and link previews show.
	defaults := map[string]bool{}
	for _, m := range scaffoldDefaults(ps) {
		defaults[m.phrase] = true
	}
	titleOK := len(titles) > 0 && !defaults["default project title"] && (len(ps) == 1 || len(titles) > 1)
	b.add("title", "finish", "real page titles", yes(titleOK, 4), 4, "")
	b.add("description", "finish", "meta description", yes(reMetaDesc.MatchString(front) && !defaults["generator's default description"], 4), 4, "")
	og := yes(reOGTitle.MatchString(front), 2) + yes(reOGImage.MatchString(front) && !defaults["builder's default share image"], 2)
	b.add("share-preview", "finish", "link preview (og:title, og:image)", og, 4, "")
	icon := reIcon.FindString(front)
	b.add("favicon", "finish", "own favicon", yes(icon != "" && !defaults["default Vite favicon"], 3), 3, "")
	b.add("lang", "finish", "page language", yes(reLang.MatchString(front), 2), 2, "")
	b.add("viewport", "finish", "mobile viewport", yes(reViewport.MatchString(front), 2), 2, "")
	b.add("canonical", "finish", "canonical URL", yes(reCanonical.MatchString(front), 1), 1, "")

	// Trust pages: who runs the site and how to reach them.
	var privacy, about bool
	for _, m := range reAnchor.FindAllStringSubmatch(all, -1) {
		label := m[1] + " " + webtext.Visible(m[2])
		privacy = privacy || rePrivacy.MatchString(label)
		about = about || reAbout.MatchString(label)
	}
	contact := false
	for _, m := range reContact.FindAllString(all, -1) {
		if !strings.Contains(strings.ToLower(m), "example") {
			contact = true
		}
	}
	b.add("privacy", "trust", "privacy or terms page", yes(privacy, 4), 4, "")
	b.add("about", "trust", "about or team page", yes(about, 3), 3, "")
	b.add("contact", "trust", "real contact details", yes(contact, 3), 3, "")

	// Extra craft: things a few prompts rarely produce.
	b.add("structured-data", "craft", "structured data (JSON-LD)", yes(reJSONLD.MatchString(all), 4), 4, "")
	b.add("languages", "craft", "more than one language", yes(reHreflang.MatchString(all), 4), 4, "")
	b.add("manifest", "craft", "web app manifest", yes(reManifest.MatchString(all), 2), 2, "")
	b.add("accessibility", "craft", "accessible images and forms", yes(accessible(all), 3), 3, "")
	b.add("landmarks", "craft", "page landmarks (main, nav)", yes(len(reLandmarks.FindAllString(front, -1)) >= 2, 2), 2, "")
	return &b.d
}

// accessible reports whether every image has alt text and every visible form
// field has a label.
func accessible(html string) bool {
	for _, img := range reImg.FindAllString(html, -1) {
		if !reAlt.MatchString(img) {
			return false
		}
	}
	labelled := map[string]bool{}
	for _, m := range reLabelFor.FindAllStringSubmatch(html, -1) {
		labelled[m[1]] = true
	}
	for _, in := range reInput.FindAllString(html, -1) {
		if reHidden.MatchString(in) || reAria.MatchString(in) {
			continue
		}
		id := reInputID.FindStringSubmatch(in)
		if id == nil || !labelled[id[1]] {
			return false
		}
	}
	return true
}
