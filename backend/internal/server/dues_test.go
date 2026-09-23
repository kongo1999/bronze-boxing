package server

import (
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bronzeboxing/internal/migrate"
	"bronzeboxing/internal/models"
)

func nowFn() time.Time { return time.Now() }

// ── Pure rules ────────────────────────────────────────────────────────────

func TestChargeState(t *testing.T) {
	cases := []struct {
		due, paid int64
		source    string
		want      string
	}{
		{5000, 0, models.ChargeSourceNormal, models.ChargeUnpaid},
		{5000, 2000, models.ChargeSourceNormal, models.ChargePartial},
		{5000, 5000, models.ChargeSourceNormal, models.ChargePaid},
		{0, 0, models.ChargeSourceAdjusted, models.ChargeWaived},
		{2000, 2000, models.ChargeSourceAdjusted, models.ChargePaid},
		{5000, 5000, models.ChargeSourceImported, models.ChargeUnverified},
	}
	for _, c := range cases {
		if got := chargeState(c.due, c.paid, c.source); got != c.want {
			t.Errorf("chargeState(%d,%d,%s) = %s, want %s", c.due, c.paid, c.source, got, c.want)
		}
	}
	if got := displayState(models.ChargeUnpaid, "2026-11", "2026-09"); got != models.ChargeUpcoming {
		t.Errorf("future unpaid = %s, want upcoming", got)
	}
	if got := displayState(models.ChargePartial, "2026-11", "2026-09"); got != models.ChargePartial {
		t.Errorf("future partial = %s, want partial", got)
	}
}

func TestTermResolution(t *testing.T) {
	at := func(s string) time.Time { tt, _ := time.Parse(time.RFC3339, s); return tt }
	terms := []models.SubscriptionTerm{
		{ID: primitive.NewObjectID(), EffectiveDate: "2026-01-10", BillingFromMonth: "2026-01", MonthlyFee: 100, Status: "active", CreatedAt: at("2026-01-10T10:00:00Z")},
		{ID: primitive.NewObjectID(), EffectiveDate: "2026-03-15", BillingFromMonth: "2026-04", MonthlyFee: 120, Status: "active", CreatedAt: at("2026-03-15T10:00:00Z")},
		{ID: primitive.NewObjectID(), EffectiveDate: "2026-06-20", BillingFromMonth: "2026-07", MonthlyFee: 120, Status: "inactive", CreatedAt: at("2026-06-20T10:00:00Z")},
		// Same-day correction of the June change: the later record wins.
		{ID: primitive.NewObjectID(), EffectiveDate: "2026-06-20", BillingFromMonth: "2026-07", MonthlyFee: 120, Status: "inactive", CreatedAt: at("2026-06-20T11:00:00Z"), Reason: "second"},
	}
	if billingTermFor(terms, "2025-12") != nil {
		t.Error("no term should bill before the first billing month")
	}
	if got := billingTermFor(terms, "2026-03"); got.MonthlyFee != 100 {
		t.Errorf("March fee = %v, want 100 (the April change hasn't started)", got.MonthlyFee)
	}
	if got := billingTermFor(terms, "2026-04"); got.MonthlyFee != 120 || !billable(got) {
		t.Errorf("April = %+v, want billable 120", got)
	}
	if got := billingTermFor(terms, "2026-06"); !billable(got) {
		t.Error("June is still billed: going inactive on the 20th bills from July")
	}
	if got := billingTermFor(terms, "2026-07"); billable(got) || got.Reason != "second" {
		t.Errorf("July = %+v, want the later inactive term", got)
	}
	if got := statusTermOn(terms, "2026-06-30"); got.Status != "inactive" {
		t.Error("inactive at the end of June")
	}
	if got := statusTermOn(terms, "2026-06-19"); got.Status != "active" {
		t.Error("still active on June 19")
	}
}

// ── Dues flows (integration) ──────────────────────────────────────────────

// The review's acceptance case: $50 due, $20 paid is partial with exactly $30
// remaining — never counted as paid — and becomes paid after $30 more.
func TestPartialPaymentLifecycle(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	tr := e.addTrainee("Karim", 50)

	row := e.dues(month)[tr.ID]
	if row.Due != 50 || row.State != models.ChargeUnpaid || row.Source != models.ChargeSourceNormal {
		t.Fatalf("new trainee row = %+v, want 50 unpaid", row)
	}
	first := e.pay(tr.ID, 20, month)
	row = e.dues(month)[tr.ID]
	if row.State != models.ChargePartial || row.AmountPaid != 20 || row.Remaining != 30 {
		t.Fatalf("after $20: %+v, want partial 20/50, 30 remaining", row)
	}
	var dash struct {
		PartialCount int `json:"partialCount"`
		UnpaidCount  int `json:"unpaidCount"`
	}
	e.ok("GET", "/dashboard", nil, &dash)
	if dash.PartialCount != 1 || dash.UnpaidCount != 0 {
		t.Fatalf("dashboard partial=%d unpaid=%d, want 1/0", dash.PartialCount, dash.UnpaidCount)
	}

	eb := e.fail("POST", "/payments", map[string]any{"trainee": tr.ID, "amount": 31, "type": "subscription", "periodMonth": month},
		http.StatusBadRequest, CodeOverpayment)
	if eb.Details["remaining"] != 30.0 {
		t.Fatalf("overpay details = %v, want remaining 30", eb.Details)
	}
	e.pay(tr.ID, 30, month)
	if row = e.dues(month)[tr.ID]; row.State != models.ChargePaid || row.Remaining != 0 {
		t.Fatalf("after $30 more: %+v, want paid", row)
	}

	// Voiding the first payment puts it back to partial ($30 of $50).
	e.ok("POST", "/payments/"+first.ID+"/void", map[string]any{"reason": "bounced"}, nil)
	if row = e.dues(month)[tr.ID]; row.State != models.ChargePartial || row.Remaining != 20 {
		t.Fatalf("after void: %+v, want partial with 20 remaining", row)
	}
	e.assertIntegrity()
}

// Editing a payment moves money between charges and back correctly.
func TestPaymentCorrectionRebalancesCharges(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	a := e.addTrainee("A", 50)
	b := e.addTrainee("B", 50)
	p := e.pay(a.ID, 50, month)

	// Reassign to B (same amount: no reason needed) — A back to unpaid, B paid.
	e.ok("PUT", "/payments/"+p.ID, map[string]any{"trainee": b.ID, "amount": 50, "type": "subscription", "periodMonth": month}, nil)
	d := e.dues(month)
	if d[a.ID].State != models.ChargeUnpaid || d[b.ID].State != models.ChargePaid {
		t.Fatalf("after reassign A=%s B=%s", d[a.ID].State, d[b.ID].State)
	}
	// Reduce the amount: needs a reason; then B is partial.
	e.fail("PUT", "/payments/"+p.ID, map[string]any{"trainee": b.ID, "amount": 20, "type": "subscription", "periodMonth": month},
		http.StatusBadRequest, CodeReasonRequired)
	e.ok("PUT", "/payments/"+p.ID, map[string]any{"trainee": b.ID, "amount": 20, "type": "subscription", "periodMonth": month, "reason": "typo"}, nil)
	if got := e.dues(month)[b.ID]; got.State != models.ChargePartial || got.Remaining != 30 {
		t.Fatalf("after reduce B = %+v", got)
	}
	// Reclassify to a drop-in: B's dues are untouched again.
	e.ok("PUT", "/payments/"+p.ID, map[string]any{"trainee": b.ID, "amount": 20, "type": "dropin"}, nil)
	if got := e.dues(month)[b.ID]; got.State != models.ChargeUnpaid {
		t.Fatalf("after reclassify B = %+v", got)
	}
	e.assertIntegrity()
}

// A fee change from next month never rewrites a month that is already
// charged; this month's charge changes only by an explicit, audited choice.
func TestFeeChangeKeepsIssuedMonthsFixed(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	next := models.ShiftMonth(month, 1)
	tr := e.addTrainee("Jad", 50)
	e.pay(tr.ID, 50, month)

	var upd traineeOut
	e.ok("PUT", "/trainees/"+tr.ID, map[string]any{"name": "Jad", "monthlyFee": 70, "status": "active", "termsApplyFrom": "next_month", "termsConfirm": true}, &upd)
	if upd.MonthlyFee != 70 || upd.FeeFromMonth != next {
		t.Fatalf("summary = %+v, want 70 from %s", upd, next)
	}
	if row := e.dues(month)[tr.ID]; row.Due != 50 || row.State != models.ChargePaid {
		t.Fatalf("this month after fee change: %+v, want still 50 paid", row)
	}
	if row := e.dues(next)[tr.ID]; row.Due != 70 || !row.Projected || row.State != models.ChargeUpcoming {
		t.Fatalf("next month: %+v, want projected 70 upcoming", row)
	}

	// Re-pricing this month's issued charge needs a reason, and is refused
	// below what was already paid.
	body := map[string]any{"name": "Jad", "monthlyFee": 40, "status": "active", "termsApplyFrom": "this_month", "termsConfirm": true}
	e.fail("PUT", "/trainees/"+tr.ID, body, http.StatusBadRequest, CodeReasonRequired)
	body["termsReason"] = "family discount"
	e.fail("PUT", "/trainees/"+tr.ID, body, http.StatusBadRequest, CodeDueBelowPaid)
	body["monthlyFee"] = 60
	e.ok("PUT", "/trainees/"+tr.ID, body, nil)
	row := e.dues(month)[tr.ID]
	if row.Due != 60 || row.State != models.ChargePartial || row.Source != models.ChargeSourceAdjusted {
		t.Fatalf("after this-month change: %+v, want adjusted 60 partial", row)
	}
	var trail []struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	e.ok("GET", "/audit/charge/"+row.ChargeID, nil, &trail)
	if len(trail) != 1 || trail[0].Action != "adjust" || trail[0].Reason != "family discount" {
		t.Fatalf("charge trail = %+v", trail)
	}
	e.assertIntegrity()
}

// Going inactive leaves this month's issued charge standing, keeps the
// trainee visible in that month's dues, and stops billing afterwards.
func TestInactiveTraineeKeepsIssuedDues(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	next := models.ShiftMonth(month, 1)
	tr := e.addTrainee("Omar", 40)
	e.ok("PUT", "/trainees/"+tr.ID, map[string]any{"name": "Omar", "monthlyFee": 40, "status": "inactive", "termsApplyFrom": "this_month", "termsConfirm": true}, nil)
	row, ok := e.dues(month)[tr.ID]
	if !ok || row.Due != 40 || row.State != models.ChargeUnpaid || row.Trainee.Status != "inactive" {
		t.Fatalf("this month: %+v (present %v), want the 40 charge still owed", row, ok)
	}
	if _, ok := e.dues(next)[tr.ID]; ok {
		t.Fatal("an inactive trainee must not be billed next month")
	}
}

// Collecting an older month's due today: the payment belongs to that period
// while the cash lands in today's month.
func TestLatePaymentForOlderPeriod(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	prev := models.ShiftMonth(month, -1)
	tr := e.addTrainee("Lara", 90)
	// The trainee's billing actually began last month (as if created then).
	if _, err := e.store.Coll(models.CollTerms).InsertOne(e.ctx, models.SubscriptionTerm{
		Trainee: oid(t, tr.ID), EffectiveDate: prev + "-01", BillingFromMonth: prev, MonthlyFee: 90, Status: "active",
		CreatedAt: nowFn().AddDate(0, -1, 0),
	}); err != nil {
		t.Fatal(err)
	}
	if row := e.dues(prev)[tr.ID]; row.Due != 90 || row.State != models.ChargeUnpaid {
		t.Fatalf("last month = %+v, want 90 unpaid", row)
	}
	p := e.pay(tr.ID, 90, prev)
	if models.MonthKey(parseTime(t, p.Date)) != month {
		t.Fatalf("cash date %s should be in %s", p.Date, month)
	}
	if row := e.dues(prev)[tr.ID]; row.State != models.ChargePaid {
		t.Fatalf("last month after late payment = %+v", row)
	}
	var fin struct {
		Income float64 `json:"income"`
	}
	e.ok("GET", "/financials?m="+month, nil, &fin)
	if fin.Income != 90 {
		t.Fatalf("this month's cash = %v, want 90", fin.Income)
	}
	e.ok("GET", "/financials?m="+prev, nil, &fin)
	if fin.Income != 0 {
		t.Fatalf("last month's cash = %v, want 0", fin.Income)
	}
}

func TestSubscriptionWithoutDuesIsRefused(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	free := e.addTrainee("Drop-in only", 0)
	e.fail("POST", "/payments", map[string]any{"trainee": free.ID, "amount": 20, "type": "subscription", "periodMonth": month},
		http.StatusBadRequest, CodeNoCharge)
	// Months before the trainee's first billing term have no dues either.
	paid := e.addTrainee("Paid", 50)
	e.fail("POST", "/payments", map[string]any{"trainee": paid.ID, "amount": 20, "type": "subscription", "periodMonth": models.ShiftMonth(month, -3)},
		http.StatusBadRequest, CodeNoCharge)
	// A prepayment for next month issues that month's charge.
	e.pay(paid.ID, 50, models.ShiftMonth(month, 1))
	if row := e.dues(models.ShiftMonth(month, 1))[paid.ID]; row.State != models.ChargePaid || row.Projected {
		t.Fatalf("prepaid month = %+v", row)
	}
}

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tt, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return tt
}

// The migration imports legacy data without inventing dues, flags what it
// can't know, and is a no-op the second time.
func TestTermsMigrationIsIdempotent(t *testing.T) {
	e := newEnv(t)
	now := nowFn()
	month := models.MonthKey(now)
	old := models.ShiftMonth(month, -2)
	ctx := e.ctx
	// Legacy-shaped data: trainees with no terms, payments with no charges.
	tA, tB, tC := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	_, _ = e.store.Coll(models.CollTrainees).InsertMany(ctx, []any{
		models.Trainee{ID: tA, Name: "Legacy A", MonthlyFee: 100, Status: "active", CreatedAt: now.AddDate(-1, 0, 0)},
		models.Trainee{ID: tB, Name: "Legacy B", MonthlyFee: 80, Status: "active", CreatedAt: now.AddDate(-1, 0, 0)},
		models.Trainee{ID: tC, Name: "Legacy C", MonthlyFee: 0, Status: "inactive", CreatedAt: now.AddDate(-1, 0, 0)},
	})
	pay := func(tid *primitive.ObjectID, amt float64, period string, date time.Time) models.Payment {
		return models.Payment{ID: primitive.NewObjectID(), Trainee: tid, Amount: amt, Type: "subscription", PeriodMonth: period, Date: date, CreatedAt: date}
	}
	_, _ = e.store.Coll(models.CollPayments).InsertMany(ctx, []any{
		pay(&tA, 100, old, now.AddDate(0, -2, 0)), // old month, paid
		pay(&tA, 40, month, now),                  // this month, partial
		pay(&tB, 50, old, now.AddDate(0, -2, 0)),  // old month, something paid
		pay(nil, 30, "", now),                     // unattributable legacy row
	})

	run := func(o migrate.Options) *migrate.Runner {
		r := &migrate.Runner{Store: e.store}
		if err := r.Run(ctx, Migrations(), o); err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := run(migrate.Options{Only: "2026-09-003-subscription-terms-charges", Rerun: true})
	if len(r.Notes) != 1 {
		t.Fatalf("notes = %v, want exactly the unattributable payment", r.Notes)
	}
	charges := func() int64 { return e.count(models.CollCharges, bson.M{}) }
	terms := func() int64 { return e.count(models.CollTerms, bson.M{}) }
	c1, t1 := charges(), terms()

	d := e.dues(month)
	if d[tA.Hex()].State != models.ChargePartial || d[tA.Hex()].Remaining != 60 {
		t.Fatalf("A this month = %+v, want partial 40/100", d[tA.Hex()])
	}
	if d[tB.Hex()].State != models.ChargeUnpaid || d[tB.Hex()].Due != 80 {
		t.Fatalf("B this month = %+v, want 80 unpaid", d[tB.Hex()])
	}
	if _, ok := d[tC.Hex()]; ok {
		t.Fatal("C has no fee and must not be charged")
	}
	o := e.dues(old)
	if o[tA.Hex()].State != models.ChargeUnverified || o[tB.Hex()].State != models.ChargeUnverified {
		t.Fatalf("old month rows = %+v, want unverified imports", o)
	}
	if n := e.count(models.CollCharges, bson.M{"periodMonth": models.ShiftMonth(month, -1)}); n != 0 {
		t.Fatalf("%d charges invented for a month with no payments", n)
	}
	// Unverified dues refuse new money until reconciled; reconciling works.
	e.fail("POST", "/payments", map[string]any{"trainee": tB.Hex(), "amount": 30, "type": "subscription", "periodMonth": old},
		http.StatusConflict, CodeUnverifiedDue)
	e.ok("POST", "/subscription-charges/"+o[tB.Hex()].ChargeID+"/adjust", map[string]any{"due": 80, "reason": "was 80 then"}, nil)
	e.pay(tB.Hex(), 30, old)
	if row := e.dues(old)[tB.Hex()]; row.State != models.ChargePaid || row.Source != models.ChargeSourceAdjusted {
		t.Fatalf("reconciled B old = %+v", row)
	}

	// Second run changes nothing.
	run(migrate.Options{Only: "2026-09-003-subscription-terms-charges", Rerun: true})
	if charges() != c1 || terms() != t1 {
		t.Fatalf("rerun changed counts: charges %d→%d terms %d→%d", c1, charges(), t1, terms())
	}
	e.assertIntegrity()
}
