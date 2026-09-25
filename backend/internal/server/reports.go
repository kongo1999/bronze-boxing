package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

// The monthly report: one aggregation, read inside a single snapshot
// transaction, serves the screen and the CSV — so the two can't disagree,
// and every figure reconciles to its source (the ledger for cash, the
// stored charges for dues, attendance records for classes).
//
// Two clocks, stated plainly: the Financial section is about cash by the
// day money moved; the Subscription section is about the fee period, so a
// late payment for August counts toward August's dues but in September's
// cash.

type reportFinancial struct {
	TraineePayments float64            `json:"traineePayments"` // cash from payments, every type
	ShopSales       float64            `json:"shopSales"`
	Refunds         float64            `json:"refunds"` // shop returns, as a positive amount
	ShopNet         float64            `json:"shopNet"`
	Income          float64            `json:"income"`
	Expenses        float64            `json:"expenses"`
	Net             float64            `json:"net"`
	ByType          map[string]float64 `json:"byType"`
	ByCategory      map[string]float64 `json:"byCategory"`
	ByMethod        map[string]float64 `json:"byMethod"`
	Counts          map[string]int     `json:"counts"`
	Previous        cashTotals         `json:"previous"`
	// Percent changes against the previous month; nil when it was zero.
	IncomeChangePct   *float64 `json:"incomeChangePct"`
	ExpensesChangePct *float64 `json:"expensesChangePct"`
	NetChangePct      *float64 `json:"netChangePct"`
}

type reportPayer struct {
	TraineeID string  `json:"traineeId"`
	Name      string  `json:"name"`
	Due       float64 `json:"due"`
	Paid      float64 `json:"paid"`
	Remaining float64 `json:"remaining"`
}

type reportSubs struct {
	Billed           int           `json:"billed"` // trainees with a verified charge (unverified imports excluded)
	TotalBilled      float64       `json:"totalBilled"`
	PaidTowardPeriod float64       `json:"paidTowardPeriod"` // at any cash date
	Outstanding      float64       `json:"outstanding"`
	Paid             int           `json:"paid"`
	Partial          int           `json:"partial"`
	Unpaid           int           `json:"unpaid"`
	Waived           int           `json:"waived"`
	Unverified       int           `json:"unverified"`
	UnverifiedPaid   float64       `json:"unverifiedPaid"`
	PartialPayers    []reportPayer `json:"partialPayers"`
	UnpaidPayers     []reportPayer `json:"unpaidPayers"`
}

type reportTrainees struct {
	ActiveAtMonthEnd  int `json:"activeAtMonthEnd"`  // from effective-dated terms
	UnknownAtMonthEnd int `json:"unknownAtMonthEnd"` // legacy status before dated history began
	Joined            int `json:"joined"`
	Inactivated       int `json:"inactivated"` // went inactive or were archived during the month
	Attended          int `json:"attended"`    // distinct people who attended at least once
}

type reportSeries struct {
	SeriesID  string `json:"seriesId"`
	Title     string `json:"title"`
	Completed int    `json:"completed"`
	Planned   int    `json:"planned"`
	InMonth   int    `json:"inMonth"`
}

type reportSessions struct {
	Total         int            `json:"total"` // occurrences starting in the month
	Scheduled     int            `json:"scheduled"`
	Completed     int            `json:"completed"`
	Cancelled     int            `json:"cancelled"`
	Group         int            `json:"group"`   // not cancelled
	Private       int            `json:"private"` // not cancelled
	Started       int            `json:"started"` // not cancelled and already begun
	NotMarkedDone int            `json:"notMarkedDone"`
	CompletionPct *float64       `json:"completionPct"` // completed / started
	SeriesCreated int            `json:"seriesCreated"`
	Series        []reportSeries `json:"series"`
	PlansActive   int            `json:"plansActive"`   // at month end
	PlansUnknown  int            `json:"plansUnknown"`  // no trustworthy historical state
	PlanCredits   int            `json:"planCredits"`   // plan sessions earned in the month
	PlanRemaining int            `json:"planRemaining"` // still owed on active plans at month end
}

type reportAttendance struct {
	Attended       int      `json:"attended"`
	NoShow         int      `json:"noShow"`
	Unmarked       int      `json:"unmarked"` // still "booked" on a class that has begun
	BookedAhead    int      `json:"bookedAhead"`
	People         int      `json:"people"`       // distinct attended
	NoShowPeople   int      `json:"noShowPeople"` // distinct with a no-show
	RatePct        *float64 `json:"ratePct"`      // attended / (attended + no-show)
	CappedClasses  int      `json:"cappedClasses"`
	CappedBooked   int      `json:"cappedBooked"`
	CappedCapacity int      `json:"cappedCapacity"`
	OccupancyPct   *float64 `json:"occupancyPct"` // booked / capacity, classes with a capacity only
}

type reportItem struct {
	ItemID  string  `json:"itemId"`
	Name    string  `json:"name"`
	Units   int     `json:"units"`
	Revenue float64 `json:"revenue"`
}

type reportStock struct {
	ItemID string `json:"itemId"`
	Name   string `json:"name"`
	// AtMonthEnd comes from the stock ledger; nil when tracking began later.
	AtMonthEnd *int `json:"atMonthEnd"`
	Now        int  `json:"now"`
}

type reportInventory struct {
	UnitsSold         int           `json:"unitsSold"`
	SalesRevenue      float64       `json:"salesRevenue"`
	LegacyLinkedSales int           `json:"legacyLinkedSales"` // sale cash is represented by a linked payment
	UnitsReturned     int           `json:"unitsReturned"`
	Refunds           float64       `json:"refunds"`
	TopByUnits        []reportItem  `json:"topByUnits"`
	TopByRevenue      []reportItem  `json:"topByRevenue"`
	LowNow            int           `json:"lowNow"` // current state, not historical
	OutNow            int           `json:"outNow"` // current state, not historical
	Stock             []reportStock `json:"stock"`
}

type monthlyReport struct {
	Month         string           `json:"month"`
	From          time.Time        `json:"from"`
	To            time.Time        `json:"to"`
	GeneratedAt   time.Time        `json:"generatedAt"`
	Timezone      string           `json:"timezone"`
	Partial       bool             `json:"partial"` // the month isn't over yet
	Financial     reportFinancial  `json:"financial"`
	Subscriptions reportSubs       `json:"subscriptions"`
	Trainees      reportTrainees   `json:"trainees"`
	Sessions      reportSessions   `json:"sessions"`
	Attendance    reportAttendance `json:"attendance"`
	Inventory     reportInventory  `json:"inventory"`
}

func registerReports(r fiber.Router, store *db.Store) {
	r.Get("/reports/monthly", func(c *fiber.Ctx) error {
		rep, err := reportFromQuery(c, store)
		if err != nil {
			return err
		}
		return c.JSON(rep)
	})
	r.Get("/reports/monthly/export", func(c *fiber.Ctx) error {
		rep, err := reportFromQuery(c, store)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
		c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="report-%s.csv"`, rep.Month))
		return c.Send(reportCSV(rep))
	})
}

func reportFromQuery(c *fiber.Ctx, store *db.Store) (*monthlyReport, error) {
	month := c.Query("m")
	if month == "" {
		month = models.MonthKey(time.Now())
	}
	if !models.ValidMonth(month) {
		return nil, badField("m", "m must be YYYY-MM")
	}
	if month > models.MonthKey(time.Now()) {
		return nil, badField("m", "a report can't be for a month that hasn't started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Issue the month's charges first (idempotent), then read everything
	// from one snapshot.
	if err := ensureChargesForMonth(ctx, store, month); err != nil {
		return nil, err
	}
	var rep *monthlyReport
	err := store.WithTx(ctx, func(tx context.Context) error {
		var err error
		rep, err = buildMonthlyReport(tx, store, month)
		return err
	})
	return rep, err
}

func buildMonthlyReport(ctx context.Context, store *db.Store, month string) (*monthlyReport, error) {
	from, to, _ := models.MonthRange(month)
	now := clockNow()
	rep := &monthlyReport{Month: month, From: from, To: to, GeneratedAt: time.Now().UTC(), Timezone: time.Local.String(), Partial: now.Before(to)}
	var err error
	if rep.Financial, err = reportFinancials(ctx, store, month, from, to); err != nil {
		return nil, err
	}
	if rep.Subscriptions, err = reportSubscriptions(ctx, store, month); err != nil {
		return nil, err
	}
	if err := reportPeople(ctx, store, rep, from, to, now); err != nil {
		return nil, err
	}
	if rep.Inventory, err = reportShop(ctx, store, from, to); err != nil {
		return nil, err
	}
	return rep, nil
}

func pct(num, den float64) *float64 {
	if den == 0 {
		return nil
	}
	v := float64(int64(num/den*1000+0.5)) / 10 // one decimal
	return &v
}

func changePct(cur, prev float64) *float64 {
	if prev == 0 {
		return nil
	}
	return pct(cur-prev, abs(prev))
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func reportFinancials(ctx context.Context, store *db.Store, month string, from, to time.Time) (reportFinancial, error) {
	rows, err := loadLedger(ctx, store, from, to)
	if err != nil {
		return reportFinancial{}, err
	}
	pFrom, pTo, _ := models.MonthRange(models.ShiftMonth(month, -1))
	prevRows, err := loadLedger(ctx, store, pFrom, pTo)
	if err != nil {
		return reportFinancial{}, err
	}
	s := summarize(rows, from, to)
	var pay, sales, refunds int64
	for _, r := range rows {
		if r.Voided {
			continue
		}
		switch r.Kind {
		case "payment":
			pay += r.inCents
		case "sale":
			sales += r.inCents
		case "return":
			refunds -= r.inCents
		}
	}
	prev := totalsOf(prevRows)
	return reportFinancial{
		TraineePayments: models.Amount(pay), ShopSales: models.Amount(sales), Refunds: models.Amount(refunds),
		ShopNet: models.Amount(sales - refunds), Income: s.Income, Expenses: s.Outgoings, Net: s.Net,
		ByType: s.ByType, ByCategory: s.ByCategory, ByMethod: s.ByMethod, Counts: s.Counts, Previous: prev,
		IncomeChangePct: changePct(s.Income, prev.Income), ExpensesChangePct: changePct(s.Outgoings, prev.Outgoings),
		NetChangePct: changePct(s.Net, prev.Net),
	}, nil
}

func reportSubscriptions(ctx context.Context, store *db.Store, month string) (reportSubs, error) {
	rows, err := listDues(ctx, store, month)
	if err != nil {
		return reportSubs{}, err
	}
	out := reportSubs{PartialPayers: []reportPayer{}, UnpaidPayers: []reportPayer{}}
	var billed, paid, outstanding, unverifiedPaid int64
	for _, r := range rows {
		if r.Projected {
			continue
		}
		payer := reportPayer{TraineeID: r.Trainee.ID.Hex(), Name: r.Trainee.Name, Due: r.Due, Paid: r.AmountPaid, Remaining: r.Remaining}
		if r.State == models.ChargeUnverified {
			out.Unverified++
			unverifiedPaid += models.Cents(r.AmountPaid)
			continue
		}
		out.Billed++
		billed += models.Cents(r.Due)
		paid += models.Cents(r.AmountPaid)
		switch r.State {
		case models.ChargePaid:
			out.Paid++
		case models.ChargePartial:
			out.Partial++
			outstanding += models.Cents(r.Remaining)
			out.PartialPayers = append(out.PartialPayers, payer)
		case models.ChargeUnpaid:
			out.Unpaid++
			outstanding += models.Cents(r.Remaining)
			out.UnpaidPayers = append(out.UnpaidPayers, payer)
		case models.ChargeWaived:
			out.Waived++
		}
	}
	out.TotalBilled, out.PaidTowardPeriod = models.Amount(billed), models.Amount(paid)
	out.Outstanding, out.UnverifiedPaid = models.Amount(outstanding), models.Amount(unverifiedPaid)
	byRemaining := func(p []reportPayer) {
		sort.SliceStable(p, func(i, j int) bool { return p[i].Remaining > p[j].Remaining })
	}
	byRemaining(out.PartialPayers)
	byRemaining(out.UnpaidPayers)
	return out, nil
}

// reportPeople fills the trainee, session and attendance sections from one
// read of the month's sessions.
func reportPeople(ctx context.Context, store *db.Store, rep *monthlyReport, from, to, now time.Time) error {
	asOf := to.Add(-time.Nanosecond)
	if now.Before(asOf) {
		asOf = now // the current month is a report so far, not a forecast
	}
	lastDay := models.DateKey(asOf)
	firstDay := models.DateKey(from)

	// Trainees: status at month end from the effective-dated terms — never
	// from today's profile.
	terms, err := loadTerms(ctx, store, bson.M{})
	if err != nil {
		return err
	}
	var trainees []models.Trainee
	if cur, err := store.Coll(models.CollTrainees).Find(ctx, bson.M{}); err != nil {
		return err
	} else if err := cur.All(ctx, &trainees); err != nil {
		return err
	}
	inactivated := map[primitive.ObjectID]bool{}
	for _, t := range trainees {
		ts := terms[t.ID]
		if st := statusTermOn(ts, lastDay); st != nil {
			// A migrated term copied today's status back to the join date. It
			// establishes status only from the migration day onward; older
			// months cannot honestly be called active or inactive.
			if st.Source == "migrated" && asOf.Before(st.CreatedAt) {
				rep.Trainees.UnknownAtMonthEnd++
			} else if st.Status == models.StatusActive {
				rep.Trainees.ActiveAtMonthEnd++
			}
		} else if !t.CreatedAt.After(asOf) {
			rep.Trainees.UnknownAtMonthEnd++
		}
		if !t.CreatedAt.Before(from) && t.CreatedAt.Before(to) {
			rep.Trainees.Joined++
		}
		if t.ArchivedAt != nil && !t.ArchivedAt.Before(from) && t.ArchivedAt.Before(to) {
			inactivated[t.ID] = true
		}
		// Went from active to inactive by a term effective this month.
		before := statusTermOn(ts, models.DateKey(from.Add(-time.Nanosecond)))
		after := statusTermOn(ts, lastDay)
		if before != nil && before.Status == models.StatusActive && after != nil && after.Status == models.StatusInactive &&
			after.EffectiveDate >= firstDay {
			inactivated[t.ID] = true
		}
	}
	rep.Trainees.Inactivated = len(inactivated)

	// Sessions starting in the month.
	var sessions []models.Session
	if cur, err := store.Coll(models.CollSessions).Find(ctx, bson.M{"start": bson.M{"$gte": from, "$lt": to}}); err != nil {
		return err
	} else if err := cur.All(ctx, &sessions); err != nil {
		return err
	}
	ss, at := &rep.Sessions, &rep.Attendance
	attended, noShow := map[primitive.ObjectID]bool{}, map[primitive.ObjectID]bool{}
	seriesInMonth := map[string]int{}
	var cappedBooked, cappedCap int
	for _, s := range sessions {
		ss.Total++
		switch s.Status {
		case models.SessScheduled:
			ss.Scheduled++
		case models.SessCompleted:
			ss.Completed++
		case models.SessCancelled:
			ss.Cancelled++
			continue // cancelled classes stay out of everything below
		}
		if s.Type == models.SessionPrivate {
			ss.Private++
		} else {
			ss.Group++
		}
		if s.SeriesID != "" {
			seriesInMonth[s.SeriesID]++
		}
		begun := !s.Start.After(now)
		if begun {
			ss.Started++
			if s.Status == models.SessScheduled {
				ss.NotMarkedDone++
			}
		}
		if s.Capacity > 0 {
			at.CappedClasses++
			cappedBooked += len(s.Attendees)
			cappedCap += s.Capacity
		}
		for _, a := range s.Attendees {
			switch a.Status {
			case models.AttendAttended:
				at.Attended++
				attended[a.Trainee] = true
			case models.AttendNoShow:
				at.NoShow++
				noShow[a.Trainee] = true
			default:
				if begun {
					at.Unmarked++
				} else {
					at.BookedAhead++
				}
			}
		}
	}
	ss.CompletionPct = pct(float64(ss.Completed), float64(ss.Started))
	at.People, at.NoShowPeople = len(attended), len(noShow)
	rep.Trainees.Attended = len(attended)
	at.RatePct = pct(float64(at.Attended), float64(at.Attended+at.NoShow))
	at.CappedBooked, at.CappedCapacity = cappedBooked, cappedCap
	at.OccupancyPct = pct(float64(cappedBooked), float64(cappedCap))

	// Series with classes this month, with their overall completed/planned.
	ids := make([]string, 0, len(seriesInMonth))
	for id := range seriesInMonth {
		ids = append(ids, id)
	}
	ss.Series = []reportSeries{}
	if len(ids) > 0 {
		var docs []models.SessionSeries
		if cur, err := store.Coll(models.CollSeries).Find(ctx, bson.M{"seriesId": bson.M{"$in": ids}}); err != nil {
			return err
		} else if err := cur.All(ctx, &docs); err != nil {
			return err
		}
		planned := map[string]models.SessionSeries{}
		for _, d := range docs {
			planned[d.SeriesID] = d
		}
		agg, err := store.Coll(models.CollSessions).Aggregate(ctx, mongo.Pipeline{
			{{Key: "$match", Value: bson.M{"seriesId": bson.M{"$in": ids}, "status": models.SessCompleted}}},
			{{Key: "$group", Value: bson.M{"_id": "$seriesId", "n": bson.M{"$sum": 1}, "title": bson.M{"$first": "$title"}}}},
		})
		if err != nil {
			return err
		}
		var done []struct {
			ID    string `bson:"_id"`
			N     int    `bson:"n"`
			Title string `bson:"title"`
		}
		if err := agg.All(ctx, &done); err != nil {
			return err
		}
		completed := map[string]int{}
		for _, d := range done {
			completed[d.ID] = d.N
		}
		for _, id := range ids {
			p := planned[id]
			title := p.Title
			if title == "" {
				for _, s := range sessions {
					if s.SeriesID == id {
						title = s.Title
						break
					}
				}
			}
			ss.Series = append(ss.Series, reportSeries{SeriesID: id, Title: title, Completed: completed[id], Planned: p.PlannedCount, InMonth: seriesInMonth[id]})
		}
		sort.Slice(ss.Series, func(i, j int) bool { return ss.Series[i].Title < ss.Series[j].Title })
	}
	n, err := store.Coll(models.CollSeries).CountDocuments(ctx, bson.M{"createdAt": bson.M{"$gte": from, "$lt": to}, "inferred": bson.M{"$ne": true}})
	if err != nil {
		return err
	}
	ss.SeriesCreated = int(n)

	// Plans: active at month end; credits earned in the month; what's still
	// owed on them at month end.
	var plans []models.SessionPlan
	if cur, err := store.Coll(models.CollPlans).Find(ctx, bson.M{}); err != nil {
		return err
	} else if err := cur.All(ctx, &plans); err != nil {
		return err
	}
	if len(plans) > 0 {
		states, err := planStatesAt(ctx, store, plans, asOf)
		if err != nil {
			return err
		}
		credits, err := planCreditsBefore(ctx, store, to)
		if err != nil {
			return err
		}
		inMonth, err := planCreditsBetween(ctx, store, from, to)
		if err != nil {
			return err
		}
		for _, current := range plans {
			p, known := states[current.ID]
			ss.PlanCredits += inMonth[current.ID]
			if !known {
				if !current.CreatedAt.After(asOf) {
					ss.PlansUnknown++
				}
				continue
			}
			activeAtEnd := p.StartDate <= lastDay && (p.EndDate == "" || p.EndDate >= lastDay) && p.Status == "active"
			if !activeAtEnd {
				continue
			}
			ss.PlansActive++
			if left := p.TargetCount - credits[p.ID]; left > 0 {
				ss.PlanRemaining += left
			}
		}
	}
	return nil
}

// planStatesAt reads the last audited plan state recorded by the report's
// as-of instant. A current document updated later cannot establish an older
// month's status, target or dates. Plans without audit history are usable only
// when their current document has not changed since the requested instant.
func planStatesAt(ctx context.Context, store *db.Store, plans []models.SessionPlan, asOf time.Time) (map[primitive.ObjectID]models.SessionPlan, error) {
	out := map[primitive.ObjectID]models.SessionPlan{}
	ids := make([]primitive.ObjectID, 0, len(plans))
	for _, p := range plans {
		ids = append(ids, p.ID)
		if !p.CreatedAt.After(asOf) && !p.UpdatedAt.After(asOf) {
			out[p.ID] = p
		}
	}
	cur, err := store.Coll(models.CollAudit).Find(ctx, bson.M{
		"entity": "plan", "ref": bson.M{"$in": ids}, "at": bson.M{"$lte": asOf},
	})
	if err != nil {
		return nil, err
	}
	var events []models.AuditEntry
	if err := cur.All(ctx, &events); err != nil {
		return nil, err
	}
	latest := map[primitive.ObjectID]time.Time{}
	for _, event := range events {
		if !event.At.After(latest[event.Ref]) || event.After == nil {
			continue
		}
		raw, err := bson.Marshal(event.After)
		if err != nil {
			return nil, err
		}
		var state models.SessionPlan
		if err := bson.Unmarshal(raw, &state); err != nil {
			return nil, err
		}
		out[event.Ref] = state
		latest[event.Ref] = event.At
	}
	return out, nil
}

// planCreditsBetween counts, per plan, linked bookings the trainee attended
// in completed classes starting in [from, to).
func planCreditsBetween(ctx context.Context, store *db.Store, from, to time.Time) (map[primitive.ObjectID]int, error) {
	return planCredits(ctx, store, bson.M{"$gte": from, "$lt": to})
}

func planCreditsBefore(ctx context.Context, store *db.Store, to time.Time) (map[primitive.ObjectID]int, error) {
	return planCredits(ctx, store, bson.M{"$lt": to})
}

func planCredits(ctx context.Context, store *db.Store, when bson.M) (map[primitive.ObjectID]int, error) {
	agg, err := store.Coll(models.CollSessions).Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"start": when, "status": models.SessCompleted, "attendees.planId": bson.M{"$exists": true}}}},
		{{Key: "$unwind", Value: "$attendees"}},
		{{Key: "$match", Value: bson.M{"attendees.status": models.AttendAttended, "attendees.planId": bson.M{"$ne": nil}}}},
		{{Key: "$group", Value: bson.M{"_id": "$attendees.planId", "n": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID primitive.ObjectID `bson:"_id"`
		N  int                `bson:"n"`
	}
	if err := agg.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := map[primitive.ObjectID]int{}
	for _, r := range rows {
		out[r.ID] = r.N
	}
	return out, nil
}

func reportShop(ctx context.Context, store *db.Store, from, to time.Time) (reportInventory, error) {
	out := reportInventory{TopByUnits: []reportItem{}, TopByRevenue: []reportItem{}, Stock: []reportStock{}}
	var sales []models.Sale
	if cur, err := store.Coll(models.CollSales).Find(ctx, bson.M{"date": bson.M{"$gte": from, "$lt": to}, "voidedAt": nil}); err != nil {
		return out, err
	} else if err := cur.All(ctx, &sales); err != nil {
		return out, err
	}
	byItem := map[primitive.ObjectID]*reportItem{}
	var revenue int64
	for _, s := range sales {
		if s.PaymentID != nil {
			out.LegacyLinkedSales++ // still a physical shop sale; cash is deduplicated in Financial
		}
		out.UnitsSold += s.Qty
		revenue += models.Cents(s.Total)
		it := byItem[s.Item]
		if it == nil {
			it = &reportItem{ItemID: s.Item.Hex(), Name: s.ItemName}
			byItem[s.Item] = it
		}
		it.Units += s.Qty
		it.Revenue = models.Amount(models.Cents(it.Revenue) + models.Cents(s.Total))
	}
	out.SalesRevenue = models.Amount(revenue)

	// Returns dated this month, for sales that still stand.
	var returns []models.SaleReturn
	if cur, err := store.Coll(models.CollReturns).Find(ctx, bson.M{"date": bson.M{"$gte": from, "$lt": to}}); err != nil {
		return out, err
	} else if err := cur.All(ctx, &returns); err != nil {
		return out, err
	}
	if len(returns) > 0 {
		saleIDs := make([]primitive.ObjectID, 0, len(returns))
		for _, r := range returns {
			saleIDs = append(saleIDs, r.Sale)
		}
		voided := map[primitive.ObjectID]bool{}
		var vs []models.Sale
		if cur, err := store.Coll(models.CollSales).Find(ctx, bson.M{"_id": bson.M{"$in": saleIDs}, "voidedAt": bson.M{"$ne": nil}}); err != nil {
			return out, err
		} else if err := cur.All(ctx, &vs); err != nil {
			return out, err
		}
		for _, s := range vs {
			voided[s.ID] = true
		}
		var refunds int64
		for _, r := range returns {
			if voided[r.Sale] {
				continue
			}
			out.UnitsReturned += r.Qty
			refunds += models.Cents(r.Amount)
		}
		out.Refunds = models.Amount(refunds)
	}

	items := make([]reportItem, 0, len(byItem))
	for _, it := range byItem {
		items = append(items, *it)
	}
	top := func(less func(a, b reportItem) bool) []reportItem {
		s := append([]reportItem{}, items...)
		sort.SliceStable(s, func(i, j int) bool {
			if less(s[i], s[j]) != less(s[j], s[i]) {
				return less(s[i], s[j])
			}
			return s[i].Name < s[j].Name
		})
		if len(s) > 5 {
			s = s[:5]
		}
		return s
	}
	out.TopByUnits = top(func(a, b reportItem) bool { return a.Units > b.Units })
	out.TopByRevenue = top(func(a, b reportItem) bool { return a.Revenue > b.Revenue })

	// Stock: today's shortages (labelled current), and each item's count at
	// month end from the stock ledger.
	var inv []models.InventoryItem
	if cur, err := store.Coll(models.CollInventory).Find(ctx, bson.M{}); err != nil {
		return out, err
	} else if err := cur.All(ctx, &inv); err != nil {
		return out, err
	}
	agg, err := store.Coll(models.CollMovements).Aggregate(ctx, mongo.Pipeline{
		{{Key: "$group", Value: bson.M{"_id": "$item",
			"before": bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$lt": bson.A{"$at", to}}, "$delta", 0}}},
			"first":  bson.M{"$min": "$at"}}}},
	})
	if err != nil {
		return out, err
	}
	var ledger []struct {
		ID     primitive.ObjectID `bson:"_id"`
		Before int                `bson:"before"`
		First  time.Time          `bson:"first"`
	}
	if err := agg.All(ctx, &ledger); err != nil {
		return out, err
	}
	atEnd := map[primitive.ObjectID]*int{}
	for _, l := range ledger {
		if l.First.Before(to) {
			v := l.Before
			atEnd[l.ID] = &v
		}
	}
	for _, it := range inv {
		switch it.Shortage() {
		case "out":
			out.OutNow++
		case "low":
			out.LowNow++
		}
		out.Stock = append(out.Stock, reportStock{ItemID: it.ID.Hex(), Name: it.Name, AtMonthEnd: atEnd[it.ID], Now: it.Stock})
	}
	sort.Slice(out.Stock, func(i, j int) bool { return out.Stock[i].Name < out.Stock[j].Name })
	return out, nil
}

// reportCSV is the same report as rows of section, metric, value.
func reportCSV(r *monthlyReport) []byte {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	row := func(section, metric string, value any) {
		var v string
		switch x := value.(type) {
		case float64:
			v = strconv.FormatFloat(x, 'f', 2, 64)
		case *float64:
			if x != nil {
				v = strconv.FormatFloat(*x, 'f', 1, 64) + "%"
			}
		case *int:
			if x != nil {
				v = strconv.Itoa(*x)
			} else {
				v = "unknown (before stock tracking began)"
			}
		default:
			v = fmt.Sprint(x)
		}
		_ = w.Write([]string{section, metric, v})
	}
	_ = w.Write([]string{"Section", "Metric", "Value"})
	row("Report", "Month", r.Month)
	row("Report", "Generated", r.GeneratedAt.Format(time.RFC3339))
	row("Report", "Timezone", r.Timezone)
	if r.Partial {
		row("Report", "Note", "month in progress — figures so far")
	}
	f := r.Financial
	row("Financial (cash by day received)", "Trainee payments", f.TraineePayments)
	row("Financial (cash by day received)", "Shop sales", f.ShopSales)
	row("Financial (cash by day received)", "Shop refunds", f.Refunds)
	row("Financial (cash by day received)", "Total income", f.Income)
	for _, k := range sortedKeys(f.ByCategory) {
		row("Financial (cash by day received)", "Expenses: "+categoryWord(k), f.ByCategory[k])
	}
	row("Financial (cash by day received)", "Total expenses", f.Expenses)
	row("Financial (cash by day received)", "Net cash", f.Net)
	for _, k := range sortedKeys(f.ByMethod) {
		row("Financial (cash by day received)", "Income by method: "+k, f.ByMethod[k])
	}
	row("Financial (cash by day received)", "Previous month income", f.Previous.Income)
	row("Financial (cash by day received)", "Previous month expenses", f.Previous.Outgoings)
	row("Financial (cash by day received)", "Previous month net", f.Previous.Net)
	row("Financial (cash by day received)", "Income change", f.IncomeChangePct)
	row("Financial (cash by day received)", "Net change", f.NetChangePct)
	s := r.Subscriptions
	const subs = "Subscriptions (fee period)"
	row(subs, "Trainees billed (verified)", s.Billed)
	row(subs, "Total billed", s.TotalBilled)
	row(subs, "Paid toward the period", s.PaidTowardPeriod)
	row(subs, "Outstanding", s.Outstanding)
	row(subs, "Fully paid", s.Paid)
	row(subs, "Partially paid", s.Partial)
	row(subs, "Unpaid", s.Unpaid)
	row(subs, "Waived", s.Waived)
	row(subs, "Unverified (imported, outside the totals)", s.Unverified)
	for _, p := range s.PartialPayers {
		row(subs, "Partial: "+p.Name, fmt.Sprintf("paid %.2f of %.2f, %.2f remaining", p.Paid, p.Due, p.Remaining))
	}
	t := r.Trainees
	row("Trainees", "Active at month end", t.ActiveAtMonthEnd)
	row("Trainees", "Status unknown at month end (before tracking began)", t.UnknownAtMonthEnd)
	row("Trainees", "Joined", t.Joined)
	row("Trainees", "Went inactive or archived", t.Inactivated)
	row("Trainees", "Attended at least once", t.Attended)
	ss := r.Sessions
	row("Sessions", "Classes in the month", ss.Total)
	row("Sessions", "Scheduled", ss.Scheduled)
	row("Sessions", "Completed", ss.Completed)
	row("Sessions", "Cancelled", ss.Cancelled)
	row("Sessions", "Group (not cancelled)", ss.Group)
	row("Sessions", "Private (not cancelled)", ss.Private)
	row("Sessions", "Begun, not marked done", ss.NotMarkedDone)
	row("Sessions", "Completion (completed / begun)", ss.CompletionPct)
	row("Sessions", "Series created", ss.SeriesCreated)
	for _, se := range ss.Series {
		row("Sessions", "Series: "+se.Title, fmt.Sprintf("%d/%d completed (%d this month)", se.Completed, se.Planned, se.InMonth))
	}
	row("Sessions", "Active plans at month end", ss.PlansActive)
	row("Sessions", "Plan status unknown at month end", ss.PlansUnknown)
	row("Sessions", "Plan sessions earned", ss.PlanCredits)
	row("Sessions", "Plan sessions still owed", ss.PlanRemaining)
	a := r.Attendance
	row("Attendance", "Attended", a.Attended)
	row("Attendance", "No-shows", a.NoShow)
	row("Attendance", "Not marked yet", a.Unmarked)
	row("Attendance", "Booked ahead", a.BookedAhead)
	row("Attendance", "People who attended", a.People)
	row("Attendance", "People with a no-show", a.NoShowPeople)
	row("Attendance", "Attendance rate (attended / decided)", a.RatePct)
	row("Attendance", fmt.Sprintf("Occupancy (%d classes with a capacity: %d booked of %d places)", a.CappedClasses, a.CappedBooked, a.CappedCapacity), a.OccupancyPct)
	inv := r.Inventory
	row("Shop", "Units sold", inv.UnitsSold)
	row("Shop", "Sales revenue", inv.SalesRevenue)
	row("Shop", "Sales linked to legacy payments", inv.LegacyLinkedSales)
	row("Shop", "Units returned", inv.UnitsReturned)
	row("Shop", "Refunds", inv.Refunds)
	for _, it := range inv.TopByUnits {
		row("Shop", "Top by units: "+it.Name, it.Units)
	}
	row("Shop", "Low stock now (current)", inv.LowNow)
	row("Shop", "Out of stock now (current)", inv.OutNow)
	for _, st := range inv.Stock {
		row("Shop", "Stock at month end: "+st.Name, st.AtMonthEnd)
	}
	w.Flush()
	return buf.Bytes()
}

func sortedKeys(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
