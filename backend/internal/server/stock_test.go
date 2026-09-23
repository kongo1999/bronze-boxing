package server

import (
	"net/http"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"bronzeboxing/internal/models"
)

type movementOut struct {
	Delta      int    `json:"delta"`
	Kind       string `json:"kind"`
	Reason     string `json:"reason"`
	StockAfter int    `json:"stockAfter"`
	Actor      string `json:"actor"`
}

type saleFullOut struct {
	ID            string  `json:"id"`
	Qty           int     `json:"qty"`
	Total         float64 `json:"total"`
	Method        string  `json:"method"`
	ReturnedQty   int     `json:"returnedQty"`
	ReturnedTotal float64 `json:"returnedTotal"`
}

// Deliveries, write-offs and counted corrections each move stock by exactly
// what they say, write one ledger line with a reason, and keep the ledger
// adding up to the shelf.
func TestStockAdjustmentsKeepTheLedgerTrue(t *testing.T) {
	e := newEnv(t)
	it := e.addItem("Wraps", 4, 12, 5)

	var res struct {
		Item struct {
			Stock     int     `json:"stock"`
			CostPrice float64 `json:"costPrice"`
		} `json:"item"`
		Movement movementOut `json:"movement"`
	}
	adj := "/inventory/" + it.ID + "/adjustments"
	e.ok("POST", adj, map[string]any{"kind": "restock", "qty": 10, "unitCost": 4.5}, &res)
	if res.Item.Stock != 14 || res.Movement.Delta != 10 || res.Movement.StockAfter != 14 || res.Item.CostPrice != 4.5 {
		t.Fatalf("restock = %+v", res)
	}
	e.fail("POST", adj, map[string]any{"kind": "damage", "qty": 2}, http.StatusBadRequest, CodeReasonRequired)
	e.fail("POST", adj, map[string]any{"kind": "damage", "qty": 99, "reason": "flood"}, http.StatusConflict, CodeInsufficient)
	e.ok("POST", adj, map[string]any{"kind": "damage", "qty": 2, "reason": "torn"}, &res)
	if res.Item.Stock != 12 || res.Movement.Delta != -2 || res.Movement.Reason != "torn" {
		t.Fatalf("damage = %+v", res)
	}
	e.fail("POST", adj, map[string]any{"kind": "correction", "count": 11}, http.StatusBadRequest, CodeReasonRequired)
	e.fail("POST", adj, map[string]any{"kind": "correction", "count": 12, "reason": "count"}, http.StatusBadRequest, CodeValidation)
	// The count was taken when the shelf read 14; it has moved since.
	e.fail("POST", adj, map[string]any{"kind": "correction", "count": 11, "expected": 14, "reason": "count"}, http.StatusConflict, CodeConflict)
	e.ok("POST", adj, map[string]any{"kind": "correction", "count": 11, "expected": 12, "reason": "shelf count"}, &res)
	if res.Item.Stock != 11 || res.Movement.Delta != -1 {
		t.Fatalf("correction = %+v", res)
	}
	e.fail("POST", adj, map[string]any{"kind": "sale", "qty": 1}, http.StatusBadRequest, CodeValidation)
	e.fail("POST", adj, map[string]any{"kind": "restock", "qty": 0}, http.StatusBadRequest, CodeValidation)

	var moves []movementOut
	e.ok("GET", "/inventory/"+it.ID+"/movements", nil, &moves)
	kinds := []string{}
	for _, m := range moves {
		kinds = append(kinds, m.Kind)
	}
	if len(moves) != 4 || kinds[0] != "correction" || kinds[3] != "opening" {
		t.Fatalf("ledger = %v", kinds)
	}
	if n := e.count(models.CollAudit, bson.M{"entity": "item", "action": bson.M{"$in": []string{"restock", "damage", "correction"}}}); n != 3 {
		t.Fatalf("audit lines = %d, want 3", n)
	}
	e.assertIntegrity()
}

// An archived item can't be sold and drops out of the active list; an item
// with history can only be archived, while one added by mistake can be
// deleted outright.
func TestArchiveInsteadOfDeleteForItemsWithHistory(t *testing.T) {
	e := newEnv(t)
	it := e.addItem("Water", 20, 1, 0.4)
	e.ok("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 1}, nil)

	body := e.fail("DELETE", "/inventory/"+it.ID, nil, http.StatusConflict, CodeConflict)
	if body.Details == nil {
		t.Fatal("refusal should say what history exists")
	}
	e.ok("POST", "/inventory/"+it.ID+"/archive", map[string]any{"reason": "discontinued"}, nil)
	e.fail("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 1}, http.StatusConflict, CodeItemInactive)
	var active []itemOut
	e.ok("GET", "/inventory?active=true", nil, &active)
	if len(active) != 0 {
		t.Fatalf("archived item still listed as active: %+v", active)
	}
	e.ok("POST", "/inventory/"+it.ID+"/unarchive", nil, nil)
	e.ok("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 1}, nil)

	oops := e.addItem("Typo item", 3, 5, 0)
	e.ok("DELETE", "/inventory/"+oops.ID, nil, nil)
	if n := e.count(models.CollMovements, bson.M{"item": oid(t, oops.ID)}); n != 0 {
		t.Fatalf("deleted item left %d ledger lines", n)
	}
	e.assertIntegrity()
}

// Shortages: out at zero, low at or under the threshold.
func TestInventoryShortageFilter(t *testing.T) {
	e := newEnv(t)
	e.ok("POST", "/inventory", map[string]any{"name": "Gloves", "stock": 2, "price": 45, "lowStockThreshold": 3}, nil)
	e.ok("POST", "/inventory", map[string]any{"name": "Tape", "stock": 0, "price": 3}, nil)
	e.ok("POST", "/inventory", map[string]any{"name": "Shirts", "stock": 9, "price": 20, "lowStockThreshold": 3}, nil)
	names := func(q string) []string {
		var out []itemOut
		e.ok("GET", "/inventory?stock="+q, nil, &out)
		n := []string{}
		for _, it := range out {
			n = append(n, it.Name)
		}
		return n
	}
	if got := names("short"); len(got) != 2 || got[0] != "Gloves" || got[1] != "Tape" {
		t.Fatalf("short = %v", got)
	}
	if got := names("out"); len(got) != 1 || got[0] != "Tape" {
		t.Fatalf("out = %v", got)
	}
	e.fail("GET", "/inventory?stock=plenty", nil, http.StatusBadRequest, CodeValidation)
}

// A partial return restocks, refunds on its own cash day and is capped by
// what's left of the sale; racing returns can't take back more than was
// sold; voiding afterwards restocks only what was still out and nets the
// money to zero.
func TestPartialReturnsRestockRefundAndCap(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	it := e.addItem("Gloves", 5, 10, 6)
	var s saleFullOut
	e.ok("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 3, "method": "card"}, &s)
	if s.Method != "card" || s.Total != 30 || e.stock(it.ID) != 2 {
		t.Fatalf("sale = %+v stock %d", s, e.stock(it.ID))
	}
	ret := "/sales/" + s.ID + "/returns"
	e.fail("POST", ret, map[string]any{"qty": 1}, http.StatusBadRequest, CodeReasonRequired)
	e.fail("POST", ret, map[string]any{"qty": 4, "reason": "x"}, http.StatusBadRequest, CodeReturnExceeds)
	e.fail("POST", ret, map[string]any{"qty": 1, "amount": 31, "reason": "x"}, http.StatusBadRequest, CodeReturnExceeds)

	var r struct {
		Sale      saleFullOut `json:"sale"`
		Restocked int         `json:"restocked"`
	}
	e.ok("POST", ret, map[string]any{"qty": 1, "reason": "wrong size"}, &r)
	if r.Sale.ReturnedQty != 1 || r.Sale.ReturnedTotal != 10 || r.Restocked != 1 || e.stock(it.ID) != 3 {
		t.Fatalf("after return: %+v stock %d", r, e.stock(it.ID))
	}
	var fin finOut
	e.ok("GET", "/financials?m="+month, nil, &fin)
	if fin.Income != 20 {
		t.Fatalf("income = %v, want 30 sold - 10 refunded", fin.Income)
	}

	// Two returns of 2 race for the 2 units still out: exactly one wins.
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = e.req("POST", ret, map[string]any{"qty": 2, "reason": "race"})
		}(i)
	}
	wg.Wait()
	wins := 0
	for _, c := range codes {
		if c == http.StatusCreated {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("racing returns: codes %v, want exactly one success", codes)
	}
	var got saleFullOut
	e.ok("GET", "/sales/"+s.ID, nil, &got)
	if got.ReturnedQty != 3 || e.stock(it.ID) != 5 {
		t.Fatalf("returnedQty %d stock %d, want 3 and 5", got.ReturnedQty, e.stock(it.ID))
	}
	e.fail("POST", ret, map[string]any{"qty": 1, "reason": "again"}, http.StatusBadRequest, CodeReturnExceeds)

	var rc struct {
		Number  string              `json:"number"`
		Net     float64             `json:"net"`
		Returns []struct{ Qty int } `json:"returns"`
		Method  string              `json:"method"`
	}
	e.ok("GET", "/sales/"+s.ID+"/receipt", nil, &rc)
	if rc.Net != 0 || len(rc.Returns) != 2 || rc.Method != "card" || rc.Number[:2] != "S-" {
		t.Fatalf("receipt = %+v", rc)
	}

	// Void: nothing left out to restock, and the month nets to zero.
	var v struct {
		Restocked int `json:"restocked"`
	}
	e.ok("POST", "/sales/"+s.ID+"/void", map[string]any{"reason": "entered twice"}, &v)
	e.ok("GET", "/financials?m="+month, nil, &fin)
	if v.Restocked != 0 || e.stock(it.ID) != 5 || fin.Income != 0 {
		t.Fatalf("after void: restocked %d stock %d income %v", v.Restocked, e.stock(it.ID), fin.Income)
	}
	e.fail("POST", ret, map[string]any{"qty": 1, "reason": "late"}, http.StatusConflict, CodeVoidedLocked)
	e.assertIntegrity()
}
