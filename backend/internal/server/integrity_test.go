package server

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bronzeboxing/internal/models"
)

type itemOut struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Stock int     `json:"stock"`
	Price float64 `json:"price"`
}

type saleOut struct {
	ID        string  `json:"id"`
	Qty       int     `json:"qty"`
	Total     float64 `json:"total"`
	UnitCost  float64 `json:"unitCost"`
	VoidedAt  *string `json:"voidedAt"`
	UnitPrice float64 `json:"unitPrice"`
}

func (e *testEnv) addItem(name string, stock int, price, cost float64) itemOut {
	e.t.Helper()
	var it itemOut
	e.ok("POST", "/inventory", map[string]any{"name": name, "stock": stock, "price": price, "costPrice": cost}, &it)
	return it
}

func (e *testEnv) stock(id string) int {
	e.t.Helper()
	var it itemOut
	e.ok("GET", "/inventory/"+id, nil, &it)
	return it.Stock
}

func (e *testEnv) assertIntegrity() {
	e.t.Helper()
	findings, err := VerifyIntegrity(e.ctx, e.store)
	if err != nil {
		e.t.Fatalf("verify: %v", err)
	}
	for _, f := range findings {
		e.t.Errorf("integrity: %s: %s", f.Kind, f.Detail)
	}
}

func oid(t *testing.T, hex string) primitive.ObjectID {
	t.Helper()
	id, err := primitive.ObjectIDFromHex(hex)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// A failure after the first write of a sale must leave stock, sales, the
// stock ledger and the audit trail exactly as they were.
func TestSellRollsBackOnMidTransactionFailure(t *testing.T) {
	e := newEnv(t)
	it := e.addItem("Gloves", 3, 45, 28)
	for _, fp := range []string{"sell.afterStock", "sell.afterSale"} {
		failpoint = func(name string) error {
			if name == fp {
				return errors.New("injected failure at " + name)
			}
			return nil
		}
		e.fail("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 2}, http.StatusInternalServerError, "")
		failpoint = func(string) error { return nil }

		if got := e.stock(it.ID); got != 3 {
			t.Fatalf("%s: stock = %d, want 3 (unchanged)", fp, got)
		}
		if n := e.count(models.CollSales, bson.M{}); n != 0 {
			t.Fatalf("%s: %d sales left behind", fp, n)
		}
		if n := e.count(models.CollMovements, bson.M{"kind": models.MoveSale}); n != 0 {
			t.Fatalf("%s: %d orphan sale movements", fp, n)
		}
		if n := e.count(models.CollAudit, bson.M{"entity": "sale"}); n != 0 {
			t.Fatalf("%s: %d orphan audit lines", fp, n)
		}
	}
	e.assertIntegrity()
}

func TestPaymentRollsBackOnMidTransactionFailure(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	tr := e.addTrainee("Karim", 50)
	for _, fp := range []string{"payment.afterCharge", "payment.afterInsert"} {
		failpoint = func(name string) error {
			if name == fp {
				return errors.New("injected")
			}
			return nil
		}
		e.fail("POST", "/payments", map[string]any{"trainee": tr.ID, "amount": 20, "type": "subscription", "periodMonth": month},
			http.StatusInternalServerError, "")
		failpoint = func(string) error { return nil }
		if row := e.dues(month)[tr.ID]; row.AmountPaid != 0 || row.State != models.ChargeUnpaid {
			t.Fatalf("%s: dues row = %+v, want untouched", fp, row)
		}
		if n := e.count(models.CollPayments, bson.M{}); n != 0 {
			t.Fatalf("%s: %d payments left behind", fp, n)
		}
	}
	e.assertIntegrity()
}

// Many simultaneous sells of the last units: exactly the stock that exists is
// sold, never more, and the ledger still adds up.
func TestConcurrentSellsNeverOversell(t *testing.T) {
	e := newEnv(t)
	it := e.addItem("Last wraps", 3, 8, 4)
	const buyers = 12
	var wg sync.WaitGroup
	codes := make([]int, buyers)
	for i := 0; i < buyers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = e.req("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 1})
		}(i)
	}
	wg.Wait()
	sold := 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			sold++
		case http.StatusConflict:
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if sold != 3 {
		t.Fatalf("sold %d, want exactly 3", sold)
	}
	if s := e.stock(it.ID); s != 0 {
		t.Fatalf("stock = %d, want 0", s)
	}
	if n := e.count(models.CollSales, bson.M{}); n != 3 {
		t.Fatalf("%d sales, want 3", n)
	}
	e.fail("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 1}, http.StatusConflict, CodeInsufficient)
	e.assertIntegrity()
}

// Concurrent subscription payments can't jointly overpay a month: the charge
// document serializes them.
func TestConcurrentDuesPaymentsNeverOverpay(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	tr := e.addTrainee("Rami", 50)
	const n = 10
	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = e.req("POST", "/payments", map[string]any{
				"trainee": tr.ID, "amount": 20, "type": "subscription", "periodMonth": month,
			})
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, c := range codes {
		if c == http.StatusCreated {
			ok++
		} else if c != http.StatusBadRequest {
			t.Fatalf("unexpected status %d", c)
		}
	}
	if ok != 2 {
		t.Fatalf("%d payments accepted, want 2 (2×20 ≤ 50 < 3×20)", ok)
	}
	row := e.dues(month)[tr.ID]
	if row.AmountPaid != 40 || row.Remaining != 10 || row.State != models.ChargePartial {
		t.Fatalf("row = %+v, want 40 paid / 10 remaining / partial", row)
	}
	e.assertIntegrity()
}

func TestVoidIsIdempotentAndRequiresReason(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	tr := e.addTrainee("Nour", 80)
	p := e.pay(tr.ID, 30, month)

	e.fail("DELETE", "/payments/"+p.ID, nil, http.StatusBadRequest, CodeReasonRequired)
	e.ok("POST", "/payments/"+p.ID+"/void", map[string]any{"reason": "entered twice"}, nil)
	e.fail("POST", "/payments/"+p.ID+"/void", map[string]any{"reason": "again"}, http.StatusConflict, CodeAlreadyVoided)
	e.fail("PUT", "/payments/"+p.ID, map[string]any{"trainee": tr.ID, "amount": 10, "type": "subscription", "periodMonth": month, "reason": "x"},
		http.StatusConflict, CodeVoidedLocked)

	row := e.dues(month)[tr.ID]
	if row.AmountPaid != 0 || row.State != models.ChargeUnpaid {
		t.Fatalf("after void row = %+v, want unpaid", row)
	}
	var got paymentOut
	e.ok("GET", "/payments/"+p.ID, nil, &got)
	if got.VoidedAt == nil || got.VoidReason != "entered twice" {
		t.Fatalf("payment = %+v, want voided with reason", got)
	}
	e.assertIntegrity()
}

func TestSaleVoidRestocksOnceAndCorrectionNeedsReason(t *testing.T) {
	e := newEnv(t)
	it := e.addItem("Tee", 10, 20, 9)
	var s saleOut
	e.ok("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 2}, &s)
	if s.UnitCost != 9 {
		t.Fatalf("unitCost = %v, want the 9 snapshot", s.UnitCost)
	}
	e.fail("PUT", "/sales/"+s.ID, map[string]any{"qty": 3}, http.StatusBadRequest, CodeReasonRequired)
	e.ok("PUT", "/sales/"+s.ID, map[string]any{"qty": 3, "reason": "miscounted"}, &s)
	if s.Total != 60 || e.stock(it.ID) != 7 {
		t.Fatalf("after correction total %v stock %d, want 60 / 7", s.Total, e.stock(it.ID))
	}
	e.ok("POST", "/sales/"+s.ID+"/void", map[string]any{"reason": "customer changed mind"}, nil)
	e.fail("POST", "/sales/"+s.ID+"/void", map[string]any{"reason": "again"}, http.StatusConflict, CodeAlreadyVoided)
	if got := e.stock(it.ID); got != 10 {
		t.Fatalf("stock after void = %d, want 10", got)
	}
	e.assertIntegrity()
}

func TestDeletedReferencesAreRejectedOrHandled(t *testing.T) {
	e := newEnv(t)
	tr := e.addTrainee("Gone", 0)
	e.ok("DELETE", "/trainees/"+tr.ID, nil, nil)
	it := e.addItem("Rope", 5, 12, 5)

	// Linking to a trainee that no longer exists is refused everywhere.
	e.fail("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 1, "trainee": tr.ID}, http.StatusBadRequest, CodeTraineeNotFound)
	e.fail("POST", "/payments", map[string]any{"trainee": tr.ID, "amount": 5, "type": "dropin"}, http.StatusBadRequest, CodeTraineeNotFound)
	e.fail("POST", "/sessions", map[string]any{"title": "PT", "type": "private", "start": nowFn().Add(48 * 3600e9), "durationMin": 60,
		"attendees": []map[string]any{{"trainee": tr.ID}}}, http.StatusBadRequest, CodeTraineeNotFound)

	// An item with sales can't be deleted any more (archive instead)...
	var s saleOut
	e.ok("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 1}, &s)
	e.fail("DELETE", "/inventory/"+it.ID, nil, http.StatusConflict, CodeConflict)
	// ...but one removed before that rule existed leaves sales that must
	// still void cleanly.
	if _, err := e.store.Coll(models.CollInventory).DeleteOne(e.ctx, bson.M{"_id": oid(t, it.ID)}); err != nil {
		t.Fatal(err)
	}
	var res struct {
		Restocked int `json:"restocked"`
	}
	e.ok("POST", "/sales/"+s.ID+"/void", map[string]any{"reason": "item gone"}, &res)
	if res.Restocked != 0 {
		t.Fatalf("restocked %d into a removed item", res.Restocked)
	}
}

func TestAuditIsCompleteWithActorAndReason(t *testing.T) {
	e := newEnv(t)
	var ex struct {
		ID string `json:"id"`
	}
	e.ok("POST", "/expenses", map[string]any{"amount": 95, "category": "utilities", "note": "Electricity"}, &ex)
	e.fail("PUT", "/expenses/"+ex.ID, map[string]any{"amount": 90, "category": "utilities"}, http.StatusBadRequest, CodeReasonRequired)
	e.ok("PUT", "/expenses/"+ex.ID, map[string]any{"amount": 90, "category": "utilities", "reason": "bill was 90"}, nil)
	e.ok("POST", "/expenses/"+ex.ID+"/void", map[string]any{"reason": "duplicate"}, nil)

	var trail []struct {
		Action string `json:"action"`
		Actor  string `json:"actor"`
		Reason string `json:"reason"`
	}
	e.ok("GET", "/audit/expense/"+ex.ID, nil, &trail)
	if len(trail) != 3 {
		t.Fatalf("trail = %+v, want create+update+void", trail)
	}
	want := []struct{ action, reason string }{{"void", "duplicate"}, {"update", "bill was 90"}, {"create", ""}}
	for i, w := range want {
		if trail[i].Action != w.action || trail[i].Reason != w.reason || trail[i].Actor != "local" {
			t.Errorf("trail[%d] = %+v, want %s/%q by local", i, trail[i], w.action, w.reason)
		}
	}
}

func TestServerRejectsInvalidInput(t *testing.T) {
	e := newEnv(t)
	tr := e.addTrainee("Valid", 10)
	it := e.addItem("Bottle", 5, 5, 2)
	cases := []struct {
		method, path string
		body         any
		field        string
	}{
		{"POST", "/trainees", map[string]any{"name": "X", "monthlyFee": -5}, "monthlyFee"},
		{"POST", "/trainees", map[string]any{"name": "X", "status": "sleeping"}, "status"},
		{"POST", "/trainees", map[string]any{"name": "X", "skillLevel": "pro"}, "skillLevel"},
		{"POST", "/payments", map[string]any{"amount": 10, "type": "gift"}, "type"},
		{"POST", "/payments", map[string]any{"amount": 0, "type": "other"}, "amount"},
		{"POST", "/payments", map[string]any{"trainee": tr.ID, "amount": 5, "type": "subscription", "periodMonth": "2026-13"}, "periodMonth"},
		{"POST", "/payments", map[string]any{"amount": 5, "type": "subscription", "periodMonth": "2026-09"}, "trainee"},
		{"POST", "/payments", map[string]any{"amount": 5, "type": "other", "day": "2026-02-30"}, "day"},
		{"POST", "/expenses", map[string]any{"amount": 5, "category": "snacks"}, "category"},
		{"POST", "/sessions", map[string]any{"title": "T", "start": nowFn(), "durationMin": 0}, "durationMin"},
		{"POST", "/sessions", map[string]any{"title": "T", "start": nowFn(), "durationMin": 60, "type": "solo"}, "type"},
		{"POST", "/sessions", map[string]any{"title": "T", "start": nowFn(), "durationMin": 60,
			"attendees": []map[string]any{{"trainee": tr.ID}, {"trainee": tr.ID}}}, "attendees"},
		{"POST", "/inventory/" + it.ID + "/sell", map[string]any{"qty": 0}, "qty"},
		{"POST", "/inventory/" + it.ID + "/sell", map[string]any{"qty": 1, "unitPrice": -1}, "unitPrice"},
		{"POST", "/reminders", map[string]any{"title": "R", "priority": "urgent"}, "priority"},
		{"GET", "/sessions?from=last-week", nil, "from"},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s %s %s", c.method, c.path, c.field), func(t *testing.T) {
			eb := e.fail(c.method, c.path, c.body, http.StatusBadRequest, "")
			if eb.Field != c.field {
				t.Fatalf("field = %q (%s), want %q", eb.Field, eb.Error, c.field)
			}
		})
	}
}
