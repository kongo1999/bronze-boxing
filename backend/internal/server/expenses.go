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
	g.Delete("/:id", h.remove)
	r.Get("/financials", h.financials)
	r.Get("/financials/export", h.statement)
}

type expenseInput struct {
	Amount   float64    `json:"amount"`
	Category string     `json:"category"`
	Note     string     `json:"note"`
	Date     *time.Time `json:"date"`
}

func (h *expenseHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to := monthOrRange(c)
	cur, err := h.store.Coll(models.CollExpenses).Find(ctx,
		bson.M{"date": bson.M{"$gte": from, "$lt": to}},
		options.Find().SetSort(bson.D{{Key: "date", Value: -1}}))
	if err != nil {
		return err
	}
	out := []models.Expense{}
	if err := cur.All(ctx, &out); err != nil {
		return err
	}
	return c.JSON(out)
}

func (h *expenseHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in expenseInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	in.Amount = round2(in.Amount)
	if in.Amount <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "amount must be positive")
	}
	e := models.Expense{
		Amount:    in.Amount,
		Category:  defaultStr(in.Category, models.ExpOther),
		Note:      in.Note,
		Date:      time.Now(),
		CreatedAt: time.Now(),
	}
	if in.Date != nil {
		e.Date = *in.Date
	}
	res, err := h.store.Coll(models.CollExpenses).InsertOne(ctx, e)
	if err != nil {
		return err
	}
	e.ID = res.InsertedID.(primitive.ObjectID)
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
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	in.Amount = round2(in.Amount)
	if in.Amount <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "amount must be positive")
	}
	var prev models.Expense
	if err := h.store.Coll(models.CollExpenses).FindOne(ctx, bson.M{"_id": id}).Decode(&prev); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "expense not found")
	}
	if prev.VoidedAt != nil {
		return fiber.NewError(fiber.StatusConflict, "expense is voided and can no longer be edited")
	}
	set := bson.M{
		"amount":   in.Amount,
		"category": defaultStr(in.Category, models.ExpOther),
		"note":     in.Note,
	}
	if in.Date != nil {
		set["date"] = *in.Date
	}
	if _, err := h.store.Coll(models.CollExpenses).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
		return err
	}
	var e models.Expense
	if err := h.store.Coll(models.CollExpenses).FindOne(ctx, bson.M{"_id": id}).Decode(&e); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "expense not found")
	}
	writeAudit(ctx, h.store, "expense", id, "update", prev, e)
	return c.JSON(e)
}

// remove voids an expense rather than deleting it — the record stays in the
// books, marked void, and stops counting toward outgoings.
func (h *expenseHandler) remove(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var prev models.Expense
	if err := h.store.Coll(models.CollExpenses).FindOne(ctx, bson.M{"_id": id}).Decode(&prev); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "expense not found")
	}
	if prev.VoidedAt != nil {
		return fiber.NewError(fiber.StatusConflict, "expense is already voided")
	}
	now := time.Now()
	reason := c.Query("reason")
	if _, err := h.store.Coll(models.CollExpenses).UpdateOne(ctx,
		bson.M{"_id": id, "voidedAt": nil},
		bson.M{"$set": bson.M{"voidedAt": now, "voidReason": reason}}); err != nil {
		return err
	}
	after := prev
	after.VoidedAt = &now
	after.VoidReason = reason
	writeAudit(ctx, h.store, "expense", id, "void", prev, after)
	return c.JSON(fiber.Map{"ok": true, "voided": true})
}

// ledgerRows loads the period's live (non-voided) payments, expenses and
// sales — the single source both the financials summary and the statement
// export compute from, so the two can never disagree.
func ledgerRows(ctx context.Context, store *db.Store, from, to time.Time, includeVoided bool) ([]models.Payment, []models.Expense, []models.Sale, error) {
	dateFilter := func() bson.M {
		f := bson.M{"date": bson.M{"$gte": from, "$lt": to}}
		if !includeVoided {
			f = notVoided(f)
		}
		return f
	}
	pcur, err := store.Coll(models.CollPayments).Find(ctx, dateFilter(),
		options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return nil, nil, nil, err
	}
	var payments []models.Payment
	if err := pcur.All(ctx, &payments); err != nil {
		return nil, nil, nil, err
	}
	ecur, err := store.Coll(models.CollExpenses).Find(ctx, dateFilter(),
		options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return nil, nil, nil, err
	}
	var expenses []models.Expense
	if err := ecur.All(ctx, &expenses); err != nil {
		return nil, nil, nil, err
	}
	scur, err := store.Coll(models.CollSales).Find(ctx, dateFilter(),
		options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return nil, nil, nil, err
	}
	var sales []models.Sale
	if err := scur.All(ctx, &sales); err != nil {
		return nil, nil, nil, err
	}
	return payments, expenses, sales, nil
}

// financials reports income (payments + shop sales) vs outgoings (expenses)
// for a period, with breakdowns by payment type and expense category. Voided
// records never count.
func (h *expenseHandler) financials(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to := monthOrRange(c)

	payments, expenses, sales, err := ledgerRows(ctx, h.store, from, to, false)
	if err != nil {
		return err
	}

	var income, outgoings float64
	byType := map[string]float64{}
	for _, p := range payments {
		income += p.Amount
		byType[p.Type] += p.Amount
	}
	// Inventory sales are shop income too (skip any legacy sale already
	// mirrored as a payment to avoid double-count).
	for _, s := range sales {
		if s.PaymentID == nil {
			income += s.Total
			byType["sale"] += s.Total
		}
	}
	byCategory := map[string]float64{}
	for _, e := range expenses {
		outgoings += e.Amount
		byCategory[e.Category] += e.Amount
	}
	for k, v := range byType {
		byType[k] = round2(v)
	}
	for k, v := range byCategory {
		byCategory[k] = round2(v)
	}
	return c.JSON(fiber.Map{
		"income":     round2(income),
		"outgoings":  round2(outgoings),
		"net":        round2(income - outgoings),
		"byType":     byType,
		"byCategory": byCategory,
		"from":       from,
		"to":         to,
	})
}

// statement exports the full month ledger as CSV: every payment, sale and
// expense in date order (voided rows included, marked VOID and excluded from
// the totals), followed by income / outgoings / net summary lines.
func (h *expenseHandler) statement(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to := monthOrRange(c)

	payments, expenses, sales, err := ledgerRows(ctx, h.store, from, to, true)
	if err != nil {
		return err
	}

	type row struct {
		date           time.Time
		kind, detail   string
		typ, note      string
		in, out        float64
		voided         bool
	}
	rows := make([]row, 0, len(payments)+len(expenses)+len(sales))
	for _, p := range payments {
		rows = append(rows, row{p.Date, "payment", p.TraineeName, p.Type, p.Note, p.Amount, 0, p.VoidedAt != nil})
	}
	for _, s := range sales {
		if s.PaymentID != nil {
			continue // legacy mirror: already present as a payment row
		}
		detail := s.ItemName
		if s.TraineeName != "" {
			detail += " → " + s.TraineeName
		}
		rows = append(rows, row{s.Date, "sale", detail, "sale", "", s.Total, 0, s.VoidedAt != nil})
	}
	for _, e := range expenses {
		rows = append(rows, row{e.Date, "expense", e.Category, "expense", e.Note, 0, e.Amount, e.VoidedAt != nil})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].date.Before(rows[j].date) })

	var income, outgoings float64
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
			r.date.Format("2006-01-02 15:04"),
			r.kind, r.detail, r.typ, r.note,
			strconv.FormatFloat(r.in, 'f', 2, 64),
			strconv.FormatFloat(r.out, 'f', 2, 64),
			status,
		})
	}
	_ = w.Write([]string{})
	_ = w.Write([]string{"", "", "", "", "Total income", strconv.FormatFloat(round2(income), 'f', 2, 64), "", ""})
	_ = w.Write([]string{"", "", "", "", "Total outgoings", "", strconv.FormatFloat(round2(outgoings), 'f', 2, 64), ""})
	_ = w.Write([]string{"", "", "", "", "Net", strconv.FormatFloat(round2(income-outgoings), 'f', 2, 64), "", ""})
	w.Flush()

	label := c.Query("m")
	if label == "" {
		label = from.Format("2006-01")
	}
	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=statement-%s.csv", label))
	return c.Send(buf.Bytes())
}
