package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/fuzzy"
	"bronzeboxing/internal/models"
)

// The ledger is every money movement in a period — payments, shop sales,
// returns and expenses — in one list. The Financials summary, the on-screen
// ledger, the CSV statement and the monthly report all read it through
// loadLedger, so their totals can never disagree. Everything here is net
// CASH: money in minus recorded money out by cash date. It is not profit.
//
// Voided rows stay visible (marked) and never count. A sale that was also
// mirrored as a payment (legacy) counts once, as the payment. A return
// counts only while its sale is live.

func registerLedger(r fiber.Router, store *db.Store) {
	h := &ledgerHandler{store}
	r.Get("/financials", h.financials)
	r.Get("/financials/export", h.statement)
	r.Get("/ledger", h.ledger)
	r.Get("/cash-closings", h.closings)
	r.Put("/cash-closings/:day", h.saveClosing)
}

type ledgerHandler struct{ store *db.Store }

type ledgerRow struct {
	Kind        string    `json:"kind"` // payment | sale | return | expense
	ID          string    `json:"id"`
	Sale        string    `json:"sale,omitempty"` // the sale a return belongs to
	Date        time.Time `json:"date"`
	Day         string    `json:"day"`
	Detail      string    `json:"detail"`
	Type        string    `json:"type"` // payment type, "sale", "return", or expense category
	Method      string    `json:"method,omitempty"`
	Reference   string    `json:"reference,omitempty"`
	PeriodMonth string    `json:"periodMonth,omitempty"`
	Trainee     string    `json:"trainee,omitempty"`
	Note        string    `json:"note,omitempty"`
	In          float64   `json:"in"`
	Out         float64   `json:"out"`
	Voided      bool      `json:"voided"`
	VoidReason  string    `json:"voidReason,omitempty"`
	inCents     int64
	outCents    int64
}

type ledgerFilter struct {
	Kind    string // payment | sale | return | expense | income (payment+sale+return) | ""
	Type    string // payment type / expense category / "sale" / "return"
	Method  string
	Trainee string
	Voided  bool // include voided rows
	Query   fuzzy.Query
}

func filterFromQuery(c *fiber.Ctx) ledgerFilter {
	return ledgerFilter{
		Kind: c.Query("kind"), Type: c.Query("type"), Method: c.Query("method"), Trainee: c.Query("trainee"),
		Voided: c.Query("voided") != "0", Query: fuzzy.NewQuery(c.Query("q")),
	}
}

func (f ledgerFilter) keep(r ledgerRow) bool {
	switch f.Kind {
	case "":
	case "income":
		if r.Kind == "expense" {
			return false
		}
	default:
		if r.Kind != f.Kind {
			return false
		}
	}
	if f.Type != "" && r.Type != f.Type {
		return false
	}
	if f.Method != "" && methodKey(r) != f.Method {
		return false
	}
	if f.Trainee != "" && r.Trainee != f.Trainee {
		return false
	}
	if !f.Voided && r.Voided {
		return false
	}
	return f.Query.Empty() || f.score(r) > 0
}

// score ranks a row against the search: its description first, then note,
// reference, type and period.
func (f ledgerFilter) score(r ledgerRow) int {
	return fuzzy.Match(f.Query, fuzzy.NewTarget(r.Detail, r.Note, r.Reference, payTypeWord(r.Type), categoryWord(r.Type), r.PeriodMonth)).Score
}

// methodKey is how a row was paid, for the method split: payments carry it
// (legacy ones are "unspecified"); shop sales and returns default to cash.
func methodKey(r ledgerRow) string {
	switch {
	case r.Kind == "expense":
		return ""
	case r.Method != "":
		return r.Method
	case r.Kind == "sale" || r.Kind == "return":
		return models.MethodCash
	default:
		return "unspecified"
	}
}

// loadLedger reads every money row whose cash date is in [from, to).
func loadLedger(ctx context.Context, store *db.Store, from, to time.Time) ([]ledgerRow, error) {
	inRange := bson.M{"date": bson.M{"$gte": from, "$lt": to}}
	byDate := options.Find().SetSort(bson.D{{Key: "date", Value: 1}, {Key: "_id", Value: 1}})
	var rows []ledgerRow

	var payments []models.Payment
	if cur, err := store.Coll(models.CollPayments).Find(ctx, inRange, byDate); err != nil {
		return nil, err
	} else if err := cur.All(ctx, &payments); err != nil {
		return nil, err
	}
	for _, p := range payments {
		r := ledgerRow{
			Kind: "payment", ID: p.ID.Hex(), Date: p.Date, Detail: defaultStr(p.TraineeName, "—"), Type: p.Type,
			Method: p.Method, Reference: p.Reference, PeriodMonth: p.PeriodMonth, Note: p.Note,
			inCents: models.Cents(p.Amount), Voided: p.VoidedAt != nil, VoidReason: p.VoidReason,
		}
		if p.Trainee != nil {
			r.Trainee = p.Trainee.Hex()
		}
		rows = append(rows, r)
	}

	var sales []models.Sale
	if cur, err := store.Coll(models.CollSales).Find(ctx, inRange, byDate); err != nil {
		return nil, err
	} else if err := cur.All(ctx, &sales); err != nil {
		return nil, err
	}
	for _, s := range sales {
		if s.PaymentID != nil {
			continue // legacy mirror: already counted as its payment
		}
		detail := fmt.Sprintf("%s × %d", s.ItemName, s.Qty)
		if s.TraineeName != "" {
			detail += " → " + s.TraineeName
		}
		r := ledgerRow{
			Kind: "sale", ID: s.ID.Hex(), Date: s.Date, Detail: detail, Type: models.PaySale, Method: s.Method,
			inCents: models.Cents(s.Total), Voided: s.VoidedAt != nil, VoidReason: s.VoidReason,
		}
		if s.Trainee != nil {
			r.Trainee = s.Trainee.Hex()
		}
		rows = append(rows, r)
	}

	var returns []models.SaleReturn
	if cur, err := store.Coll(models.CollReturns).Find(ctx, inRange, byDate); err != nil {
		return nil, err
	} else if err := cur.All(ctx, &returns); err != nil {
		return nil, err
	}
	if len(returns) > 0 {
		saleIDs := make([]primitive.ObjectID, 0, len(returns))
		for _, r := range returns {
			saleIDs = append(saleIDs, r.Sale)
		}
		voidedSale := map[primitive.ObjectID]bool{}
		cur, err := store.Coll(models.CollSales).Find(ctx, bson.M{"_id": bson.M{"$in": saleIDs}, "voidedAt": bson.M{"$ne": nil}},
			options.Find().SetProjection(bson.M{"_id": 1}))
		if err != nil {
			return nil, err
		}
		var vs []models.Sale
		if err := cur.All(ctx, &vs); err != nil {
			return nil, err
		}
		for _, s := range vs {
			voidedSale[s.ID] = true
		}
		for _, rt := range returns {
			rows = append(rows, ledgerRow{
				Kind: "return", ID: rt.ID.Hex(), Sale: rt.Sale.Hex(), Date: rt.Date,
				Detail: fmt.Sprintf("%s × %d returned", rt.ItemName, rt.Qty), Type: "return", Note: rt.Reason,
				inCents: -models.Cents(rt.Amount), Voided: voidedSale[rt.Sale],
				VoidReason: map[bool]string{true: "sale voided"}[voidedSale[rt.Sale]],
			})
		}
	}

	var expenses []models.Expense
	if cur, err := store.Coll(models.CollExpenses).Find(ctx, inRange, byDate); err != nil {
		return nil, err
	} else if err := cur.All(ctx, &expenses); err != nil {
		return nil, err
	}
	for _, e := range expenses {
		rows = append(rows, ledgerRow{
			Kind: "expense", ID: e.ID.Hex(), Date: e.Date, Detail: e.Category, Type: e.Category, Note: e.Note,
			outCents: models.Cents(e.Amount), Voided: e.VoidedAt != nil, VoidReason: e.VoidReason,
		})
	}

	for i := range rows {
		rows[i].Day = models.DateKey(rows[i].Date)
		rows[i].In = models.Amount(rows[i].inCents)
		rows[i].Out = models.Amount(rows[i].outCents)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Date.Before(rows[j].Date) })
	return rows, nil
}

type cashTotals struct {
	Income    float64 `json:"income"`
	Outgoings float64 `json:"outgoings"`
	Net       float64 `json:"net"`
}

func totalsOf(rows []ledgerRow) cashTotals {
	var in, out int64
	for _, r := range rows {
		if r.Voided {
			continue
		}
		in += r.inCents
		out += r.outCents
	}
	return cashTotals{Income: models.Amount(in), Outgoings: models.Amount(out), Net: models.Amount(in - out)}
}

// previousWindow is the comparison period: the previous calendar month for
// a month, otherwise the same-length window ending where this one starts.
func previousWindow(c *fiber.Ctx, from, to time.Time) (time.Time, time.Time) {
	if m := c.Query("m"); m != "" {
		start, end, _ := models.MonthRange(models.ShiftMonth(m, -1))
		return start, end
	}
	return from.Add(-to.Sub(from)), from
}

type financialSummary struct {
	cashTotals
	ByType     map[string]float64 `json:"byType"`
	ByCategory map[string]float64 `json:"byCategory"`
	ByMethod   map[string]float64 `json:"byMethod"`
	Counts     map[string]int     `json:"counts"`
	From       time.Time          `json:"from"`
	To         time.Time          `json:"to"`
}

func summarize(rows []ledgerRow, from, to time.Time) financialSummary {
	byType, byCategory, byMethod := map[string]int64{}, map[string]int64{}, map[string]int64{}
	counts := map[string]int{}
	for _, r := range rows {
		if r.Voided {
			counts["voided"]++
			continue
		}
		counts[r.Kind]++
		if r.Kind == "expense" {
			byCategory[r.Type] += r.outCents
			continue
		}
		byType[r.Type] += r.inCents
		byMethod[methodKey(r)] += r.inCents
	}
	return financialSummary{
		cashTotals: totalsOf(rows), ByType: centsMap(byType), ByCategory: centsMap(byCategory),
		ByMethod: centsMap(byMethod), Counts: counts, From: from, To: to,
	}
}

func centsMap(m map[string]int64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = models.Amount(v)
	}
	return out
}

// financials reports net cash for a period (?m= or ?from=&to=) with
// breakdowns by payment type, expense category and payment method, and the
// previous period for comparison. Voided records never count.
func (h *ledgerHandler) financials(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to, err := monthOrRange(c)
	if err != nil {
		return err
	}
	rows, err := loadLedger(ctx, h.store, from, to)
	if err != nil {
		return err
	}
	pFrom, pTo := previousWindow(c, from, to)
	prevRows, err := loadLedger(ctx, h.store, pFrom, pTo)
	if err != nil {
		return err
	}
	s := summarize(rows, from, to)
	prev := totalsOf(prevRows)
	return c.JSON(fiber.Map{
		"income": s.Income, "outgoings": s.Outgoings, "net": s.Net,
		"byType": s.ByType, "byCategory": s.ByCategory, "byMethod": s.ByMethod, "counts": s.Counts,
		"from": from, "to": to,
		"previous": fiber.Map{"income": prev.Income, "outgoings": prev.Outgoings, "net": prev.Net, "from": pFrom, "to": pTo},
	})
}

// ledger lists the period's rows, newest first, narrowed by the same filters
// the statement export accepts; totals cover the filtered live rows.
func (h *ledgerHandler) ledger(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to, err := monthOrRange(c)
	if err != nil {
		return err
	}
	rows, err := loadLedger(ctx, h.store, from, to)
	if err != nil {
		return err
	}
	f := filterFromQuery(c)
	kept := make([]ledgerRow, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- { // newest first
		if f.keep(rows[i]) {
			kept = append(kept, rows[i])
		}
	}
	if !f.Query.Empty() { // a search lists the best matches first
		sc := make([]int, len(kept))
		for i := range kept {
			sc[i] = f.score(kept[i])
		}
		idx := make([]int, len(kept))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool { return sc[idx[a]] > sc[idx[b]] })
		ranked := make([]ledgerRow, len(kept))
		for i, j := range idx {
			ranked[i] = kept[j]
		}
		kept = ranked
	}
	limit := min(max(atoiDefault(c.Query("limit"), 50), 1), 500)
	offset := max(atoiDefault(c.Query("offset"), 0), 0)
	end := min(offset+limit, len(kept))
	pageRows := []ledgerRow{}
	if offset < len(kept) {
		pageRows = kept[offset:end]
	}
	return c.JSON(fiber.Map{
		"items": pageRows, "total": len(kept), "hasMore": end < len(kept), "offset": offset, "limit": limit,
		"totals": totalsOf(kept), "from": from, "to": to,
	})
}

// statement exports the period's ledger as CSV — exactly the rows the ledger
// shows for the same filters, oldest first, voided rows marked VOID and left
// out of the totals, followed by the totals.
func (h *ledgerHandler) statement(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to, err := monthOrRange(c)
	if err != nil {
		return err
	}
	rows, err := loadLedger(ctx, h.store, from, to)
	if err != nil {
		return err
	}
	f := filterFromQuery(c)
	cents := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Date", "Kind", "Detail", "Type", "Period", "Method", "Reference", "Note", "In", "Out", "Status", "Void reason"})
	var kept []ledgerRow
	for _, r := range rows {
		if !f.keep(r) {
			continue
		}
		kept = append(kept, r)
		status := ""
		if r.Voided {
			status = "VOID"
		}
		method := methodKey(r)
		_ = w.Write([]string{
			r.Date.In(time.Local).Format("2006-01-02 15:04"), r.Kind, r.Detail, r.Type, r.PeriodMonth,
			method, r.Reference, r.Note, cents(r.In), cents(r.Out), status, r.VoidReason,
		})
	}
	t := totalsOf(kept)
	_ = w.Write([]string{})
	_ = w.Write([]string{"", "", "", "", "", "", "", "Total in", cents(t.Income), "", "", ""})
	_ = w.Write([]string{"", "", "", "", "", "", "", "Total out", "", cents(t.Outgoings), "", ""})
	_ = w.Write([]string{"", "", "", "", "", "", "", "Net cash", cents(t.Net), "", "", ""})
	w.Flush()

	label := c.Query("m")
	if label == "" {
		label = models.DateKey(from) + "_" + models.DateKey(to.Add(-time.Nanosecond))
	}
	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=statement-%s.csv", label))
	return c.Send(buf.Bytes())
}

// ── Daily cash reconciliation ─────────────────────────────────────────────

type closingDay struct {
	Day string `json:"day"`
	// Money the books say came in that day, by how it was paid. Cash-in is
	// what should be in the till: cash payments, shop sales (cash unless the
	// sale says otherwise) less cash refunds, plus payments whose method was
	// never recorded (shown separately so the gap is visible).
	Cash        float64  `json:"cash"`
	Unspecified float64  `json:"unspecified"`
	Card        float64  `json:"card"`
	Transfer    float64  `json:"transfer"`
	Other       float64  `json:"other"`
	Expected    float64  `json:"expected"` // cash + unspecified
	Counted     *float64 `json:"counted,omitempty"`
	Difference  *float64 `json:"difference,omitempty"` // counted − expected
	Note        string   `json:"note,omitempty"`
	Actor       string   `json:"actor,omitempty"`
}

// closings lists each studio day in the period that had income or a count,
// newest first, comparing the recorded cash with the counted closing amount.
func (h *ledgerHandler) closings(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to, err := monthOrRange(c)
	if err != nil {
		return err
	}
	rows, err := loadLedger(ctx, h.store, from, to)
	if err != nil {
		return err
	}
	type acc struct{ cash, unspec, card, transfer, other int64 }
	days := map[string]*acc{}
	get := func(d string) *acc {
		if days[d] == nil {
			days[d] = &acc{}
		}
		return days[d]
	}
	for _, r := range rows {
		if r.Voided || r.Kind == "expense" {
			continue
		}
		a := get(r.Day)
		switch methodKey(r) {
		case models.MethodCash:
			a.cash += r.inCents
		case models.MethodCard:
			a.card += r.inCents
		case models.MethodTransfer:
			a.transfer += r.inCents
		case models.MethodOther:
			a.other += r.inCents
		default:
			a.unspec += r.inCents
		}
	}
	cur, err := h.store.Coll(models.CollClosings).Find(ctx, bson.M{"day": bson.M{"$gte": models.DateKey(from), "$lt": models.DateKey(to)}})
	if err != nil {
		return err
	}
	var closings []models.CashClosing
	if err := cur.All(ctx, &closings); err != nil {
		return err
	}
	byDay := map[string]models.CashClosing{}
	for _, cl := range closings {
		byDay[cl.Day] = cl
		get(cl.Day)
	}
	out := make([]closingDay, 0, len(days))
	for d, a := range days {
		cd := closingDay{
			Day: d, Cash: models.Amount(a.cash), Unspecified: models.Amount(a.unspec), Card: models.Amount(a.card),
			Transfer: models.Amount(a.transfer), Other: models.Amount(a.other), Expected: models.Amount(a.cash + a.unspec),
		}
		if cl, ok := byDay[d]; ok {
			counted := models.Amount(cl.CountedCents)
			diff := models.Amount(cl.CountedCents - (a.cash + a.unspec))
			cd.Counted, cd.Difference, cd.Note, cd.Actor = &counted, &diff, cl.Note, cl.Actor
		}
		out = append(out, cd)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day > out[j].Day })
	return c.JSON(out)
}

// saveClosing records (or corrects) the cash counted at the end of a day.
func (h *ledgerHandler) saveClosing(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	day := c.Params("day")
	if _, err := models.ParseDay(day); err != nil {
		return badField("day", "day must be YYYY-MM-DD")
	}
	if day > models.DateKey(time.Now()) {
		return badField("day", "you can't count a day that hasn't happened yet")
	}
	var in struct {
		Counted *float64 `json:"counted"`
		Note    string   `json:"note"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	if in.Counted == nil {
		return badField("counted", "counted is required")
	}
	counted, err := validMoney("counted", *in.Counted, true)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = h.store.Coll(models.CollClosings).UpdateOne(ctx, bson.M{"day": day}, bson.M{
		"$set":         bson.M{"countedCents": models.Cents(counted), "note": strings.TrimSpace(in.Note), "actor": actorOf(c), "updatedAt": now},
		"$setOnInsert": bson.M{"day": day, "createdAt": now},
	}, options.Update().SetUpsert(true))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true, "day": day, "counted": counted})
}
