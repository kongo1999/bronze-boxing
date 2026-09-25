package server

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"bronzeboxing/internal/fuzzy"
	"bronzeboxing/internal/models"
)

type searchOut struct {
	Query      string `json:"query"`
	DidYouMean string `json:"didYouMean"`
	Groups     []struct {
		Kind    string `json:"kind"`
		Total   int    `json:"total"`
		HasMore bool   `json:"hasMore"`
		Items   []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
			Flag  string `json:"flag"`
			Typo  bool   `json:"typo"`
		} `json:"items"`
	} `json:"groups"`
}

func (e *testEnv) search(q string, extra ...string) searchOut {
	e.t.Helper()
	var out searchOut
	path := "/search?q=" + url.QueryEscape(q)
	for _, x := range extra {
		path += "&" + x
	}
	e.ok("GET", path, nil, &out)
	return out
}

// labels of one group, in order.
func (s searchOut) labels(kind string) []string {
	for _, g := range s.Groups {
		if g.Kind == kind {
			out := []string{}
			for _, it := range g.Items {
				out = append(out, it.Label)
			}
			return out
		}
	}
	return nil
}

func (s searchOut) flag(kind, label string) string {
	for _, g := range s.Groups {
		if g.Kind == kind {
			for _, it := range g.Items {
				if it.Label == label {
					return it.Flag
				}
			}
		}
	}
	return "-"
}

// Typed groups, typo tolerance and "did you mean", across record kinds.
func TestGlobalSearchGroupsTyposAndSuggestions(t *testing.T) {
	e := newEnv(t)
	jad := e.addTrainee("Jad Saliba", 50)
	e.addTrainee("Karim Haddad", 50)
	e.ok("PUT", "/trainees/"+jad.ID, map[string]any{"phone": "70 555 666"}, nil)
	it := e.addItem("Boxing Gloves 12oz", 5, 45, 28)
	e.ok("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 1, "trainee": jad.ID}, nil)
	e.ok("POST", "/expenses", map[string]any{"amount": 30, "category": "supplies", "note": "Glove spray"}, nil)
	e.ok("POST", "/reminders", map[string]any{"title": "Order more gloves", "dueDay": models.DateKey(time.Now()), "priority": "normal"}, nil)

	got := e.search("jda")
	if l := got.labels(kindTrainee); len(l) != 1 || l[0] != "Jad Saliba" {
		t.Fatalf("trainees for 'jda' = %v", l)
	}
	if got.DidYouMean != "jad" {
		t.Fatalf("didYouMean = %q, want jad", got.DidYouMean)
	}
	if got := e.search("5556"); len(got.labels(kindTrainee)) != 1 {
		t.Fatalf("phone fragment: %+v", got)
	}
	gl := e.search("gloves")
	if gl.DidYouMean != "" {
		t.Fatalf("exact matches exist, no suggestion expected: %q", gl.DidYouMean)
	}
	for _, k := range []string{kindSale, kindItem, kindExpense, kindReminder} {
		if len(gl.labels(k)) != 1 {
			t.Fatalf("'gloves' in %s: %v (groups %+v)", k, gl.labels(k), gl.Groups)
		}
	}
	if got := e.search("zzqx"); len(got.Groups) != 0 || got.DidYouMean != "" {
		t.Fatalf("nonsense matched: %+v", got)
	}
	// One kind, paged: "show all" for a group.
	only := e.search("gloves", "kinds=item", "limit=1")
	if len(only.Groups) != 1 || only.Groups[0].Kind != kindItem {
		t.Fatalf("kinds filter: %+v", only.Groups)
	}
}

// Every mutation is visible to the very next search: a rename, an archive,
// a void, a delete — and even a write made straight to the database.
func TestSearchNeverServesStaleResults(t *testing.T) {
	e := newEnv(t)
	tr := e.addTrainee("Jad Saliba", 50)
	if l := e.search("saliba").labels(kindTrainee); len(l) != 1 {
		t.Fatalf("before rename: %v", l)
	}
	e.ok("PUT", "/trainees/"+tr.ID, map[string]any{"name": "Jad Salem"}, nil)
	if l := e.search("saliba").labels(kindTrainee); len(l) != 0 {
		t.Fatalf("old name still found: %v", l)
	}
	if l := e.search("salem").labels(kindTrainee); len(l) != 1 {
		t.Fatalf("new name not found: %v", l)
	}
	e.ok("POST", "/trainees/"+tr.ID+"/archive", map[string]any{"reason": "moved away"}, nil)
	if f := e.search("salem").flag(kindTrainee, "Jad Salem"); f != "archived" {
		t.Fatalf("archived flag = %q", f)
	}

	p := e.pay(tr.ID, 20, models.MonthKey(nowFn()))
	if f := e.search("salem").flag(kindPayment, "Jad Salem"); f != "" {
		t.Fatalf("live payment flag = %q", f)
	}
	e.ok("POST", "/payments/"+p.ID+"/void", map[string]any{"reason": "entered twice"}, nil)
	if f := e.search("salem").flag(kindPayment, "Jad Salem"); f != "void" {
		t.Fatalf("voided payment flag = %q", f)
	}

	it := e.addItem("Speed Rope", 3, 12, 5)
	if l := e.search("rope").labels(kindItem); len(l) != 1 {
		t.Fatalf("item: %v", l)
	}
	e.ok("DELETE", "/inventory/"+it.ID, nil, nil)
	if l := e.search("rope").labels(kindItem); len(l) != 0 {
		t.Fatalf("deleted item still found: %v", l)
	}

	// Straight to the database, bypassing every handler.
	if _, err := e.store.Coll(models.CollInventory).InsertOne(e.ctx, models.InventoryItem{
		ID: primitive.NewObjectID(), Name: "Heavy Bag", Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if l := e.search("heavy bag").labels(kindItem); len(l) != 1 {
		t.Fatalf("direct insert not found: %v", l)
	}
}

// Server-side search covers the whole filtered list, not a page of it: a
// match buried among 1,200 records, and one that would sit on page five.
func TestListSearchFindsMatchesBeyondPageOne(t *testing.T) {
	e := newEnv(t)
	docs := make([]any, 0, 1200)
	for i := 0; i < 1200; i++ {
		docs = append(docs, models.Trainee{ID: primitive.NewObjectID(), Name: fmt.Sprintf("Member %04d", i),
			Status: models.StatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	}
	if _, err := e.store.Coll(models.CollTrainees).InsertMany(e.ctx, docs); err != nil {
		t.Fatal(err)
	}
	zeina := e.addTrainee("Zeina Khalil", 40)
	var roster []traineeOut
	e.ok("GET", "/trainees?q="+url.QueryEscape("zeyna khalil"), nil, &roster)
	if len(roster) != 1 || roster[0].ID != zeina.ID {
		t.Fatalf("roster search = %+v", roster)
	}

	month := models.MonthKey(nowFn())
	e.pay(zeina.ID, 40, month) // the oldest payment: last page by date
	other := e.addTrainee("Rami Khoury", 0)
	for i := 0; i < 45; i++ {
		e.ok("POST", "/payments", map[string]any{"trainee": other.ID, "amount": 5, "type": "dropin"}, nil)
	}
	var pg struct {
		Items []struct {
			TraineeName string `json:"traineeName"`
		} `json:"items"`
		Total int `json:"total"`
	}
	e.ok("GET", "/payments?limit=10&q="+url.QueryEscape("zeina"), nil, &pg)
	if pg.Total != 1 || pg.Items[0].TraineeName != "Zeina Khalil" {
		t.Fatalf("payments search = %+v", pg)
	}
	// The list's own filters still apply to a search.
	e.ok("GET", "/payments?limit=10&type=dropin&q="+url.QueryEscape("zeina"), nil, &pg)
	if pg.Total != 0 {
		t.Fatalf("type filter ignored under search: %+v", pg)
	}
	// The ledger's search is the same matcher.
	var led struct {
		Total int `json:"total"`
	}
	e.ok("GET", "/ledger?m="+month+"&q="+url.QueryEscape("zeyna"), nil, &led)
	if led.Total != 1 {
		t.Fatalf("ledger search total = %d", led.Total)
	}
}

func TestSearchPostingsPreserveFuzzyMatches(t *testing.T) {
	idx := &searchIndex{entries: map[string]map[primitive.ObjectID]*searchEntry{kindTrainee: {}},
		postings: map[string]map[string]map[primitive.ObjectID]struct{}{}}
	targets := []fuzzy.Target{
		fuzzy.NewTarget("Zeina Khalil", "71 555 666"),
		fuzzy.NewTarget("Jad Saliba"),
		fuzzy.NewTarget("Private session"),
		fuzzy.NewTarget("No-show reminder"),
		fuzzy.NewTarget("Boxing T-shirt"),
	}
	for _, target := range targets {
		id := primitive.NewObjectID()
		entry := &searchEntry{target: target}
		idx.entries[kindTrainee][id] = entry
		idx.addPostings(kindTrainee, id, entry)
	}
	queries := []string{"zeina", "zeyna", "ezina", "ziena", "khalil", "jda", "pt", "private", "no show", "noshow", "tee", "5556", "boxing shirt"}
	for _, raw := range queries {
		q := fuzzy.NewQuery(raw)
		got := idx.candidateIDs(kindTrainee, q)
		for id, entry := range idx.entries[kindTrainee] {
			if fuzzy.Match(q, entry.target).Score > 0 {
				if _, ok := got[id]; !ok {
					t.Fatalf("posting index missed %q for target %q", raw, entry.target.Primary)
				}
			}
		}
	}
}

func TestSearchPostingsCoverSharedFuzzyFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/fixtures/fuzzy-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name    string     `json:"name"`
			Q       string     `json:"q"`
			Targets [][]string `json:"targets"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			idx := &searchIndex{entries: map[string]map[primitive.ObjectID]*searchEntry{kindTrainee: {}},
				postings: map[string]map[string]map[primitive.ObjectID]struct{}{}}
			for _, fields := range c.Targets {
				id := primitive.NewObjectID()
				entry := &searchEntry{target: fuzzy.NewTarget(fields[0], fields[1:]...)}
				idx.entries[kindTrainee][id] = entry
				idx.addPostings(kindTrainee, id, entry)
			}
			q := fuzzy.NewQuery(c.Q)
			candidates := idx.candidateIDs(kindTrainee, q)
			for id, entry := range idx.entries[kindTrainee] {
				if fuzzy.Match(q, entry.target).Score > 0 {
					if _, ok := candidates[id]; !ok {
						t.Fatalf("missed %q for %q", c.Q, entry.target.Primary)
					}
				}
			}
		})
	}
}

func TestMovementHistorySearchAndPaging(t *testing.T) {
	e := newEnv(t)
	it := e.addItem("Heavy bag", 5, 10, 4)
	id := oid(t, it.ID)
	docs := make([]any, 0, 125)
	for i := 0; i < 124; i++ {
		docs = append(docs, models.StockMovement{ID: primitive.NewObjectID(), Item: id,
			ItemName: "Heavy bag", Kind: models.MoveCorrection, Reason: fmt.Sprintf("Routine count %03d", i),
			At: time.Now().Add(time.Duration(i) * time.Minute)})
	}
	docs = append(docs, models.StockMovement{ID: primitive.NewObjectID(), Item: id,
		ItemName: "Heavy bag", Kind: models.MoveCorrection, Reason: "Misplaced stock found", At: time.Now().Add(-time.Hour)})
	if _, err := e.store.Coll(models.CollMovements).InsertMany(e.ctx, docs); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Items []models.StockMovement `json:"items"`
		Total int                    `json:"total"`
	}
	e.ok("GET", "/inventory/"+it.ID+"/movements?limit=10&q=misplced", nil, &page)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Reason != "Misplaced stock found" {
		t.Fatalf("movement search = %+v", page)
	}
	e.ok("GET", "/inventory/"+it.ID+"/movements?limit=10&offset=120", nil, &page)
	if page.Total != 126 || len(page.Items) != 6 {
		t.Fatalf("movement paging = total %d, items %d", page.Total, len(page.Items))
	}
}

func BenchmarkSearchCandidates10000(b *testing.B) {
	idx := &searchIndex{entries: map[string]map[primitive.ObjectID]*searchEntry{kindTrainee: {}},
		postings: map[string]map[string]map[primitive.ObjectID]struct{}{}}
	for i := 0; i < 10000; i++ {
		id := primitive.NewObjectID()
		entry := &searchEntry{target: fuzzy.NewTarget(fmt.Sprintf("Member %05d", i))}
		idx.entries[kindTrainee][id] = entry
		idx.addPostings(kindTrainee, id, entry)
	}
	id := primitive.NewObjectID()
	entry := &searchEntry{target: fuzzy.NewTarget("Zeina Khalil")}
	idx.entries[kindTrainee][id] = entry
	idx.addPostings(kindTrainee, id, entry)
	for _, raw := range []string{"zeyna khalil", "member 042"} {
		b.Run(raw, func(b *testing.B) {
			q := fuzzy.NewQuery(raw)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for id := range idx.candidateIDs(kindTrainee, q) {
					_ = fuzzy.Match(q, idx.entries[kindTrainee][id].target)
				}
			}
		})
	}
}
