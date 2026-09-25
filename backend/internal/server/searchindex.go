package server

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/fuzzy"
	"bronzeboxing/internal/models"
)

// The search index: every searchable record's normalized text, held in
// memory and ranked with the shared fuzzy matcher. It is built from the
// database on first use and then kept current from a MongoDB change stream,
// drained before every search — so any write, from any code path (or a
// migration, or a manual fix in the shell), is reflected in the very next
// search, and no write path has to remember to update it. Without a replica
// set (dev-only degraded mode) there is no change stream; the index is
// rebuilt when older than a couple of seconds instead.

const (
	kindTrainee  = "trainee"
	kindSession  = "session"
	kindPayment  = "payment"
	kindSale     = "sale"
	kindItem     = "item"
	kindExpense  = "expense"
	kindReminder = "reminder"
	kindMovement = "movement" // item history search; omitted from global groups
)

// collSearchSync holds throwaway markers: a search writes one and reads the
// change stream up to it, which proves every write acknowledged before the
// search began has been applied to the index.
const collSearchSync = "search_sync"

// searchKinds is the order groups appear in global search.
var searchKinds = []string{kindTrainee, kindSession, kindPayment, kindSale, kindItem, kindExpense, kindReminder}

var kindOfColl = map[string]string{
	models.CollTrainees:  kindTrainee,
	models.CollSessions:  kindSession,
	models.CollPayments:  kindPayment,
	models.CollSales:     kindSale,
	models.CollInventory: kindItem,
	models.CollExpenses:  kindExpense,
	models.CollReminders: kindReminder,
	models.CollMovements: kindMovement,
}

// searchHit is one result as the API returns it.
type searchHit struct {
	Kind   string    `json:"kind"`
	ID     string    `json:"id"`
	Label  string    `json:"label"`
	Sub    string    `json:"sub,omitempty"`
	Date   time.Time `json:"date,omitempty"`
	Amount *float64  `json:"amount,omitempty"`
	// Flag marks records that are kept but not current: void, archived,
	// inactive, cancelled, done.
	Flag  string `json:"flag,omitempty"`
	Score int    `json:"score"`
	Typo  bool   `json:"typo,omitempty"`
	// Month / Trainee help the client open the right filtered page.
	Month   string `json:"month,omitempty"`
	Trainee string `json:"trainee,omitempty"`
}

type searchEntry struct {
	hit    searchHit
	target fuzzy.Target
}

type searchIndex struct {
	store    *db.Store
	mu       sync.Mutex
	entries  map[string]map[primitive.ObjectID]*searchEntry
	postings map[string]map[string]map[primitive.ObjectID]struct{}
	cs       *mongo.ChangeStream
	built    time.Time
}

func newSearchIndex(store *db.Store) *searchIndex {
	return &searchIndex{store: store}
}

// sync brings the index up to date with every committed write.
func (x *searchIndex) sync(ctx context.Context) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if !x.store.SupportsTx {
		if x.entries == nil || time.Since(x.built) > 2*time.Second {
			return x.rebuild(ctx)
		}
		return nil
	}
	if x.cs == nil {
		return x.rebuildAndWatch(ctx)
	}
	// The change stream can trail an acknowledged write by a moment, so
	// write a marker and read up to it: everything committed before it is
	// then in the index.
	marker := primitive.NewObjectID()
	if _, err := x.store.Coll(collSearchSync).InsertOne(ctx, bson.M{"_id": marker, "at": time.Now()}); err != nil {
		return err
	}
	defer func() { _, _ = x.store.Coll(collSearchSync).DeleteOne(context.Background(), bson.M{"_id": marker}) }()
	wait, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for x.cs.Next(wait) {
		var key struct {
			NS struct {
				Coll string `bson:"coll"`
			} `bson:"ns"`
			Key struct {
				ID any `bson:"_id"`
			} `bson:"documentKey"`
		}
		if bson.Unmarshal(x.cs.Current, &key) == nil && key.NS.Coll == collSearchSync {
			if id, ok := key.Key.ID.(primitive.ObjectID); ok && id == marker {
				return nil
			}
			continue
		}
		if !x.apply(x.cs.Current) {
			return x.restart(ctx)
		}
	}
	// The stream broke (or the marker never came): start over from a fresh
	// read, which is correct by construction.
	return x.restart(ctx)
}

func (x *searchIndex) restart(ctx context.Context) error {
	if x.cs != nil {
		_ = x.cs.Close(ctx)
		x.cs = nil
	}
	return x.rebuildAndWatch(ctx)
}

// rebuildAndWatch opens the change stream first and then reads everything:
// a write racing the scan is either in the scan, in the stream, or both —
// applying it twice is harmless.
func (x *searchIndex) rebuildAndWatch(ctx context.Context) error {
	colls := []string{collSearchSync}
	for c := range kindOfColl {
		colls = append(colls, c)
	}
	cs, err := x.store.DB.Watch(ctx,
		mongo.Pipeline{{{Key: "$match", Value: bson.M{"ns.coll": bson.M{"$in": colls}}}}},
		options.ChangeStream().SetFullDocument(options.UpdateLookup).SetMaxAwaitTime(100*time.Millisecond))
	if err != nil {
		return err
	}
	if err := x.rebuild(ctx); err != nil {
		_ = cs.Close(ctx)
		return err
	}
	x.cs = cs
	return nil
}

func (x *searchIndex) rebuild(ctx context.Context) error {
	entries := map[string]map[primitive.ObjectID]*searchEntry{}
	for coll, kind := range kindOfColl {
		entries[kind] = map[primitive.ObjectID]*searchEntry{}
		cur, err := x.store.Coll(coll).Find(ctx, bson.M{})
		if err != nil {
			return err
		}
		for cur.Next(ctx) {
			if e := buildEntry(kind, cur.Current); e != nil {
				id, _ := primitive.ObjectIDFromHex(e.hit.ID)
				entries[kind][id] = e
			}
		}
		if err := cur.Err(); err != nil {
			_ = cur.Close(ctx)
			return err
		}
		_ = cur.Close(ctx)
	}
	x.entries = entries
	x.postings = map[string]map[string]map[primitive.ObjectID]struct{}{}
	for kind, group := range entries {
		for id, entry := range group {
			x.addPostings(kind, id, entry)
		}
	}
	x.built = time.Now()
	return nil
}

// The posting lists narrow typo-tolerant ranking to plausible records. A
// matching word shares a character pair with the query except for short
// typo cases, which use their first character. Final acceptance and
// ordering still use fuzzy.Match, so search semantics do not change.
func searchKeys(word string) []string {
	r := []rune(word)
	if len(r) == 0 {
		return nil
	}
	keys := []string{"first:" + string(r[0])}
	for i := 0; i+1 < len(r); i++ {
		keys = append(keys, "pair:"+string(r[i:i+2]))
	}
	return keys
}

func queryKeys(word string) []string {
	r := []rune(word)
	if len(r) == 0 {
		return nil
	}
	keys := []string{}
	// A three-letter typo must keep its first character. A four-letter
	// middle transposition can change every pair; both need the fallback.
	if len(r) == 1 || len(r) == 3 && !onlyDigits(word) || len(r) == 4 && !onlyDigits(word) {
		keys = append(keys, "first:"+string(r[0]))
	}
	for i := 0; i+1 < len(r); i++ {
		keys = append(keys, "pair:"+string(r[i:i+2]))
	}
	return keys
}

func onlyDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func (x *searchIndex) addPostings(kind string, id primitive.ObjectID, entry *searchEntry) {
	if x.postings[kind] == nil {
		x.postings[kind] = map[string]map[primitive.ObjectID]struct{}{}
	}
	for _, word := range entry.target.IndexWords() {
		for _, key := range searchKeys(word) {
			if x.postings[kind][key] == nil {
				x.postings[kind][key] = map[primitive.ObjectID]struct{}{}
			}
			x.postings[kind][key][id] = struct{}{}
		}
	}
}

func (x *searchIndex) removeEntry(kind string, id primitive.ObjectID) {
	if old := x.entries[kind][id]; old != nil {
		for _, word := range old.target.IndexWords() {
			for _, key := range searchKeys(word) {
				delete(x.postings[kind][key], id)
				if len(x.postings[kind][key]) == 0 {
					delete(x.postings[kind], key)
				}
			}
		}
	}
	delete(x.entries[kind], id)
}

func (x *searchIndex) candidateIDs(kind string, q fuzzy.Query) map[primitive.ObjectID]struct{} {
	var candidates map[primitive.ObjectID]struct{}
	for _, alternatives := range q.IndexTerms() {
		wordCandidates := map[primitive.ObjectID]struct{}{}
		for _, word := range alternatives {
			for _, key := range queryKeys(word) {
				for id := range x.postings[kind][key] {
					wordCandidates[id] = struct{}{}
				}
			}
		}
		if candidates == nil {
			candidates = wordCandidates
		} else {
			for id := range candidates {
				if _, ok := wordCandidates[id]; !ok {
					delete(candidates, id)
				}
			}
		}
		if len(candidates) == 0 {
			break
		}
	}
	return candidates
}

// apply folds one change event into the index; false asks for a rebuild
// (the stream was invalidated by a drop or rename).
func (x *searchIndex) apply(raw bson.Raw) bool {
	var ev struct {
		Op string `bson:"operationType"`
		NS struct {
			Coll string `bson:"coll"`
		} `bson:"ns"`
		Key struct {
			ID primitive.ObjectID `bson:"_id"`
		} `bson:"documentKey"`
		Full bson.Raw `bson:"fullDocument"`
	}
	if err := bson.Unmarshal(raw, &ev); err != nil {
		return false
	}
	kind, ok := kindOfColl[ev.NS.Coll]
	switch ev.Op {
	case "insert", "update", "replace":
		if !ok {
			return true
		}
		if len(ev.Full) == 0 { // deleted again before we looked it up
			x.removeEntry(kind, ev.Key.ID)
			return true
		}
		x.removeEntry(kind, ev.Key.ID)
		if e := buildEntry(kind, ev.Full); e != nil {
			x.entries[kind][ev.Key.ID] = e
			x.addPostings(kind, ev.Key.ID, e)
		}
		return true
	case "delete":
		if ok {
			x.removeEntry(kind, ev.Key.ID)
		}
		return true
	default: // drop, rename, dropDatabase, invalidate
		return false
	}
}

// search ranks every entry of the given kinds against q.
func (x *searchIndex) search(ctx context.Context, q fuzzy.Query, kinds ...string) (map[string][]searchHit, error) {
	if err := x.sync(ctx); err != nil {
		return nil, err
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	out := map[string][]searchHit{}
	for _, kind := range kinds {
		type scored struct {
			hit     searchHit
			primary string
		}
		var got []scored
		for id := range x.candidateIDs(kind, q) {
			e := x.entries[kind][id]
			if e == nil {
				continue
			}
			r := fuzzy.Match(q, e.target)
			if r.Score < fuzzy.MinScore {
				continue
			}
			h := e.hit
			h.Score, h.Typo = r.Score, r.Typo
			got = append(got, scored{h, e.target.Primary})
		}
		sort.SliceStable(got, func(i, j int) bool {
			a, b := got[i], got[j]
			if a.hit.Score != b.hit.Score || a.primary != b.primary {
				return fuzzy.Less(a.hit.Score, a.primary, b.hit.Score, b.primary)
			}
			if !a.hit.Date.Equal(b.hit.Date) {
				return a.hit.Date.After(b.hit.Date) // same text: newest first
			}
			return a.hit.ID < b.hit.ID
		})
		hits := make([]searchHit, len(got))
		for i, g := range got {
			hits[i] = g.hit
		}
		out[kind] = hits
	}
	return out, nil
}

// correction is the "Did you mean…" for a query: when nothing matched
// without a typo, the best result's corrected words.
func (x *searchIndex) correction(q fuzzy.Query, groups map[string][]searchHit) string {
	var best *searchHit
	for _, hits := range groups {
		for i := range hits {
			if !hits[i].Typo {
				return ""
			}
			if best == nil || hits[i].Score > best.Score {
				best = &hits[i]
			}
		}
	}
	if best == nil {
		return ""
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	id, _ := primitive.ObjectIDFromHex(best.ID)
	if e := x.entries[best.Kind][id]; e != nil {
		return fuzzy.Match(q, e.target).Corrected
	}
	return ""
}

// rankedIDs is the order-preserving list of a kind's matches, for lists
// that page through search results.
func (x *searchIndex) rankedIDs(ctx context.Context, q fuzzy.Query, kind string) ([]primitive.ObjectID, error) {
	groups, err := x.search(ctx, q, kind)
	if err != nil {
		return nil, err
	}
	ids := make([]primitive.ObjectID, 0, len(groups[kind]))
	for _, h := range groups[kind] {
		id, _ := primitive.ObjectIDFromHex(h.ID)
		ids = append(ids, id)
	}
	return ids, nil
}

func buildEntry(kind string, raw bson.Raw) *searchEntry {
	switch kind {
	case kindTrainee:
		var t models.Trainee
		if bson.Unmarshal(raw, &t) != nil {
			return nil
		}
		flag := ""
		if t.ArchivedAt != nil {
			flag = "archived"
		} else if t.Status == models.StatusInactive {
			flag = "inactive"
		}
		return &searchEntry{
			hit:    searchHit{Kind: kind, ID: t.ID.Hex(), Label: t.Name, Sub: t.Phone, Date: t.CreatedAt, Flag: flag},
			target: fuzzy.NewTarget(t.Name, t.Phone),
		}
	case kindSession:
		var s models.Session
		if bson.Unmarshal(raw, &s) != nil {
			return nil
		}
		names := make([]string, 0, len(s.Attendees))
		for _, a := range s.Attendees {
			names = append(names, a.TraineeName)
		}
		flag := ""
		if s.Status != models.SessScheduled {
			flag = s.Status
		}
		return &searchEntry{
			hit: searchHit{Kind: kind, ID: s.ID.Hex(), Label: s.Title, Sub: strings.Join(firstN(names, 3), ", "),
				Date: s.Start, Flag: flag, Month: models.MonthKey(s.Start)},
			target: fuzzy.NewTarget(s.Title, strings.Join(names, " "), s.Type, s.Location),
		}
	case kindPayment:
		var p models.Payment
		if bson.Unmarshal(raw, &p) != nil {
			return nil
		}
		typ := payTypeWord(p.Type)
		label := defaultStr(p.TraineeName, typ)
		sub := typ
		if p.PeriodMonth != "" {
			sub += " · " + p.PeriodMonth
		}
		amt := p.Amount
		h := searchHit{Kind: kind, ID: p.ID.Hex(), Label: label, Sub: sub, Date: p.Date, Amount: &amt,
			Flag: voidFlag(p.VoidedAt), Month: models.MonthKey(p.Date)}
		if p.Trainee != nil {
			h.Trainee = p.Trainee.Hex()
		}
		return &searchEntry{hit: h, target: fuzzy.NewTarget(label, p.Note, p.Reference, typ, p.PeriodMonth)}
	case kindSale:
		var s models.Sale
		if bson.Unmarshal(raw, &s) != nil {
			return nil
		}
		amt := s.Total - s.ReturnedTotal
		return &searchEntry{
			hit: searchHit{Kind: kind, ID: s.ID.Hex(), Label: fmt.Sprintf("%s × %d", s.ItemName, s.Qty),
				Sub: defaultStr(s.TraineeName, "Walk-in"), Date: s.Date, Amount: &amt, Flag: voidFlag(s.VoidedAt), Month: models.MonthKey(s.Date)},
			target: fuzzy.NewTarget(s.ItemName, s.TraineeName, "sale"),
		}
	case kindItem:
		var it models.InventoryItem
		if bson.Unmarshal(raw, &it) != nil {
			return nil
		}
		flag := ""
		if !it.Active {
			flag = "archived"
		}
		price := it.Price
		return &searchEntry{
			hit:    searchHit{Kind: kind, ID: it.ID.Hex(), Label: it.Name, Sub: it.SKU, Date: it.CreatedAt, Amount: &price, Flag: flag},
			target: fuzzy.NewTarget(it.Name, it.SKU),
		}
	case kindExpense:
		var e models.Expense
		if bson.Unmarshal(raw, &e) != nil {
			return nil
		}
		cat := categoryWord(e.Category)
		amt := e.Amount
		return &searchEntry{
			hit: searchHit{Kind: kind, ID: e.ID.Hex(), Label: cat, Sub: e.Note, Date: e.Date, Amount: &amt,
				Flag: voidFlag(e.VoidedAt), Month: models.MonthKey(e.Date)},
			target: fuzzy.NewTarget(cat, e.Note, "expense"),
		}
	case kindReminder:
		var r models.Reminder
		if bson.Unmarshal(raw, &r) != nil {
			return nil
		}
		r = withDay(r)
		flag := ""
		if r.Done {
			flag = "done"
		}
		return &searchEntry{
			hit:    searchHit{Kind: kind, ID: r.ID.Hex(), Label: r.Title, Sub: r.RelatedLabel, Date: r.DueDate, Flag: flag},
			target: fuzzy.NewTarget(r.Title, r.RelatedLabel),
		}
	case kindMovement:
		var m models.StockMovement
		if bson.Unmarshal(raw, &m) != nil {
			return nil
		}
		return &searchEntry{
			hit:    searchHit{Kind: kind, ID: m.ID.Hex(), Label: m.ItemName, Sub: m.Kind, Date: m.At},
			target: fuzzy.NewTarget(m.Kind, m.Reason, m.Actor, m.ItemName),
		}
	}
	return nil
}

func voidFlag(t *time.Time) string {
	if t != nil {
		return "void"
	}
	return ""
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

var payTypeWords = map[string]string{
	models.PaySubscription: "Subscription", models.PayPrivate: "Private session", models.PayDropin: "Drop-in",
	models.PaySale: "Shop sale", models.PayOther: "Other",
}

func payTypeWord(t string) string { return defaultStr(payTypeWords[t], t) }

var categoryWords = map[string]string{
	"rent": "Rent", "equipment": "Equipment", "utilities": "Utilities", "supplies": "Supplies", "wages": "Wages", "other": "Other",
}

func categoryWord(c string) string { return defaultStr(categoryWords[c], c) }

// indexes holds one search index per database (tests run many side by side).
var indexes sync.Map // *db.Store → *searchIndex

func searchIndexFor(store *db.Store) *searchIndex {
	if x, ok := indexes.Load(store); ok {
		return x.(*searchIndex)
	}
	x, _ := indexes.LoadOrStore(store, newSearchIndex(store))
	return x.(*searchIndex)
}
