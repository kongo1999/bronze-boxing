package server

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"bronzeboxing/internal/models"
)

type finOut struct {
	Income     float64            `json:"income"`
	Outgoings  float64            `json:"outgoings"`
	Net        float64            `json:"net"`
	ByType     map[string]float64 `json:"byType"`
	ByCategory map[string]float64 `json:"byCategory"`
	ByMethod   map[string]float64 `json:"byMethod"`
	Previous   struct {
		Income float64 `json:"income"`
		Net    float64 `json:"net"`
	} `json:"previous"`
}

type ledgerOut struct {
	Items []struct {
		Kind   string  `json:"kind"`
		In     float64 `json:"in"`
		Out    float64 `json:"out"`
		Voided bool    `json:"voided"`
		Method string  `json:"method"`
	} `json:"items"`
	Total  int `json:"total"`
	Totals struct {
		Income    float64 `json:"income"`
		Outgoings float64 `json:"outgoings"`
		Net       float64 `json:"net"`
	} `json:"totals"`
}

// csvTotals reads the "Total in / Total out / Net cash" lines of a statement.
func csvTotals(t *testing.T, raw []byte) (rows int, in, out, net float64) {
	t.Helper()
	recs, err := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	parse := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	for _, r := range recs[1:] {
		switch {
		case len(r) > 7 && r[7] == "Total in":
			in = parse(r[8])
		case len(r) > 7 && r[7] == "Total out":
			out = parse(r[9])
		case len(r) > 7 && r[7] == "Net cash":
			net = parse(r[8])
		case len(r) > 1 && r[1] != "":
			rows++
		}
	}
	return
}

// Every view of a period's money — the summary, the ledger, the CSV — adds
// up to the same numbers; voided records show in the ledger and CSV but
// never count; a filter narrows the CSV exactly as it narrows the ledger.
func TestLedgerSummaryAndStatementReconcile(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	prev := models.ShiftMonth(month, -1)
	day := month + "-02"
	tr := e.addTrainee("Karim", 100)
	var p1, p2 paymentOut
	e.ok("POST", "/payments", map[string]any{"trainee": tr.ID, "amount": 60, "type": "subscription", "periodMonth": month, "day": day, "method": "cash"}, &p1)
	e.ok("POST", "/payments", map[string]any{"amount": 35.5, "type": "private", "day": day, "method": "card", "reference": "SLIP-77"}, &p2)
	e.ok("POST", "/payments", map[string]any{"amount": 20, "type": "dropin", "day": day}, nil) // method unspecified
	e.ok("POST", "/payments/"+p2.ID+"/void", map[string]any{"reason": "card declined"}, nil)
	it := e.addItem("Tee", 5, 20, 9)
	e.ok("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 2}, nil)
	e.ok("POST", "/expenses", map[string]any{"amount": 800, "category": "rent", "day": day}, nil)
	e.ok("POST", "/expenses", map[string]any{"amount": 45.25, "category": "supplies", "day": day}, nil)
	e.ok("POST", "/payments", map[string]any{"amount": 15, "type": "dropin", "day": prev + "-10"}, nil) // last month

	var fin finOut
	e.ok("GET", "/financials?m="+month, nil, &fin)
	// in: 60 + 20 + 40 (sale) = 120; the voided 35.50 card payment doesn't count
	if fin.Income != 120 || fin.Outgoings != 845.25 || fin.Net != -725.25 {
		t.Fatalf("summary = %+v", fin)
	}
	if fin.ByMethod["cash"] != 100 || fin.ByMethod["unspecified"] != 20 || fin.ByMethod["card"] != 0 {
		t.Fatalf("byMethod = %v (sale counts as cash; the voided card payment not at all)", fin.ByMethod)
	}
	if fin.ByType["subscription"] != 60 || fin.ByType["sale"] != 40 || fin.ByCategory["rent"] != 800 {
		t.Fatalf("breakdowns = %v / %v", fin.ByType, fin.ByCategory)
	}
	if fin.Previous.Income != 15 {
		t.Fatalf("previous month income = %v, want 15", fin.Previous.Income)
	}

	var led ledgerOut
	e.ok("GET", "/ledger?m="+month, nil, &led)
	if led.Total != 6 || led.Totals.Net != fin.Net || led.Totals.Income != fin.Income {
		t.Fatalf("ledger total rows %d, totals %+v; want 6 rows (voided one included) and the summary's totals", led.Total, led.Totals)
	}

	st, raw := e.req("GET", "/financials/export?m="+month, nil)
	if st != http.StatusOK {
		t.Fatalf("export: %d", st)
	}
	rows, in, out, net := csvTotals(t, raw)
	if rows != 6 || in != fin.Income || out != fin.Outgoings || net != fin.Net {
		t.Fatalf("csv rows=%d in=%v out=%v net=%v; want 6 / summary totals", rows, in, out, net)
	}
	if !strings.Contains(string(raw), "SLIP-77") || !strings.Contains(string(raw), "card declined") {
		t.Fatal("csv must carry method references and void reasons")
	}

	// Filtered: only rent. The CSV and the ledger agree.
	e.ok("GET", "/ledger?m="+month+"&kind=expense&type=rent", nil, &led)
	_, raw = e.req("GET", "/financials/export?m="+month+"&kind=expense&type=rent", nil)
	rows, _, out, _ = csvTotals(t, raw)
	if led.Total != 1 || rows != 1 || out != 800 || led.Totals.Outgoings != 800 {
		t.Fatalf("rent filter: ledger %d/%v, csv %d/%v", led.Total, led.Totals.Outgoings, rows, out)
	}
}

func TestCashClosingComparesCountedWithRecorded(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	day := month + "-01"
	if day > models.DateKey(nowFn()) {
		t.Skip("first of the month is in the future")
	}
	e.ok("POST", "/payments", map[string]any{"amount": 50, "type": "dropin", "day": day, "method": "cash"}, nil)
	e.ok("POST", "/payments", map[string]any{"amount": 30, "type": "dropin", "day": day, "method": "card"}, nil)
	e.ok("PUT", "/cash-closings/"+day, map[string]any{"counted": 45, "note": "one note short?"}, nil)
	e.fail("PUT", "/cash-closings/2099-01-01", map[string]any{"counted": 1}, http.StatusBadRequest, CodeValidation)
	var days []struct {
		Day        string   `json:"day"`
		Cash       float64  `json:"cash"`
		Card       float64  `json:"card"`
		Expected   float64  `json:"expected"`
		Counted    *float64 `json:"counted"`
		Difference *float64 `json:"difference"`
	}
	e.ok("GET", "/cash-closings?m="+month, nil, &days)
	if len(days) != 1 || days[0].Expected != 50 || days[0].Card != 30 || days[0].Counted == nil || *days[0].Difference != -5 {
		t.Fatalf("closings = %+v", days)
	}
}

func TestPaymentFiltersPagingAndReceipt(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	tr := e.addTrainee("Lara", 90)
	e.ok("POST", "/payments", map[string]any{"trainee": tr.ID, "amount": 30, "type": "subscription", "periodMonth": month, "method": "cash"}, nil)
	var last paymentOut
	e.ok("POST", "/payments", map[string]any{"trainee": tr.ID, "amount": 20, "type": "subscription", "periodMonth": month, "method": "bank_transfer", "reference": "TX-9"}, &last)
	e.ok("POST", "/payments", map[string]any{"trainee": tr.ID, "amount": 10, "type": "dropin"}, nil)
	e.fail("POST", "/payments", map[string]any{"amount": 10, "type": "dropin", "method": "cheque"}, http.StatusBadRequest, CodeValidation)

	var forPeriod []paymentOut
	e.ok("GET", "/payments?trainee="+tr.ID+"&periodMonth="+month, nil, &forPeriod)
	if len(forPeriod) != 2 {
		t.Fatalf("period filter: %d payments, want 2", len(forPeriod))
	}
	var pg struct {
		Items   []paymentOut `json:"items"`
		Total   int          `json:"total"`
		HasMore bool         `json:"hasMore"`
	}
	e.ok("GET", "/payments?trainee="+tr.ID+"&limit=2", nil, &pg)
	if len(pg.Items) != 2 || pg.Total != 3 || !pg.HasMore {
		t.Fatalf("page = %d items, total %d, hasMore %v", len(pg.Items), pg.Total, pg.HasMore)
	}
	e.fail("GET", "/payments", nil, http.StatusBadRequest, CodeValidation) // no unbounded download

	var rc struct {
		Number  string `json:"number"`
		Method  string `json:"method"`
		Void    bool   `json:"void"`
		CashDay string `json:"cashDay"`
		Studio  struct {
			Name string `json:"name"`
		} `json:"studioInfo"`
		Payment struct {
			Reference string `json:"reference"`
		} `json:"payment"`
		Charge struct {
			Remaining float64 `json:"remaining"`
		} `json:"charge"`
	}
	e.ok("GET", "/payments/"+last.ID+"/receipt", nil, &rc)
	if len(rc.Number) != 8 || rc.Method != "bank_transfer" || rc.Payment.Reference != "TX-9" || rc.Void || rc.Studio.Name == "" || rc.Charge.Remaining != 40 {
		t.Fatalf("receipt = %+v", rc)
	}
	e.ok("POST", "/payments/"+last.ID+"/void", map[string]any{"reason": "bounced"}, nil)
	e.ok("GET", "/payments/"+last.ID+"/receipt", nil, &rc)
	if !rc.Void {
		t.Fatal("a voided payment's receipt must say so")
	}
}

// A partial return lands in the cash month of the return; voiding the sale
// voids its returns with it, so nothing is subtracted twice.
func TestReturnsCountInTheirOwnMonthAndDieWithTheirSale(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	next := models.ShiftMonth(month, 1)
	nextStart, _, _ := models.MonthRange(next)
	it := e.addItem("Gloves", 5, 45, 28)
	var s saleOut
	e.ok("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 2}, &s)
	// A return dated next month (inserted directly; Phase 5 adds the endpoint).
	if _, err := e.store.Coll(models.CollReturns).InsertOne(e.ctx, models.SaleReturn{
		Sale: oid(t, s.ID), Item: oid(t, it.ID), ItemName: "Gloves", Qty: 1, Amount: 45,
		Date: nextStart.Add(36 * time.Hour), Reason: "wrong size", CreatedAt: nowFn(),
	}); err != nil {
		t.Fatal(err)
	}
	var fin finOut
	e.ok("GET", "/financials?m="+month, nil, &fin)
	if fin.Income != 90 {
		t.Fatalf("sale month income = %v, want 90", fin.Income)
	}
	e.ok("GET", "/financials?m="+next, nil, &fin)
	if fin.Income != -45 || fin.ByType["return"] != -45 {
		t.Fatalf("return month income = %v (%v), want -45", fin.Income, fin.ByType)
	}
	e.ok("POST", "/sales/"+s.ID+"/void", map[string]any{"reason": "never happened"}, nil)
	e.ok("GET", "/financials?m="+next, nil, &fin)
	if fin.Income != 0 {
		t.Fatalf("after voiding the sale, the return month = %v, want 0", fin.Income)
	}
}
