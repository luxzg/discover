package store

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The offline review calibrated headline evidence, not a catalog of products.
// Keep the live decision conservative: never substitute a topic or an outlet
// for a shared claim, and never consult the reader's clicks.
var claimWords = regexp.MustCompile("[[:alnum:]]+")
var claimWeak = wordsSet("a an and are as at be by for from has have how in into is it its new of on or out says that the their these this to what when why will with your after about now could just than more over up you gpu cpu cpus gpus graphics card chip chips company tech news models ai next gen desktop desktops prices report reports reportedly claims first time quote day launches launch benchmark benchmarks shares official release review latest reveals")
var modelCode = regexp.MustCompile(`\b[a-z]+[0-9]+[a-z0-9]*\b`)
var namedModel = regexp.MustCompile(`\b([a-z]+)[ -]?([0-9]+[a-z]{0,2})\b(\+?)`)
var variantCode = regexp.MustCompile(`\b([A-Za-z]{2,})[- ]([A-Z])\b`)
var measurement = regexp.MustCompile(`\b([0-9]+(?:\.[0-9]+)?)\+?\s*(ghz|mhz|tb|gb|watts|(?:[a-z]+ )?cores)\b`)
var thousands = regexp.MustCompile(`([0-9]),([0-9]{3})\b`)
var launchActor = regexp.MustCompile(`^([[:alpha:]][[:alnum:]-]*(?: [[:alpha:]][[:alnum:]-]*){0,2}) (?:launches|unveils|debuts|brings)\b`)
var systemScope = regexp.MustCompile(`\b(?:rack[- ]scale|rack systems?|servers? powered by|systems? powered by)\b`)
var clockNoun = regexp.MustCompile(`(?:[0-9]\s*[gm]hz\s+boost|boost (?:clock|frequency))\b`)
var riskWords = wordsSet("no not never without denies denied cancels cancelled")
var rumorWords = wordsSet("rumor rumors rumour reportedly allegedly unconfirmed")
var confirmedWords = wordsSet("confirms confirmed officially")
var coverageFamilies = []struct {
	name string
	re   *regexp.Regexp
}{
	{"availability", regexp.MustCompile(`\b(launch(es|ed)?|ships?|shipping|now available|brings? .{1,65} to)\b`)},
	{"delay", regexp.MustCompile(`\b(delays?|delayed|postponed|slips? past|pushed back)\b`)},
	{"improvement", regexp.MustCompile(`\b(improves?|improved|boost(s|ed)?|enhances?|enhanced)\b`)},
	{"preview", regexp.MustCompile(`\b(teases?|teased|teaser|previews?)\b`)},
	{"review", regexp.MustCompile(`\b(reviews?|reviewed|hands.on)\b`)},
	{"benchmark", regexp.MustCompile(`\bbenchmarks?\b`)},
	{"recall", regexp.MustCompile(`\brecalls?\b`)},
}

type coverageClaim struct {
	words                                                   []string
	strong, pairs, kinds, reviewIDs                         map[string]bool
	slots                                                   map[string]map[string]bool
	actor                                                   string
	neg, rumor, confirmed, system, overview, narrow, unsafe bool
}

func commonWords(a, b map[string]bool) int {
	n := 0
	for k := range a {
		if b[k] {
			n++
		}
	}
	return n
}

func parseCoverage(title string) coverageClaim {
	c := coverageClaim{strong: map[string]bool{}, pairs: map[string]bool{}, kinds: map[string]bool{}, reviewIDs: map[string]bool{}, slots: map[string]map[string]bool{}}
	if len(title) > 768 {
		c.unsafe = true
		return c
	}
	line := strings.ToLower(strings.ReplaceAll(title, "’", "'"))
	for thousands.MatchString(line) {
		line = thousands.ReplaceAllString(line, "${1}${2}")
	}
	c.words = claimWords.FindAllString(line, 64)
	for i, w := range c.words {
		if len(w) > 1 && !claimWeak[w] {
			c.strong[w] = true
		}
		if i > 0 {
			c.pairs[c.words[i-1]+" "+w] = true
		}
		c.neg = c.neg || riskWords[w]
		c.rumor = c.rumor || rumorWords[w]
		c.confirmed = c.confirmed || confirmedWords[w]
	}
	// Questions, directional transactions/prices and attributed daily quotations
	// need more context than bag-of-words evidence. The exact path still runs.
	c.unsafe = strings.ContainsAny(line, "?$€£¥") || strings.Contains(line, "quote of the day") || strings.Contains(line, " buys ") || strings.Contains(line, " acquires ") || strings.Contains(line, " sells ")
	for _, w := range c.words {
		if w == "will" || w == "might" || w == "would" || w == "plans" || w == "versus" {
			c.unsafe = true
		}
	}
	c.system = systemScope.MatchString(line)
	mobile, desktop := strings.Contains(line, "mobile"), strings.Contains(line, "desktop")
	c.overview = mobile && desktop
	c.narrow = mobile != desktop || strings.Contains(line, "engineering sample")
	if m := launchActor.FindStringSubmatch(line); m != nil {
		c.actor = m[1]
	}
	if lead, _, ok := strings.Cut(line, " review"); ok {
		for _, code := range modelCode.FindAllString(lead, -1) {
			c.reviewIDs[code] = true
		}
	}
	add := func(k, v string) {
		if c.slots[k] == nil {
			c.slots[k] = map[string]bool{}
		}
		c.slots[k][v] = true
	}
	for _, m := range namedModel.FindAllStringSubmatch(line, -1) {
		if !claimWeak[m[1]] {
			add("model:"+m[1], m[2]+m[3])
		}
	}
	for _, m := range variantCode.FindAllStringSubmatch(title, -1) {
		if m[2] != "I" && m[2] != "A" && !claimWeak[strings.ToLower(m[1])] {
			add("variant:"+strings.ToLower(m[1]), m[2])
		}
	}
	for _, m := range measurement.FindAllStringSubmatch(line, -1) {
		unit, v := m[2], m[1]
		if unit == "ghz" || unit == "mhz" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
			if unit == "ghz" {
				f *= 1000
			}
			unit = "clock"
			v = strconv.FormatFloat(f, 'f', 3, 64)
		}
		add("unit:"+unit, v)
	}
	for _, family := range coverageFamilies {
		s := line
		if family.name == "improvement" {
			s = clockNoun.ReplaceAllString(s, " ")
		}
		if family.re.MatchString(s) {
			c.kinds[family.name] = true
		}
	}
	return c
}

func (c coverageClaim) keys() []string {
	if c.unsafe || len(c.strong) < 3 {
		return nil
	}
	keys := make([]string, 0, len(c.strong))
	for w := range c.strong {
		keys = append(keys, w)
	}
	sort.Slice(keys, func(i, j int) bool { return betterEventBlock(keys[i], keys[j]) })
	if len(keys) > 10 {
		keys = keys[:10]
	}
	return keys
}

func relatedClaim(a, b coverageClaim) bool {
	if a.unsafe || b.unsafe || a.neg != b.neg || a.rumor && b.confirmed || b.rumor && a.confirmed {
		return false
	}
	if a.system != b.system || a.overview && b.narrow && !b.overview || b.overview && a.narrow && !a.overview {
		return false
	}
	if a.actor != "" && b.actor != "" && a.actor != b.actor || len(a.reviewIDs) > 0 && len(b.reviewIDs) > 0 && commonWords(a.reviewIDs, b.reviewIDs) == 0 {
		return false
	}
	quantity, variant := false, false
	for k, v := range a.slots {
		if w := b.slots[k]; w != nil {
			if commonWords(v, w) == 0 {
				return false
			}
			quantity = quantity || strings.HasPrefix(k, "unit:")
			variant = variant || strings.HasPrefix(k, "variant:")
		}
	}
	family := commonWords(a.kinds, b.kinds) > 0
	for _, angle := range []string{"review", "benchmark"} {
		if a.kinds[angle] != b.kinds[angle] {
			return false
		}
	}
	if !family && len(a.kinds) > 0 && len(b.kinds) > 0 {
		return false
	}
	n := commonWords(a.strong, b.strong)
	if n < 3 {
		return false
	}
	phrase := false
	for p := range a.pairs {
		if b.pairs[p] {
			x, y, _ := strings.Cut(p, " ")
			if a.strong[x] && a.strong[y] {
				phrase = true
				break
			}
		}
	}
	if family && (n >= 4 || phrase) || quantity && (phrase || variant) {
		return true
	}
	prefix := map[string]bool{}
	for i := 0; i < len(a.words) && i < len(b.words) && a.words[i] == b.words[i]; i++ {
		if a.strong[a.words[i]] {
			prefix[a.words[i]] = true
		}
		if i >= 7 && len(prefix) >= 3 {
			return true
		}
	}
	return n >= 5 && phrase && commonWords(a.pairs, b.pairs) >= 3 && 5*n >= 4*min(len(a.strong), len(b.strong))
}
