package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

type expenseHandler struct{ store *db.Store }

func registerExpenses(r fiber.Router, store *db.Store) {
	h := &expenseHandler{store}
	g := r.Group("/expenses")
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Put("/:id", h.update)
	g.Post("/:id/void", h.voidPost)
	g.Delete("/:id", h.remove)
	r.Get("/financials", h.financials)
	r.Get("/financials/export", h.statement)
}

var expenseCategories = []string{
	models.ExpRent, models.ExpEquipment, models.ExpUtilities, models.ExpSupplies, models.ExpWages, models.ExpOther,
}

type expenseInput struct {
	Amount   float64    `json:"amount"`
	Category string     `json:"category"`
	Note     string     `json:"note"`
	Date     *time.Time `json:"date"`
	Day      string     `json:"day"` // studio-local YYYY-MM-DD the money went out
	Reason   string     `json:"reason"`
}

func (h *expenseHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to, err := monthOrRange(c)
	if err != nil {
		return err
	}
	cur, err := h.store.Coll(models.CollExpenses).Find(ctx,
		bson.M{"date": bson.M{"$gte": from, "$lt": to}},
		options.Find().SetSort(bson.D{{Key: "date", Value: -1}, {Key: "_id", Value: -1}}))
	if err != nil {
		return err
	}
	out := []models.Expense{}
	if err := cur.All(ctx, &out); err != nil {
		return err
	}
	return c.JSON(out)
}

func findExpense(ctx context.Context, store *db.Store, id primitive.ObjectID) (models.Expense, error) {
	var e models.Expense
	if err := store.Coll(models.CollExpenses).FindOne(ctx, bson.M{"_id": id}).Decode(&e); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return e, notFound("expense")
		}
		return e, err
	}
	return e, nil
}

func normalizeExpense(in expenseInput) (models.Expense, error) {
	var e models.Expense
	amount, err := validMoney("amount", in.Amount, false)
	if err != nil {
		return e, err
	}
	cat := defaultStr(strings.TrimSpace(in.Category), models.ExpOther)
	if err := oneOf("category", cat, expenseCategories...); err != nil {
		return e, err
	}
	e.Amount, e.Category, e.Note = amount, cat, strings.TrimSpace(in.Note)
	return e, nil
}

func (h *expenseHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in expenseInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	e, err := normalizeExpense(in)
	if err != nil {
		return err
	}
	if e.Date, err = resolveRecordDate(in.Date, in.Day); err != nil {
		return err
	}
	actor := actorOf(c)
	e.ID, e.CreatedAt, e.CreatedBy = primitive.NewObjectID(), time.Now(), actor
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		if _, err := h.store.Coll(models.CollExpenses).InsertOne(tx, e); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "expense", Ref: e.ID, Action: "create", After: e, Actor: actor})
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(e)
}

func (h *expenseHandler) update(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in expenseInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	next, err := normalizeExpense(in)
	if err != nil {
		return err
	}
	prev, err := findExpense(ctx, h.store, id)
	if err != nil {
		return err
	}
	if prev.VoidedAt != nil {
		return apiErr(http.StatusConflict, CodeVoidedLocked, "expense is voided and can no longer be edited")
	}
	next.Date = prev.Date
	if in.Date != nil || in.Day != "" {
		if next.Date, err = resolveRecordDate(in.Date, in.Day); err != nil {
			return err
		}
	}
	reason := strings.TrimSpace(in.Reason)
	if models.Cents(next.Amount) != models.Cents(prev.Amount) {
		if reason, err = requireReason(in.Reason, "change an expense's amount"); err != nil {
			return err
		}
	}
	actor := actorOf(c)
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		live, err := findExpense(tx, h.store, id)
		if err != nil {
			return err
		}
		res, err := h.store.Coll(models.CollExpenses).UpdateOne(tx, bson.M{"_id": id, "voidedAt": nil}, bson.M{"$set": bson.M{
			"amount": next.Amount, "category": next.Category, "note": next.Note, "date": next.Date,
		}})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return apiErr(http.StatusConflict, CodeVoidedLocked, "expense is voided and can no longer be edited")
		}
		after := live
		after.Amount, after.Category, after.Note, after.Date = next.Amount, next.Category, next.Note, next.Date
		return writeAudit(tx, h.store, auditLine{Entity: "expense", Ref: id, Action: "update",
			Before: live, After: after, Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	e, err := findExpense(ctx, h.store, id)
	if err != nil {
		return err
	}
	return c.JSON(e)
}

func (h *expenseHandler) remove(c *fiber.Ctx) error {
	return h.doVoid(c, c.Query("reason"))
}

func (h *expenseHandler) voidPost(c *fiber.Ctx) error {
	var in struct {
		Reason string `json:"reason"`
	}
	_ = c.BodyParser(&in)
	return h.doVoid(c, in.Reason)
}

// doVoid voids an expense rather than deleting it — the record stays in the
// books, marked void, and stops counting toward outgoings.
func (h *expenseHandler) doVoid(c *fiber.Ctx, rawReason string) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	reason, err := requireReason(rawReason, "void an expense")
	if err != nil {
		return err
	}
	actor := actorOf(c)
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		prev, err := findExpense(tx, h.store, id)
		if err != nil {
			return err
		}
		if prev.VoidedAt != nil {
			return apiErr(http.StatusConflict, CodeAlreadyVoided, "expense is already voided")
		}
		now := time.Now()
		res, err := h.store.Coll(models.CollExpenses).UpdateOne(tx, bson.M{"_id": id, "voidedAt": nil},
			bson.M{"$set": bson.M{"voidedAt": now, "voidReason": reason, "voidedBy": actor}})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return apiErr(http.StatusConflict, CodeAlreadyVoided, "expense is already voided")
		}
		after := prev
		after.VoidedAt, after.VoidReason, after.VoidedBy = &now, reason, actor
		return writeAudit(tx, h.store, auditLine{Entity: "expense", Ref: id, Action: "void",
			Before: prev, After: after, Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true, "voided": true})
}

// ledgerRows loads the period's payments, expenses and sales (live only, or
// with voided rows too) — the single source both the financials summary and
// the statement export compute from, so the two can never disagree.
func ledgerRows(ctx context.Context, store *db.Store, from, to time.Time, includeVoided bool) ([]models.Payment, []models.Expense, []models.Sale, error) {
	dateFilter := func() bson.M {
		f := bson.M{"date": bson.M{"$gte": from, "$lt": to}}
		if !includeVoided {
			f = notVoided(f)
		}
		return f
	}
	sortAsc := options.Find().SetSort(bson.D{{Key: "date", Value: 1}, {Key: "_id", Value: 1}})
	pcur, err := store.Coll(models.CollPayments).Find(ctx, dateFilter(), sortAsc)
	if err != nil {
		return nil, nil, nil, err
	}
	var payments []models.Payment
	if err := pcur.All(ctx, &payments); err != nil {
		return nil, nil, nil, err
	}
	ecur, err := store.Coll(models.CollExpenses).Find(ctx, dateFilter(), sortAsc)
	if err != nil {
		return nil, nil, nil, err
	}
	var expenses []models.Expense
	if err := ecur.All(ctx, &expenses); err != nil {
		return nil, nil, nil, err
	}
	scur, err := store.Coll(models.CollSales).Find(ctx, dateFilter(), sortAsc)
	if err != nil {
		return nil, nil, nil, err
	}
	var sales []models.Sale
	if err := scur.All(ctx, &sales); err != nil {
		return nil, nil, nil, err
	}
	return payments, expenses, sales, nil
}

// financials reports cash in (payments + shop sales) vs recorded cash out
// (expenses) for a period, with breakdowns by payment type and expense
// category. Voided records never count.
func (h *expenseHandler) financials(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to, err := monthOrRange(c)
	if err != nil {
		return err
	}

	payments, expenses, sales, err := ledgerRows(ctx, h.store, from, to, false)
	if err != nil {
		return err
	}

	var income, outgoings int64
	byType := map[string]int64{}
	for _, p := range payments {
		income += models.Cents(p.Amount)
		byType[p.Type] += models.Cents(p.Amount)
	}
	// Inventory sales are shop income too (skip any legacy sale already
	// mirrored as a payment to avoid double-count).
	for _, s := range sales {
		if s.PaymentID == nil {
			income += models.Cents(s.Total)
			byType[models.PaySale] += models.Cents(s.Total)
		}
	}
	byCategory := map[string]int64{}
	for _, e := range expenses {
		outgoings += models.Cents(e.Amount)
		byCategory[e.Category] += models.Cents(e.Amount)
	}
	return c.JSON(fiber.Map{
		"income":     models.Amount(income),
		"outgoings":  models.Amount(outgoings),
		"net":        models.Amount(income - outgoings),
		"byType":     centsMap(byType),
		"byCategory": centsMap(byCategory),
		"from":       from,
		"to":         to,
	})
}

func centsMap(m map[string]int64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = models.Amount(v)
	}
	return out
}

// statement exports the full period ledger as CSV: every payment, sale and
// expense in date order (voided rows included, marked VOID and excluded from
// the totals), followed by income / outgoings / net summary lines.
func (h *expenseHandler) statement(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to, err := monthOrRange(c)
	if err != nil {
		return err
	}

	payments, expenses, sales, err := ledgerRows(ctx, h.store, from, to, true)
	if err != nil {
		return err
	}

	type row struct {
		date         time.Time
		kind, detail string
		typ, note    string
		in, out      int64
		voided       bool
	}
	rows := make([]row, 0, len(payments)+len(expenses)+len(sales))
	for _, p := range payments {
		rows = append(rows, row{p.Date, "payment", p.TraineeName, p.Type, p.Note, models.Cents(p.Amount), 0, p.VoidedAt != nil})
	}
	for _, s := range sales {
		if s.PaymentID != nil {
			continue // legacy mirror: already present as a payment row
		}
		detail := s.ItemName
		if s.TraineeName != "" {
			detail += " → " + s.TraineeName
		}
		rows = append(rows, row{s.Date, "sale", detail, "sale", "", models.Cents(s.Total), 0, s.VoidedAt != nil})
	}
	for _, e := range expenses {
		rows = append(rows, row{e.Date, "expense", e.Category, "expense", e.Note, 0, models.Cents(e.Amount), e.VoidedAt != nil})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].date.Before(rows[j].date) })

	cents := func(v int64) string { return strconv.FormatFloat(models.Amount(v), 'f', 2, 64) }
	var income, outgoings int64
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Date", "Kind", "Detail", "Type", "Note", "In", "Out", "Status"})
	for _, r := range rows {
		status := ""
		if r.voided {
			status = "VOID"
		} else {
			income += r.in
			outgoings += r.out
		}
		_ = w.Write([]string{
			r.date.In(time.Local).Format("2006-01-02 15:04"),
			r.kind, r.detail, r.typ, r.note,
			cents(r.in), cents(r.out), status,
		})
	}
	_ = w.Write([]string{})
	_ = w.Write([]string{"", "", "", "", "Total income", cents(income), "", ""})
	_ = w.Write([]string{"", "", "", "", "Total outgoings", "", cents(outgoings), ""})
	_ = w.Write([]string{"", "", "", "", "Net cash", cents(income - outgoings), "", ""})
	w.Flush()

	label := c.Query("m")
	if label == "" {
		label = models.DateKey(from) + "_" + models.DateKey(to)
	}
	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=statement-%s.csv", label))
	return c.Send(buf.Bytes())
}
