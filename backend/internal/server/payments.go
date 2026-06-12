package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

type paymentHandler struct{ store *db.Store }

func registerPayments(r fiber.Router, store *db.Store) {
	h := &paymentHandler{store}
	g := r.Group("/payments")
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Get("/export", h.export)
	g.Get("/:id/receipt", h.receipt)
	g.Put("/:id", h.update)
	g.Delete("/:id", h.remove)
	r.Get("/subscriptions", h.subscriptions)
}

type paymentInput struct {
	Trainee     string     `json:"trainee"`
	Amount      float64    `json:"amount"`
	Type        string     `json:"type"`
	PeriodMonth string     `json:"periodMonth"`
	Date        *time.Time `json:"date"`
	Note        string     `json:"note"`
}

// monthOrRange resolves ?m=YYYY-MM or ?from=&to= into a [start,end) window.
func monthOrRange(c *fiber.Ctx) (time.Time, time.Time) {
	if m := c.Query("m"); m != "" {
		if start, end, err := models.MonthRange(m); err == nil {
			return start, end
		}
	}
	return parseRange(c)
}

func (h *paymentHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to := monthOrRange(c)
	cur, err := h.store.Coll(models.CollPayments).Find(ctx,
		bson.M{"date": bson.M{"$gte": from, "$lt": to}},
		options.Find().SetSort(bson.D{{Key: "date", Value: -1}}))
	if err != nil {
		return err
	}
	out := []models.Payment{}
	if err := cur.All(ctx, &out); err != nil {
		return err
	}
	return c.JSON(out)
}

func (h *paymentHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in paymentInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	in.Amount = round2(in.Amount)
	if in.Amount <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "amount must be positive")
	}
	typ := defaultStr(in.Type, models.PayOther)
	if err := validatePeriodMonth(typ, in.PeriodMonth); err != nil {
		return err
	}
	p := models.Payment{
		Amount:      in.Amount,
		Type:        typ,
		PeriodMonth: in.PeriodMonth,
		Note:        in.Note,
		Date:        time.Now(),
		CreatedAt:   time.Now(),
	}
	if in.Date != nil {
		p.Date = *in.Date
	}
	if in.Trainee != "" {
		if tid, err := primitive.ObjectIDFromHex(in.Trainee); err == nil {
			if typ == models.PaySubscription {
				if err := checkSubscriptionOverpay(ctx, h.store, tid, in.PeriodMonth, in.Amount, nil); err != nil {
					return err
				}
			}
			p.Trainee = &tid
			names := traineeNames(ctx, h.store, []primitive.ObjectID{tid})
			p.TraineeName = names[tid]
		}
	}
	res, err := h.store.Coll(models.CollPayments).InsertOne(ctx, p)
	if err != nil {
		return err
	}
	p.ID = res.InsertedID.(primitive.ObjectID)
	return c.Status(fiber.StatusCreated).JSON(p)
}

func (h *paymentHandler) update(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in paymentInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	in.Amount = round2(in.Amount)
	if in.Amount <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "amount must be positive")
	}
	typ := defaultStr(in.Type, models.PayOther)
	if err := validatePeriodMonth(typ, in.PeriodMonth); err != nil {
		return err
	}
	// Voided records are frozen history — corrections happen on live ones.
	var prev models.Payment
	if err := h.store.Coll(models.CollPayments).FindOne(ctx, bson.M{"_id": id}).Decode(&prev); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "payment not found")
	}
	if prev.VoidedAt != nil {
		return fiber.NewError(fiber.StatusConflict, "payment is voided and can no longer be edited")
	}
	set := bson.M{
		"amount":      in.Amount,
		"type":        typ,
		"periodMonth": in.PeriodMonth,
		"note":        in.Note,
	}
	if in.Date != nil {
		set["date"] = *in.Date
	}
	// Re-resolve the trainee link (and denormalized name), or clear it.
	if in.Trainee != "" {
		tid, err := primitive.ObjectIDFromHex(in.Trainee)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid trainee id")
		}
		if typ == models.PaySubscription {
			if err := checkSubscriptionOverpay(ctx, h.store, tid, in.PeriodMonth, in.Amount, &id); err != nil {
				return err
			}
		}
		names := traineeNames(ctx, h.store, []primitive.ObjectID{tid})
		set["trainee"] = tid
		set["traineeName"] = names[tid]
	} else {
		set["trainee"] = nil
		set["traineeName"] = ""
	}
	if _, err := h.store.Coll(models.CollPayments).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
		return err
	}
	var p models.Payment
	if err := h.store.Coll(models.CollPayments).FindOne(ctx, bson.M{"_id": id}).Decode(&p); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "payment not found")
	}
	writeAudit(ctx, h.store, "payment", id, "update", prev, p)
	return c.JSON(p)
}

// remove voids a payment rather than deleting it: the record stays in the
// books forever, marked void, and stops counting toward revenue and dues.
func (h *paymentHandler) remove(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var prev models.Payment
	if err := h.store.Coll(models.CollPayments).FindOne(ctx, bson.M{"_id": id}).Decode(&prev); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "payment not found")
	}
	if prev.VoidedAt != nil {
		return fiber.NewError(fiber.StatusConflict, "payment is already voided")
	}
	now := time.Now()
	reason := c.Query("reason")
	if _, err := h.store.Coll(models.CollPayments).UpdateOne(ctx,
		bson.M{"_id": id, "voidedAt": nil},
		bson.M{"$set": bson.M{"voidedAt": now, "voidReason": reason}}); err != nil {
		return err
	}
	after := prev
	after.VoidedAt = &now
	after.VoidReason = reason
	writeAudit(ctx, h.store, "payment", id, "void", prev, after)
	return c.JSON(fiber.Map{"ok": true, "voided": true})
}

// validatePeriodMonth ensures a subscription payment's period is a real YYYY-MM,
// since the dues matcher keys on an exact string match.
func validatePeriodMonth(typ, periodMonth string) error {
	if typ == models.PaySubscription && periodMonth != "" {
		if _, _, err := models.MonthRange(periodMonth); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "periodMonth must be YYYY-MM")
		}
	}
	return nil
}

// checkSubscriptionOverpay rejects a subscription payment that would push a
// trainee's total for a period past their monthly fee — overpaying dues is
// almost always a data-entry mistake, and silently absorbing it corrupts the
// books. exclude is the payment being edited (so updates don't count
// themselves); nil on create. Skipped when the trainee has no monthly fee
// (nothing is owed, so there is nothing to overpay).
func checkSubscriptionOverpay(ctx context.Context, store *db.Store, traineeID primitive.ObjectID,
	periodMonth string, amount float64, exclude *primitive.ObjectID) error {
	if periodMonth == "" {
		return nil
	}
	var t models.Trainee
	if err := store.Coll(models.CollTrainees).FindOne(ctx, bson.M{"_id": traineeID}).Decode(&t); err != nil {
		return nil // missing trainee is handled by the caller's own validation
	}
	if t.MonthlyFee <= 0 {
		return nil
	}
	filter := notVoided(bson.M{
		"type":        models.PaySubscription,
		"periodMonth": periodMonth,
		"trainee":     traineeID,
	})
	if exclude != nil {
		filter["_id"] = bson.M{"$ne": *exclude}
	}
	cur, err := store.Coll(models.CollPayments).Find(ctx, filter)
	if err != nil {
		return err
	}
	var ps []models.Payment
	if err := cur.All(ctx, &ps); err != nil {
		return err
	}
	var paid float64
	for _, p := range ps {
		paid += p.Amount
	}
	remaining := t.MonthlyFee - paid
	if remaining < 0 {
		remaining = 0
	}
	if amount > remaining {
		return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf(
			"%s would overpay %s: fee %s, already paid %s, remaining %s",
			fmtAmt(amount), periodMonth, fmtAmt(t.MonthlyFee), fmtAmt(paid), fmtAmt(remaining)))
	}
	return nil
}

func fmtAmt(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func (h *paymentHandler) receipt(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var p models.Payment
	if err := h.store.Coll(models.CollPayments).FindOne(ctx, bson.M{"_id": id}).Decode(&p); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "payment not found")
	}
	return c.JSON(fiber.Map{
		"studio":  "Bronze Boxing",
		"payment": p,
		"issued":  time.Now().UTC(),
	})
}

func (h *paymentHandler) export(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to := monthOrRange(c)
	cur, err := h.store.Coll(models.CollPayments).Find(ctx,
		bson.M{"date": bson.M{"$gte": from, "$lt": to}},
		options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return err
	}
	var payments []models.Payment
	if err := cur.All(ctx, &payments); err != nil {
		return err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Date", "Trainee", "Type", "Period", "Amount", "Note", "Status"})
	for _, p := range payments {
		status := ""
		if p.VoidedAt != nil {
			status = "VOID"
		}
		_ = w.Write([]string{
			p.Date.Format("2006-01-02"),
			p.TraineeName,
			p.Type,
			p.PeriodMonth,
			strconv.FormatFloat(p.Amount, 'f', 2, 64),
			p.Note,
			status,
		})
	}
	w.Flush()
	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=payments-%s.csv", time.Now().Format("2006-01-02")))
	return c.Send(buf.Bytes())
}

type subStatus struct {
	Trainee    models.Trainee `json:"trainee"`
	Due        float64        `json:"due"`
	AmountPaid float64        `json:"amountPaid"`
	State      string         `json:"state"` // paid | partial | unpaid
}

func (h *paymentHandler) subscriptions(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	month := c.Query("m")
	if month == "" {
		month = models.MonthKey(time.Now())
	}
	statuses, err := computeSubStatuses(ctx, h.store, month)
	if err != nil {
		return err
	}
	return c.JSON(statuses)
}

// computeMonthRevenue sums all payments dated within the given YYYY-MM month.
func computeMonthRevenue(ctx context.Context, store *db.Store, month string) (float64, error) {
	start, end, err := models.MonthRange(month)
	if err != nil {
		return 0, err
	}
	cur, err := store.Coll(models.CollPayments).Find(ctx, notVoided(bson.M{"date": bson.M{"$gte": start, "$lt": end}}))
	if err != nil {
		return 0, err
	}
	var ps []models.Payment
	if err := cur.All(ctx, &ps); err != nil {
		return 0, err
	}
	var total float64
	for _, p := range ps {
		total += p.Amount
	}
	return round2(total), nil
}

// computeSubStatuses builds per-trainee subscription dues/paid/state for a month.
func computeSubStatuses(ctx context.Context, store *db.Store, month string) ([]subStatus, error) {
	cur, err := store.Coll(models.CollTrainees).Find(ctx, bson.M{
		"status":     models.StatusActive,
		"monthlyFee": bson.M{"$gt": 0},
	}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var trainees []models.Trainee
	if err := cur.All(ctx, &trainees); err != nil {
		return nil, err
	}

	// Sum subscription payments for this period, by trainee (live ones only).
	pcur, err := store.Coll(models.CollPayments).Find(ctx, notVoided(bson.M{
		"type":        models.PaySubscription,
		"periodMonth": month,
	}))
	if err != nil {
		return nil, err
	}
	var pays []models.Payment
	if err := pcur.All(ctx, &pays); err != nil {
		return nil, err
	}
	paid := map[primitive.ObjectID]float64{}
	for _, p := range pays {
		if p.Trainee != nil {
			paid[*p.Trainee] += p.Amount
		}
	}

	out := make([]subStatus, 0, len(trainees))
	for _, t := range trainees {
		amt := paid[t.ID]
		state := "unpaid"
		if amt >= t.MonthlyFee && t.MonthlyFee > 0 {
			state = "paid"
		} else if amt > 0 {
			state = "partial"
		}
		out = append(out, subStatus{Trainee: t, Due: t.MonthlyFee, AmountPaid: amt, State: state})
	}
	return out, nil
}
