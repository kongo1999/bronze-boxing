package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"bronzeboxing/internal/models"
)

type reportOut struct {
	Month     string `json:"month"`
	Financial struct {
		TraineePayments float64            `json:"traineePayments"`
		ShopSales       float64            `json:"shopSales"`
		Refunds         float64            `json:"refunds"`
		Income          float64            `json:"income"`
		Expenses        float64            `json:"expenses"`
		Net             float64            `json:"net"`
		ByCategory      map[string]float64 `json:"byCategory"`
	} `json:"financial"`
	Subscriptions struct {
		Billed           int     `json:"billed"`
		TotalBilled      float64 `json:"totalBilled"`
		PaidTowardPeriod float64 `json:"paidTowardPeriod"`
		Outstanding      float64 `json:"outstanding"`
		Paid             int     `json:"paid"`
		Partial          int     `json:"partial"`
		Unpaid           int     `json:"unpaid"`
		PartialPayers    []struct {
			Name      string  `json:"name"`
			Paid      float64 `json:"paid"`
			Remaining float64 `json:"remaining"`
		} `json:"partialPayers"`
	} `json:"subscriptions"`
	Trainees struct {
		ActiveAtMonthEnd int `json:"activeAtMonthEnd"`
		Joined           int `json:"joined"`
		Attended         int `json:"attended"`
	} `json:"trainees"`
	Sessions struct {
		Total         int      `json:"total"`
		Completed     int      `json:"completed"`
		Cancelled     int      `json:"cancelled"`
		Group         int      `json:"group"`
		Private       int      `json:"private"`
		CompletionPct *float64 `json:"completionPct"`
		Series        []struct {
			InMonth int `json:"inMonth"`
			Planned int `json:"planned"`
		} `json:"series"`
	} `json:"sessions"`
	Attendance struct {
		Attended     int      `json:"attended"`
		NoShow       int      `json:"noShow"`
		Unmarked     int      `json:"unmarked"`
		People       int      `json:"people"`
		NoShowPeople int      `json:"noShowPeople"`
		RatePct      *float64 `json:"ratePct"`
		OccupancyPct *float64 `json:"occupancyPct"`
	} `json:"attendance"`
	Inventory struct {
		UnitsSold     int     `json:"unitsSold"`
		SalesRevenue  float64 `json:"salesRevenue"`
		UnitsReturned int     `json:"unitsReturned"`
		Stock         []struct {
			Name       string `json:"name"`
			AtMonthEnd *int   `json:"atMonthEnd"`
		} `json:"stock"`
	} `json:"inventory"`
}

// The review's report scenario: partial / paid / unpaid subscribers, a late
// payment for an older period, voids, a partial return, classes attended,
// missed, left unmarked and cancelled, one trainee attending twice — and
// every total reconciles to its source.
func TestMonthlyReportReconciles(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	prev := models.ShiftMonth(month, -1)
	start, _, _ := models.MonthRange(month)
	day := func(d, h int) time.Time { return time.Date(start.Year(), start.Month(), d, h, 0, 0, 0, time.Local) }

	a := e.addTrainee("Adam Partial", 50)
	b := e.addTrainee("Bilal Paid", 50)
	e.addTrainee("Chadi Unpaid", 50)
	// Dana owes last month too: a term billing from the previous month.
	d := e.addTrainee("Dana Late", 40)
	if _, err := e.store.Coll(models.CollTerms).InsertOne(e.ctx, models.SubscriptionTerm{
		ID: primitive.NewObjectID(), Trainee: oid(t, d.ID), EffectiveDate: prev + "-01", BillingFromMonth: prev,
		MonthlyFee: 40, Status: models.StatusActive, CreatedAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	e.dues(prev) // issue last month's charge

	e.pay(a.ID, 20, month)
	e.pay(b.ID, 50, month)
	e.pay(d.ID, 40, month)
	e.pay(d.ID, 40, prev) // late: today's cash, last month's dues
	oops := e.pay(a.ID, 10, month)
	e.ok("POST", "/payments/"+oops.ID+"/void", map[string]any{"reason": "entered twice"}, nil)

	e.ok("POST", "/expenses", map[string]any{"amount": 30, "category": "supplies", "note": "tape"}, nil)
	var ex struct {
		ID string `json:"id"`
	}
	e.ok("POST", "/expenses", map[string]any{"amount": 99, "category": "rent"}, &ex)
	e.ok("POST", "/expenses/"+ex.ID+"/void", map[string]any{"reason": "wrong month"}, nil)

	it := e.addItem("Gloves", 5, 10, 6)
	var sale saleOut
	e.ok("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 3}, &sale)
	e.ok("POST", "/sales/"+sale.ID+"/returns", map[string]any{"qty": 1, "reason": "size"}, nil)
	var bad saleOut
	e.ok("POST", "/inventory/"+it.ID+"/sell", map[string]any{"qty": 1}, &bad)
	e.ok("POST", "/sales/"+bad.ID+"/void", map[string]any{"reason": "mistake"}, nil)

	session := func(title, typ string, at time.Time, capacity int, who ...string) string {
		att := []map[string]any{}
		for _, w := range who {
			att = append(att, map[string]any{"trainee": w})
		}
		var s sessionOut
		e.ok("POST", "/sessions", map[string]any{"title": title, "type": typ, "start": at, "durationMin": 60, "capacity": capacity, "attendees": att}, &s)
		return s.ID
	}
	s1 := session("Monday class", "group", day(1, 18), 4, a.ID, b.ID)
	e.setStatus(s1, models.SessCompleted)
	e.mark(s1, a.ID, models.AttendAttended)
	e.mark(s1, b.ID, models.AttendAttended)
	s2 := session("PT Adam", "private", day(2, 10), 0, a.ID, d.ID)
	e.setStatus(s2, models.SessCompleted)
	e.mark(s2, a.ID, models.AttendAttended) // Adam's second class: counts once among people
	e.mark(s2, d.ID, models.AttendNoShow)
	s3 := session("Wednesday class", "group", day(3, 18), 0, b.ID)
	e.setStatus(s3, models.SessCancelled)
	s4 := session("Thursday class", "group", day(4, 18), 0, d.ID)
	e.setStatus(s4, models.SessCompleted) // Dana left "booked": unmarked

	var rep reportOut
	e.ok("GET", "/reports/monthly?m="+month, nil, &rep)

	// Financial reconciles to the ledger summary for the same month.
	var fin finOut
	e.ok("GET", "/financials?m="+month, nil, &fin)
	f := rep.Financial
	if f.Income != fin.Income || f.Expenses != fin.Outgoings || f.Net != fin.Net {
		t.Fatalf("report %v/%v/%v vs financials %v/%v/%v", f.Income, f.Expenses, f.Net, fin.Income, fin.Outgoings, fin.Net)
	}
	if f.TraineePayments != 150 || f.ShopSales != 30 || f.Refunds != 10 || f.Income != 170 || f.Expenses != 30 || f.Net != 140 {
		t.Fatalf("financial = %+v", f)
	}

	// Subscriptions: the fee period, reconciled to the dues roster.
	s := rep.Subscriptions
	if s.Billed != 4 || s.Paid != 2 || s.Partial != 1 || s.Unpaid != 1 || s.TotalBilled != 190 || s.PaidTowardPeriod != 110 || s.Outstanding != 80 {
		t.Fatalf("subscriptions = %+v", s)
	}
	if len(s.PartialPayers) != 1 || s.PartialPayers[0].Name != "Adam Partial" || s.PartialPayers[0].Paid != 20 || s.PartialPayers[0].Remaining != 30 {
		t.Fatalf("partial payers = %+v", s.PartialPayers)
	}
	var paid float64
	for _, r := range e.dues(month) {
		paid += r.AmountPaid
	}
	if paid != s.PaidTowardPeriod {
		t.Fatalf("dues roster paid %v vs report %v", paid, s.PaidTowardPeriod)
	}
	// Last month: Dana's late payment made it paid, though the cash is this month's.
	var last reportOut
	e.ok("GET", "/reports/monthly?m="+prev, nil, &last)
	if last.Subscriptions.Paid != 1 || last.Subscriptions.PaidTowardPeriod != 40 || last.Financial.TraineePayments != 0 {
		t.Fatalf("previous month = subs %+v fin %+v", last.Subscriptions, last.Financial)
	}

	// Sessions and attendance.
	ss := rep.Sessions
	if ss.Total != 4 || ss.Completed != 3 || ss.Cancelled != 1 || ss.Group != 2 || ss.Private != 1 {
		t.Fatalf("sessions = %+v", ss)
	}
	if ss.CompletionPct == nil || *ss.CompletionPct != 100 {
		t.Fatalf("completion = %v", ss.CompletionPct)
	}
	at := rep.Attendance
	if at.Attended != 3 || at.NoShow != 1 || at.Unmarked != 1 || at.People != 2 || at.NoShowPeople != 1 {
		t.Fatalf("attendance = %+v", at)
	}
	if at.RatePct == nil || *at.RatePct != 75 || at.OccupancyPct == nil || *at.OccupancyPct != 50 {
		t.Fatalf("rate %v occupancy %v", at.RatePct, at.OccupancyPct)
	}
	if rep.Trainees.Attended != 2 || rep.Trainees.Joined != 4 || rep.Trainees.ActiveAtMonthEnd != 4 {
		t.Fatalf("trainees = %+v", rep.Trainees)
	}

	// Shop.
	inv := rep.Inventory
	if inv.UnitsSold != 3 || inv.SalesRevenue != 30 || inv.UnitsReturned != 1 {
		t.Fatalf("inventory = %+v", inv)
	}
	if len(inv.Stock) != 1 || inv.Stock[0].AtMonthEnd == nil || *inv.Stock[0].AtMonthEnd != 3 {
		t.Fatalf("stock at month end = %+v", inv.Stock)
	}

	// The CSV is the same numbers.
	st, raw := e.req("GET", "/reports/monthly/export?m="+month, nil)
	if st != http.StatusOK || !strings.Contains(string(raw), "Net cash,140.00") || !strings.Contains(string(raw), "Partial: Adam Partial") {
		t.Fatalf("csv (%d):\n%s", st, raw)
	}
	e.fail("GET", "/reports/monthly?m="+models.ShiftMonth(month, 1), nil, http.StatusBadRequest, CodeValidation)
}

// A month is its studio days, even across the autumn clock change: money
// and classes just inside either edge count, the moment after midnight on
// the 1st belongs to the next month.
func TestMonthlyReportUsesStudioDaysAcrossDST(t *testing.T) {
	e := newEnv(t)
	at := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, time.Local) }
	pay := func(when time.Time, amount float64) {
		if _, err := e.store.Coll(models.CollPayments).InsertOne(e.ctx, models.Payment{
			ID: primitive.NewObjectID(), Amount: amount, Type: models.PayDropin, Date: when, CreatedAt: when,
		}); err != nil {
			t.Fatal(err)
		}
	}
	pay(at(2025, 10, 1, 0, 30), 5)    // 2025-09-30 21:30 UTC — still October in Beirut
	pay(at(2025, 10, 26, 0, 30), 7)   // the night the clocks go back
	pay(at(2025, 10, 31, 23, 30), 11) // last half hour of October
	pay(at(2025, 11, 1, 0, 30), 100)  // 2025-10-31 22:30 UTC — November in Beirut
	if _, err := e.store.Coll(models.CollSessions).InsertOne(e.ctx, models.Session{
		ID: primitive.NewObjectID(), Title: "Midnight", Type: models.SessionGroup, Start: at(2025, 10, 1, 0, 30),
		DurationMin: 60, Status: models.SessScheduled, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	var rep reportOut
	e.ok("GET", "/reports/monthly?m=2025-10", nil, &rep)
	if rep.Financial.TraineePayments != 23 || rep.Sessions.Total != 1 {
		t.Fatalf("October 2025: payments %v (want 23), sessions %d (want 1)", rep.Financial.TraineePayments, rep.Sessions.Total)
	}
}
