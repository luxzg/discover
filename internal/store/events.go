package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode"
)

// These describe events, not products or configured topics. Unknown events can
// still match on a distinctive ordered claim. Fail open to separate cards.
var eventPhrases = []struct{ kind, phrases string }{
	{"first deliveries", "starts shipping|start shipping|begins shipping|now shipping|is shipping|first deliveries of|shipments begin|shipping begins|units reach customers|units reach buyers|ships to customers|deliveries begin"},
	{"price cut", "price cut|price cuts|cuts price of|cuts the price of|reduces price of|reduces the price of|price drops|price drop"},
	{"release", "is released|now released|officially released|is now available|now available|launches|released"},
	{"announcement", "announces|unveils|introduces|announced|unveiled"},
	{"recall", "issues recall for|recalls|is recalled"},
	{"discovery", "researchers discover|scientists discover|researchers find|scientists find|discovered|discovery of"},
}

func wordsSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(s) {
		out[w] = true
	}
	return out
}

var filler = wordsSet("a an the is are its their this that officially")
var eventFiller = wordsSet("of to for first official new customers buyers units")
var generic = wordsSet("new latest technology tech device company product news update system chip cpu gpu processor laptop software hardware version release news price model")
var uncertain = wordsSet("no not never without instead may might could would will plans expected upcoming rumor rumors rumour leak leaks leaked allegedly reportedly next delayed delay vs versus compares comparison but despite and or")
var angles = wordsSet("review reviewed benchmark benchmarks tested testing test tests analysis why how problem problems flaw flaws issue issues")
var symbols = strings.NewReplacer("$", " dollar ", "€", " eur ", "£", " gbp ", "¥", " yen-yuan ", "%", " percent ", "−", " minus ", "+", " plus ")
var frontedContext = wordsSet("near beneath under during after before inside outside")

type eventClaim struct{ kind, block, subject string }

func hasDigit(s string) bool { return strings.IndexFunc(s, unicode.IsDigit) >= 0 }

func headlineClaim(title string) (eventClaim, bool) {
	if len(title) > 768 || strings.ContainsAny(title, "?;") {
		return eventClaim{}, false
	}
	// Preserve decimal/version and numeric punctuation as tokens rather than
	// collapsing 1.20 to 120. Never equate a question or a negated prediction.
	line := symbols.Replace(strings.ToLower(title))
	// "Beneath the ice, researchers observe X" and "Researchers observe X
	// beneath the ice" convey the same ordered claim. Move only a short,
	// comma-delimited context phrase; never sort actors, verbs or numbers.
	if prefix, rest, ok := strings.Cut(line, ","); ok {
		p := strings.Fields(prefix)
		if len(p) >= 2 && len(p) <= 5 && frontedContext[p[0]] && !strings.Contains(rest, ",") {
			line = strings.TrimSpace(rest) + " " + prefix
		}
	}
	words := strings.FieldsFunc(line, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for _, w := range words {
		if uncertain[w] || w == "t" {
			return eventClaim{}, false
		}
	}
	line = " " + strings.Join(words, " ") + " "
	// Normalize a few general language equivalents, never product names.
	for _, alias := range claimAliases {
		line = strings.ReplaceAll(line, alias.from, alias.to)
	}
	kind := "shared headline claim"
	for _, family := range eventPhrases {
		for _, phrase := range strings.Split(family.phrases, "|") {
			p := " " + phrase + " "
			if strings.Contains(line, p) {
				if kind != "shared headline claim" && kind != family.kind {
					return eventClaim{}, false
				}
				kind = family.kind
				line = strings.ReplaceAll(line, p, " ")
			}
		}
	}
	var tokens []string
	block := ""
	distinct := map[string]bool{}
	for _, w := range strings.Fields(line) {
		if kind != "shared headline claim" && angles[w] {
			return eventClaim{}, false
		}
		if filler[w] || (kind != "shared headline claim" && eventFiller[w]) {
			continue
		}
		tokens = append(tokens, w)
		if generic[w] || len(w) < 2 {
			continue
		}
		distinct[w] = true
		if betterEventBlock(w, block) {
			block = w
		}
	}
	if len(distinct) < 2 || (kind == "shared headline claim" && (len(tokens) < 5 || len(distinct) < 4)) {
		return eventClaim{}, false
	}
	// Preserve word order: swapping buyer/seller or two prices changes a claim.
	return eventClaim{kind, block, strings.Join(tokens, " ")}, true
}

func betterEventBlock(word, current string) bool {
	if current == "" {
		return true
	}
	if hasDigit(word) != hasDigit(current) {
		return hasDigit(word)
	}
	if len(word) != len(current) {
		return len(word) > len(current)
	}
	return word < current
}

func similarClaim(a, b string) bool {
	if a == b {
		return true
	}
	x, y := strings.Fields(a), strings.Fields(b)
	if len(x) > len(y) {
		x, y = y, x
	}
	if len(x) < 7 || len(y) != len(x)+1 {
		return false
	}
	// A single extra descriptive word, in an otherwise identical long claim.
	// Do not ignore an extra number, another model/edition, or an event angle.
	i := 0
	for _, w := range y {
		if i < len(x) && w == x[i] {
			i++
			continue
		}
		if !softModifier[w] {
			return false
		}
	}
	return i == len(x)
}

var softModifier = wordsSet("latest updated newly official major detailed public")

var claimAliases = []struct{ from, to string }{
	{" graphics card ", " gpu "}, {" graphics processor ", " gpu "},
	{" under the ", " beneath the "}, {" under ", " beneath "},
	{" production begins ", " enters production "}, {" mass production starts ", " enters mass production "},
}

const eventSpan = int64(36 * time.Hour / time.Second)

func indexEvent(ctx context.Context, tx *sql.Tx, id int64, title string, stamp time.Time) error {
	var group any
	if claim, ok := headlineClaim(title); ok && !stamp.IsZero() {
		t := stamp.Unix()
		rows, err := tx.QueryContext(ctx, `SELECT id,subject FROM coverage_events
 WHERE kind=? AND block=? AND anchor_time BETWEEN ? AND ? ORDER BY id LIMIT 33`, claim.kind, claim.block, t-eventSpan, t+eventSpan)
		if err != nil {
			return err
		}
		var matches []int64
		n := 0
		for rows.Next() {
			var candidate int64
			var subject string
			if err := rows.Scan(&candidate, &subject); err != nil {
				rows.Close()
				return err
			}
			n++
			if similarClaim(claim.subject, subject) {
				matches = append(matches, candidate)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if n <= 32 {
			switch len(matches) {
			case 0:
				res, err := tx.ExecContext(ctx, `INSERT INTO coverage_events(kind,block,subject,anchor_time) VALUES(?,?,?,?)`, claim.kind, claim.block, claim.subject, t)
				if err != nil {
					return err
				}
				group, err = res.LastInsertId()
				if err != nil {
					return err
				}
			case 1:
				group = matches[0]
			}
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE articles SET event_id=? WHERE id=? AND event_id IS NOT ?`, group, id, group)
	return err
}

func indexStoredEvent(ctx context.Context, tx *sql.Tx, id int64) error {
	var title string
	var stamp any
	if err := tx.QueryRowContext(ctx, `SELECT title,COALESCE(published_at,first_seen_at,created_at) FROM articles WHERE id=?`, id).Scan(&title, &stamp); err != nil {
		return err
	}
	return indexEvent(ctx, tx, id, title, parseDBTime(stamp))
}

// Keyset batches avoid holding the single SQLite writer over the entire
// upgrade. Only recent, potentially displayable history needs trial indexing.
func (s *Store) prepareEvents(ctx context.Context) error {
	done, err := s.GetSettingInt(ctx, "event_index_v1", 0)
	if err != nil || done == 1 {
		return err
	}
	cursor, err := s.GetSettingInt(ctx, "event_index_v1_after", 0)
	if err != nil {
		return err
	}
	after := int64(cursor)
	for {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		last, more, err := indexEventBatch(ctx, tx, after)
		if err != nil {
			tx.Rollback()
			return err
		}
		key, value := "event_index_v1_after", last
		if !more {
			key, value = "event_index_v1", 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO app_settings(key,value) VALUES(?,CAST(? AS TEXT))
 ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		if !more {
			return nil
		}
		after = last
	}
}

func indexEventBatch(ctx context.Context, tx *sql.Tx, after int64) (int64, bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,title,COALESCE(published_at,first_seen_at,created_at) FROM articles
 WHERE id>? AND (status='unread' OR hidden_reason='duplicate') AND julianday(COALESCE(published_at,first_seen_at,created_at))>=julianday('now','-45 days')
 ORDER BY id LIMIT 256`, after)
	if err != nil {
		return 0, false, err
	}
	type entry struct {
		id    int64
		title string
		stamp any
	}
	var batch []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.title, &e.stamp); err != nil {
			rows.Close()
			return 0, false, err
		}
		batch = append(batch, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, false, err
	}
	for _, e := range batch {
		if err := indexEvent(ctx, tx, e.id, e.title, parseDBTime(e.stamp)); err != nil {
			return 0, false, err
		}
		after = e.id
	}
	return after, len(batch) == 256, nil
}
