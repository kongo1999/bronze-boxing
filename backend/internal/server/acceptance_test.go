package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bronzeboxing/internal/models"
)

// Acceptance flows 1–4 from the implementation plan, end to end through the
// API. Flow 5 (stock under concurrency and failure) is in integrity_test.go
// and stock_test.go, flow 6 (9/12 series and plan counters) in
// schedule_test.go, flow 7 (search) in search_test.go and the Playwright
// suite, flow 8 (the monthly report) in reports_test.go.

// giveTermFrom lets a trainee owe dues from an earlier month (the API only
// ever starts billing from the current one).
func (e *testEnv) giveTermFrom(traineeID, month string, fee float64) {
	e.t.Helper()
	if _, err := e.store.Coll(models.CollTerms).InsertOne(e.ctx, models.SubscriptionTerm{
		ID: primitive.NewObjectID(), Trainee: oid(e.t, traineeID), EffectiveDate: month + "-01", BillingFromMonth: month,
		MonthlyFee: fee, Status: models.StatusActive, CreatedAt: time.Now().Add(-time.Hour),
	}); err != nil {
		e.t.Fatal(err)
	}
}

// 1. $50 fee, $20 paid: partial everywhere with $30 remaining, never among
// the paid; the remaining $30 makes it paid; a fee change next month leaves
// this month paid at its original amount.
func TestAcceptance1PartialThenPaid(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	tr := e.addTrainee("Maya", 50)
	e.pay(tr.ID, 20, month)

	row := e.dues(month)[tr.ID]
	if row.State != models.ChargePartial || row.AmountPaid != 20 || row.Remaining != 30 {
		t.Fatalf("roster = %+v", row)
	}
	var dash struct {
		PartialCount int `json:"partialCount"`
		UnpaidCount  int `json:"unpaidCount"`
	}
	e.ok("GET", "/dashboard", nil, &dash)
	if dash.PartialCount != 1 || dash.UnpaidCount != 0 {
		t.Fatalf("home counts = %+v", dash)
	}
	var charges []struct {
		State     string  `json:"state"`
		Remaining float64 `json:"remaining"`
	}
	e.ok("GET", "/subscription-charges?trainee="+tr.ID, nil, &charges)
	if len(charges) != 1 || charges[0].State != models.ChargePartial || charges[0].Remaining != 30 {
		t.Fatalf("profile dues = %+v", charges)
	}
	var rep reportOut
	e.ok("GET", "/reports/monthly?m="+month, nil, &rep)
	if rep.Subscriptions.Paid != 0 || len(rep.Subscriptions.PartialPayers) != 1 || rep.Subscriptions.PartialPayers[0].Remaining != 30 {
		t.Fatalf("report = %+v", rep.Subscriptions)
	}

	e.pay(tr.ID, 30, month)
	if row := e.dues(month)[tr.ID]; row.State != models.ChargePaid || row.Remaining != 0 {
		t.Fatalf("after the rest: %+v", row)
	}
	e.ok("GET", "/reports/monthly?m="+month, nil, &rep)
	if rep.Subscriptions.Paid != 1 || len(rep.Subscriptions.PartialPayers) != 0 {
		t.Fatalf("report after the rest = %+v", rep.Subscriptions)
	}

	e.ok("PUT", "/trainees/"+tr.ID, map[string]any{"monthlyFee": 70, "termsApplyFrom": "next_month", "termsConfirm": true}, nil)
	if row := e.dues(month)[tr.ID]; row.State != models.ChargePaid || row.Due != 50 {
		t.Fatalf("this month after a fee change: %+v", row)
	}
	if next := e.dues(models.ShiftMonth(month, 1))[tr.ID]; next.Due != 70 {
		t.Fatalf("next month = %+v, want due 70", next)
	}
}

// 2. An inactivated trainee keeps their dues, payments, attendance and
// purchases; voiding a payment needs a reason, reverses dues and cash, and
// is in the audit trail.
func TestAcceptance2InactivateKeepsHistoryAndVoidReverses(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	tr := e.addTrainee("Omar", 50)
	p := e.pay(tr.ID, 50, month)
	var s sessionOut
	e.ok("POST", "/sessions", map[string]any{"title": "Class", "type": "group", "start": time.Now().Add(-2 * time.Hour), "durationMin": 60,
		"attendees": []map[string]any{{"trainee": tr.ID}}}, &s)
	e.mark(s.ID, tr.ID, models.AttendAttended)
	it := e.addItem("Wraps", 5, 8, 4)
	e.ok("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 1, "trainee": tr.ID}, nil)

	e.ok("POST", "/trainees/"+tr.ID+"/archive", map[string]any{"reason": "moved away"}, nil)
	var pays []paymentOut
	e.ok("GET", "/payments?trainee="+tr.ID, nil, &pays)
	var sessions []sessionOut
	e.ok("GET", "/sessions?trainee="+tr.ID, nil, &sessions)
	var sales []saleOut
	e.ok("GET", "/sales?trainee="+tr.ID, nil, &sales)
	if len(pays) != 1 || len(sessions) != 1 || len(sales) != 1 || sessions[0].Attendees[0].Status != models.AttendAttended {
		t.Fatalf("history after archiving: %d payments, %d sessions, %d sales", len(pays), len(sessions), len(sales))
	}
	if row, ok := e.dues(month)[tr.ID]; !ok || row.State != models.ChargePaid {
		t.Fatalf("this month's dues after archiving: %+v", row)
	}

	var before finOut
	e.ok("GET", "/financials?m="+month, nil, &before)
	e.fail("POST", "/payments/"+p.ID+"/void", map[string]any{}, http.StatusBadRequest, CodeReasonRequired)
	e.ok("POST", "/payments/"+p.ID+"/void", map[string]any{"reason": "card declined"}, nil)
	var after finOut
	e.ok("GET", "/financials?m="+month, nil, &after)
	if row := e.dues(month)[tr.ID]; row.AmountPaid != 0 || row.State != models.ChargeUnpaid {
		t.Fatalf("dues after the void: %+v", row)
	}
	if before.Income-after.Income != 50 {
		t.Fatalf("cash moved by %v, want 50", before.Income-after.Income)
	}
	if n := e.count(models.CollAudit, bson.M{"entity": "payment", "action": "void", "reason": "card declined"}); n != 1 {
		t.Fatalf("audit lines for the void = %d", n)
	}
}

// 3. Collecting an older month's dues today: the dues belong to the old
// period, the cash to today; the receipt says both.
func TestAcceptance3OldPeriodCollectedToday(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	prev := models.ShiftMonth(month, -1)
	tr := e.addTrainee("Lina", 40)
	e.giveTermFrom(tr.ID, prev, 40)
	e.dues(prev)

	p := e.pay(tr.ID, 40, prev)
	if row := e.dues(prev)[tr.ID]; row.State != models.ChargePaid {
		t.Fatalf("old month = %+v", row)
	}
	var cur, old finOut
	e.ok("GET", "/financials?m="+month, nil, &cur)
	e.ok("GET", "/financials?m="+prev, nil, &old)
	if cur.Income != 40 || old.Income != 0 {
		t.Fatalf("cash: this month %v, old month %v", cur.Income, old.Income)
	}
	var rc struct {
		CashDay string `json:"cashDay"`
		Charge  struct {
			PeriodMonth string `json:"periodMonth"`
		} `json:"charge"`
	}
	e.ok("GET", "/payments/"+p.ID+"/receipt", nil, &rc)
	if rc.Charge.PeriodMonth != prev || rc.CashDay != models.DateKey(time.Now()) {
		t.Fatalf("receipt = %+v", rc)
	}
}

// 4. An expense dated in a past month lands in that month and its CSV;
// the statement totals equal the sum of its non-void rows.
func TestAcceptance4PastExpenseAndStatement(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	prev := models.ShiftMonth(month, -1)
	e.ok("POST", "/expenses", map[string]any{"amount": 120, "category": "equipment", "note": "pads", "day": prev + "-10"}, nil)
	var ex struct {
		ID string `json:"id"`
	}
	e.ok("POST", "/expenses", map[string]any{"amount": 99, "category": "rent", "day": prev + "-12"}, &ex)
	e.ok("POST", "/expenses/"+ex.ID+"/void", map[string]any{"reason": "duplicate"}, nil)

	var old, cur finOut
	e.ok("GET", "/financials?m="+prev, nil, &old)
	e.ok("GET", "/financials?m="+month, nil, &cur)
	if old.Outgoings != 120 || cur.Outgoings != 0 {
		t.Fatalf("outgoings: past %v, this month %v", old.Outgoings, cur.Outgoings)
	}
	st, raw := e.req("GET", "/financials/export?m="+prev, nil)
	csv := string(raw)
	if st != http.StatusOK || !strings.Contains(csv, "pads") || !strings.Contains(csv, "VOID") || !strings.Contains(csv, "120.00") {
		t.Fatalf("statement (%d):\n%s", st, csv)
	}
	if strings.Contains(csv, "219.00") {
		t.Fatalf("the void was counted in the totals:\n%s", csv)
	}
}
