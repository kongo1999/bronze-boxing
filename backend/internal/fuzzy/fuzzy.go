// Package fuzzy is the one matcher behind every search in the app: lists,
// pickers and global search rank with the same rules, mirrored exactly by
// frontend/src/lib/fuzzy.ts and checked against the shared cases in
// docs/fixtures/fuzzy-cases.json.
//
// Rules: normalize case, accents, punctuation, spacing (and Arabic letter
// variants); rank exact > prefix > word prefix > substring > typo; allow a
// one-letter typo in short words and two in long ones; words may come in
// any order; a few aliases (PT = private, no show = no-show); digits in the
// query also match phone numbers and references. Every query word must
// match something, so unrelated records never appear.
package fuzzy

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Score tiers.
const (
	TierExact  = 1000 // the whole primary text equals the query
	TierPrefix = 900  // the primary text starts with the query (+50 when its first word is the query's first word)
	// Word-by-word matches score 300 + 4 × the average word score (≤ 700),
	// +10 when the words appear in the query's order.
	MinScore = 1 // any positive score is a match
)

// Word scores.
const (
	wordExact     = 100
	wordPrefix    = 85
	wordDigits    = 70
	wordSubstring = 60
	wordTypo      = 45 // −5 per extra edit
	wordTypoStart = 40 // typo within the start of a longer word
)

// joins turns common two-word spellings into one token, so "no show",
// "no-show" and "noshow" are the same word.
var joins = map[string]string{
	"no show": "noshow",
	"t shirt": "tshirt",
	"drop in": "dropin",
}

// aliases are interchangeable words.
var aliases = map[string][]string{
	"pt":           {"private"},
	"private":      {"pt"},
	"sub":          {"subscription"},
	"subscription": {"sub"},
	"tee":          {"tshirt"},
	"tshirt":       {"tee"},
}

// Normalize lowercases, strips accents and Arabic diacritics, folds Arabic
// letter variants, and turns anything that isn't a letter or digit into a
// single space.
func Normalize(s string) string {
	s = norm.NFD.String(s)
	var b strings.Builder
	b.Grow(len(s))
	space := true
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r), r == 'ـ':
			continue
		case r == 'ة':
			r = 'ه'
		case r == 'ى':
			r = 'ي'
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteString(strings.ToLower(string(r)))
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Tokens splits normalized text into words, joining the known two-word
// spellings.
func Tokens(normalized string) []string {
	if normalized == "" {
		return nil
	}
	raw := strings.Split(normalized, " ")
	out := make([]string, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if i+1 < len(raw) {
			if j, ok := joins[raw[i]+" "+raw[i+1]]; ok {
				out = append(out, j)
				i++
				continue
			}
		}
		out = append(out, raw[i])
	}
	return out
}

// Digits keeps only the ASCII digits of s.
func Digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Query is a prepared search.
type Query struct {
	Norm  string
	Words []string
	alts  [][]string
}

func NewQuery(q string) Query {
	n := Normalize(q)
	w := Tokens(n)
	alts := make([][]string, len(w))
	for i, t := range w {
		alts[i] = append([]string{t}, aliases[t]...)
	}
	return Query{Norm: strings.Join(w, " "), Words: w, alts: alts}
}

// Empty reports whether there is nothing to search for.
func (q Query) Empty() bool { return len(q.Words) == 0 }

// Target is a prepared record: its main text (a name or title) and any
// other searchable fields.
type Target struct {
	Primary string   // normalized, words joined by single spaces
	first   string   // first word of Primary
	words   []string // words of every field, Primary first
	digits  []string // digit runs per field
}

func NewTarget(primary string, others ...string) Target {
	pw := Tokens(Normalize(primary))
	t := Target{Primary: strings.Join(pw, " "), words: append([]string{}, pw...)}
	if len(pw) > 0 {
		t.first = pw[0]
	}
	if d := Digits(primary); d != "" {
		t.digits = append(t.digits, d)
	}
	for _, o := range others {
		t.words = append(t.words, Tokens(Normalize(o))...)
		if d := Digits(o); d != "" {
			t.digits = append(t.digits, d)
		}
	}
	return t
}

// Result says how well a target matched.
type Result struct {
	Score int
	// Typo is true when some word only matched with a typo.
	Typo bool
	// Corrected is the query with each typo'd word replaced by the word it
	// matched — the "Did you mean…" text.
	Corrected string
}

// Match scores a target against a query; Score 0 means no match.
func Match(q Query, t Target) Result {
	if q.Empty() || len(t.words) == 0 && len(t.digits) == 0 {
		return Result{}
	}
	best := Result{}
	if t.Primary == q.Norm {
		return Result{Score: TierExact}
	}
	if strings.HasPrefix(t.Primary, q.Norm) {
		s := TierPrefix
		if t.first == q.Words[0] {
			s += 50
		}
		best = Result{Score: s}
	}
	sum, lastPos, ordered, typo := 0, -1, true, false
	corrected := make([]string, len(q.Words))
	for i, alts := range q.alts {
		ws, pos, via, isTypo := 0, -1, "", false
		for _, a := range alts {
			for j, w := range t.words {
				s, ty := wordScore(a, w)
				if s > ws {
					ws, pos, via, isTypo = s, j, w, ty
				}
			}
			if len([]rune(a)) >= 3 && isDigits(a) {
				for _, d := range t.digits {
					if strings.Contains(d, a) && wordDigits > ws {
						ws, pos, via, isTypo = wordDigits, -1, a, false
					}
				}
			}
		}
		if ws == 0 {
			return best
		}
		sum += ws
		if pos >= 0 {
			if pos <= lastPos {
				ordered = false
			}
			lastPos = pos
		}
		if isTypo {
			typo = true
			corrected[i] = via
		} else {
			corrected[i] = q.Words[i]
		}
	}
	s := 300 + 4*sum/len(q.Words)
	if ordered && len(q.Words) > 1 {
		s += 10
	}
	if s > best.Score {
		best = Result{Score: s, Typo: typo}
		if typo {
			best.Corrected = strings.Join(corrected, " ")
		}
	}
	return best
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// wordScore compares one query word with one target word.
func wordScore(q, w string) (int, bool) {
	if q == w {
		return wordExact, false
	}
	if strings.HasPrefix(w, q) {
		return wordPrefix, false
	}
	qr, wr := []rune(q), []rune(w)
	if len(qr) >= 3 && strings.Contains(w, q) {
		return wordSubstring, false
	}
	if isDigits(q) {
		return 0, false // numbers match exactly or not at all: 5556 is not 5557
	}
	maxd := maxEdits(qr)
	if maxd == 0 || (len(qr) == 3 && qr[0] != wr[0]) {
		return 0, false
	}
	if d := osa(qr, wr, maxd); d <= maxd {
		return wordTypo - 5*(d-1), true
	}
	if len(qr) >= 4 && len(wr) > len(qr) {
		if d := osa(qr, wr[:len(qr)], maxd); d <= maxd {
			return wordTypoStart - 5*(d-1), true
		}
	}
	return 0, false
}

// maxEdits is the typo budget for a query word: none under three letters,
// one up to seven (a three-letter word must still start right), two beyond.
func maxEdits(q []rune) int {
	switch n := len(q); {
	case n < 3:
		return 0
	case n <= 7:
		return 1
	default:
		return 2
	}
}

// osa is the optimal-string-alignment edit distance (insert, delete,
// substitute, swap adjacent), giving up early past max.
func osa(a, b []rune, max int) int {
	if d := len(a) - len(b); d > max || -d > max {
		return max + 1
	}
	prev2 := make([]int, len(b)+1)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				v = min(v, prev2[j-2]+1)
			}
			cur[j] = v
			rowMin = min(rowMin, v)
		}
		if rowMin > max {
			return max + 1
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(b)]
}

// Rank returns the indexes of the targets that match, best first: by score,
// then shorter main text, then main text in code-point order, then input
// order — the same tie-breaks as the frontend.
func Rank(q Query, targets []Target) []Ranked {
	out := []Ranked{}
	for i, t := range targets {
		if r := Match(q, t); r.Score >= MinScore {
			out = append(out, Ranked{Index: i, Result: r, primary: t.Primary})
		}
	}
	sortRanked(out)
	return out
}

// Ranked is one match from Rank.
type Ranked struct {
	Index int
	Result
	primary string
}

func sortRanked(r []Ranked) {
	sort.SliceStable(r, func(i, j int) bool { return Less(r[i].Score, r[i].primary, r[j].Score, r[j].primary) })
}

// Less orders two matches: higher score, then shorter, then code-point order.
func Less(scoreA int, primaryA string, scoreB int, primaryB string) bool {
	if scoreA != scoreB {
		return scoreA > scoreB
	}
	if la, lb := utf8.RuneCountInString(primaryA), utf8.RuneCountInString(primaryB); la != lb {
		return la < lb
	}
	return primaryA < primaryB
}
