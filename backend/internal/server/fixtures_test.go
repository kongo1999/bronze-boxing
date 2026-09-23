package server

import (
	"encoding/json"
	"regexp"
	"testing"

	"bronzeboxing/internal/models"
)

// TestRecordContractFixtures exercises one of each core record through the
// real API and, with UPDATE_FIXTURES=1, writes the responses to
// docs/fixtures/*.json. Ids and timestamps are normalized so the files only
// change when the contract does.
func TestRecordContractFixtures(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())

	raw := func(method, path string, body any) any {
		t.Helper()
		st, b := e.req(method, path, body)
		if st >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, st, b)
		}
		var v any
		_ = json.Unmarshal(b, &v)
		return normalize(v)
	}
	id := func(v any) string { return v.(map[string]any)["id"].(string) }

	st, b := e.req("POST", "/trainees", map[string]any{"name": "Karim Haddad", "phone": "03 111 222", "skillLevel": "intermediate", "monthlyFee": 50})
	if st != 201 {
		t.Fatalf("trainee: %d %s", st, b)
	}
	var tr map[string]any
	_ = json.Unmarshal(b, &tr)
	tid := tr["id"].(string)
	e.fixture("trainee", normalize(tr))

	e.fixture("payment-partial", raw("POST", "/payments", map[string]any{
		"trainee": tid, "amount": 20, "type": "subscription", "periodMonth": month, "note": "Half now",
	}))
	e.fixture("subscriptions", raw("GET", "/subscriptions?m="+month, nil))
	e.fixture("subscription-charges", raw("GET", "/subscription-charges?trainee="+tid, nil))
	e.fixture("expense", raw("POST", "/expenses", map[string]any{"amount": 95.5, "category": "utilities", "note": "Electricity"}))
	e.fixture("session", raw("POST", "/sessions", map[string]any{
		"title": "Evening Group Class", "type": "group", "start": nowFn().Add(72 * 3600e9), "durationMin": 60,
		"location": "Main floor", "capacity": 12, "attendees": []map[string]any{{"trainee": tid}},
	}))
	var item map[string]any
	st, b = e.req("POST", "/inventory", map[string]any{"name": "Hand Wraps", "sku": "WRAP-45", "stock": 24, "price": 8, "costPrice": 4, "lowStockThreshold": 6})
	if st != 201 {
		t.Fatalf("item: %d %s", st, b)
	}
	_ = json.Unmarshal(b, &item)
	itemID := id(item)
	e.fixture("item", normalize(item))
	e.fixture("sale", raw("POST", "/inventory/"+itemID+"/sell", map[string]any{"qty": 2, "trainee": tid}))
	e.fixture("stock-movements", raw("GET", "/inventory/"+itemID+"/movements", nil))
	e.fixture("error-overpayment", func() any {
		_, b := e.req("POST", "/payments", map[string]any{"trainee": tid, "amount": 100, "type": "subscription", "periodMonth": month})
		var v any
		_ = json.Unmarshal(b, &v)
		return v
	}())
}

var (
	hexID   = regexp.MustCompile(`^[0-9a-f]{24}$`)
	isoTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)
	month   = regexp.MustCompile(`^\d{4}-\d{2}$`)
	day     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// normalize replaces volatile values (ids, instants, current month/day) with
// stable placeholders.
func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			t[k] = normalize(x)
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = normalize(x)
		}
		return t
	case string:
		switch {
		case hexID.MatchString(t):
			return "<id>"
		case isoTime.MatchString(t):
			return "<timestamp>"
		case month.MatchString(t):
			return "<YYYY-MM>"
		case day.MatchString(t):
			return "<YYYY-MM-DD>"
		}
	}
	return v
}
