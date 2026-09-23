package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
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

type paymentHandler struct{ store *db.Store }

func registerPayments(r fiber.Router, store *db.Store) {
	h := &paymentHandler{store}
	g := r.Group("/payments")
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Get("/export", h.export)
	g.Get("/:id/receipt", h.receipt)
	g.Get("/:id", h.get)
	g.Put("/:id", h.update)
	g.Post("/:id/void", h.voidPost)
	g.Delete("/:id", h.remove)
	r.Get("/subscriptions", h.subscriptions)
	r.Get("/subscription-charges", h.charges)
	r.Post("/subscription-charges/:id/adjust", h.adjust)
}

// Payment types a user may record directly. "sale" payments exist only as
// legacy mirrors of shop sales; new shop income goes through Inventory.
var recordablePayTypes = []string{models.PaySubscription, models.PayPrivate, models.PayDropin, models.PayOther}

var payMethods = []string{models.MethodCash, models.MethodCard, models.MethodTransfer, models.MethodOther}

// studioInfo is the identity printed on receipts (set from config in New).
type studioInfo struct {
	Name     string `json:"name"`
	Address  string `json:"address,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Currency string `json:"currency"`
}

var studio = studioInfo{Name: "Bronze Boxing Club", Currency: "$"}

type paymentInput struct {
	Trainee     string     `json:"trainee"`
	Amount      float64    `json:"amount"`
	Type        string     `json:"type"`
	PeriodMonth string     `json:"periodMonth"`
	Date        *time.Time `json:"date"`
	// Day records the cash date as a studio-local YYYY-MM-DD: today means
	// "now", another day means midday that day. Ignored when Date is sent.
	Day       string `json:"day"`
	Note      string `json:"note"`
	Method    string `json:"method"`    // cash | card | bank_transfer | other
	Reference string `json:"reference"` // optional slip / transfer reference
	Reason    string `json:"reason"`    // required when a correction changes the amount
}

// resolveRecordDate turns the optional date/day inputs into the instant the
// money moved. Defaults to now.
func resolveRecordDate(date *time.Time, day string) (time.Time, error) {
	if date != nil {
		if err := sanityTime("date", *date); err != nil {
			return time.Time{}, err
		}
		return *date, nil
	}
	if day == "" {
		return time.Now(), nil
	}
	start, err := models.ParseDay(day)
	if err != nil {
		return time.Time{}, badField("day", "day must be YYYY-MM-DD")
	}
	if day == models.DateKey(time.Now()) {
		return time.Now(), nil
	}
	return start.Add(12 * time.Hour), nil
}

// list returns payments by cash date (?m= or ?from=&to=), optionally narrowed
// to one trainee, one dues period (?periodMonth=, any cash date) and a type.
// With ?limit= the response is a page {items, total, hasMore}; without it,
// the original plain array.
func (h *paymentHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	filter := bson.M{}
	if c.Query("m") != "" || c.Query("from") != "" || c.Query("to") != "" {
		from, to, err := monthOrRange(c)
		if err != nil {
			return err
		}
		filter["date"] = bson.M{"$gte": from, "$lt": to}
	}
	if t := c.Query("trainee"); t != "" {
		tid, err := parseOID(t, "trainee")
		if err != nil {
			return err
		}
		filter["trainee"] = tid
	}
	if pm := c.Query("periodMonth"); pm != "" {
		if !models.ValidMonth(pm) {
			return badField("periodMonth", "periodMonth must be YYYY-MM")
		}
		filter["periodMonth"] = pm
	}
	if typ := c.Query("type"); typ != "" {
		filter["type"] = typ
	}
	if q, ok := searchQuery(c); ok {
		return rankedFind(c, ctx, h.store, models.CollPayments, kindPayment, filter, q,
			func(p models.Payment) primitive.ObjectID { return p.ID })
	}
	if len(filter) == 0 && c.Query("limit") == "" {
		return badField("m", "give a month, a date range, a trainee or a period (or page with limit)")
	}
	return pagedFind[models.Payment](c, ctx, h.store.Coll(models.CollPayments), filter,
		bson.D{{Key: "date", Value: -1}, {Key: "_id", Value: -1}})
}

func (h *paymentHandler) get(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	p, err := findPayment(ctx, h.store, id)
	if err != nil {
		return err
	}
	return c.JSON(p)
}

func findPayment(ctx context.Context, store *db.Store, id primitive.ObjectID) (models.Payment, error) {
	var p models.Payment
	if err := store.Coll(models.CollPayments).FindOne(ctx, bson.M{"_id": id}).Decode(&p); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return p, notFound("payment")
		}
		return p, err
	}
	return p, nil
}

// normalizePayment validates input into the fields a payment stores. The
// trainee must exist; a subscription must name its trainee and a real period.
func (h *paymentHandler) normalizePayment(ctx context.Context, in paymentInput, typeLocked string) (models.Payment, *models.Trainee, error) {
	var p models.Payment
	amount, err := validMoney("amount", in.Amount, false)
	if err != nil {
		return p, nil, err
	}
	typ := defaultStr(strings.TrimSpace(in.Type), models.PayOther)
	if typeLocked != "" {
		if typ != typeLocked {
			return p, nil, apiErr(http.StatusConflict, CodeLinkedSale,
				"this payment mirrors a shop sale — correct or void the sale in Inventory instead of reclassifying it").withField("type")
		}
	} else if err := oneOf("type", typ, recordablePayTypes...); err != nil {
		return p, nil, err
	}
	p.Amount, p.Type, p.Note = amount, typ, strings.TrimSpace(in.Note)
	if in.Method != "" {
		if err := oneOf("method", in.Method, payMethods...); err != nil {
			return p, nil, err
		}
	}
	p.Method = in.Method
	p.Reference = strings.TrimSpace(in.Reference)
	if len(p.Reference) > 120 {
		return p, nil, badField("reference", "reference must be at most 120 characters")
	}
	var trainee *models.Trainee
	if in.Trainee != "" {
		t, err := loadTrainee(ctx, h.store, in.Trainee, "trainee")
		if err != nil {
			return p, nil, err
		}
		trainee = &t
		p.Trainee = &t.ID
		p.TraineeName = t.Name
	}
	if typ == models.PaySubscription {
		if trainee == nil {
			return p, nil, badField("trainee", "a subscription payment needs its trainee")
		}
		if !models.ValidMonth(in.PeriodMonth) {
			return p, nil, badField("periodMonth", "periodMonth must be the YYYY-MM the fee covers")
		}
		p.PeriodMonth = in.PeriodMonth
	}
	return p, trainee, nil
}

func (h *paymentHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in paymentInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	p, trainee, err := h.normalizePayment(ctx, in, "")
	if err != nil {
		return err
	}
	if p.Date, err = resolveRecordDate(in.Date, in.Day); err != nil {
		return err
	}
	actor := actorOf(c)
	p.ID = primitive.NewObjectID()
	p.CreatedAt = time.Now()
	p.CreatedBy = actor

	err = h.store.WithTx(ctx, func(tx context.Context) error {
		if p.Type == models.PaySubscription {
			ch, err := ensureCharge(tx, h.store, *trainee, p.PeriodMonth)
			if err != nil {
				return err
			}
			if ch == nil {
				return explainChargeRefusal(tx, h.store, trainee.ID, p.PeriodMonth, 1)
			}
			if err := applyChargeDelta(tx, h.store, trainee.ID, p.PeriodMonth, models.Cents(p.Amount)); err != nil {
				return err
			}
		}
		if err := failpoint("payment.afterCharge"); err != nil {
			return err
		}
		if _, err := h.store.Coll(models.CollPayments).InsertOne(tx, p); err != nil {
			return err
		}
		if err := failpoint("payment.afterInsert"); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "payment", Ref: p.ID, Action: "create", After: p, Actor: actor})
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(p)
}

// reverseFromCharge takes a live subscription payment's amount back off its
// charge (void, or the "before" half of a correction). Payments that never
// counted toward a charge (legacy rows without a period) are left alone.
func reverseFromCharge(ctx context.Context, store *db.Store, p models.Payment) error {
	if p.Type != models.PaySubscription || p.Trainee == nil || p.PeriodMonth == "" {
		return nil
	}
	n, err := store.Coll(models.CollCharges).CountDocuments(ctx, chargeKey(*p.Trainee, p.PeriodMonth))
	if err != nil || n == 0 {
		return err
	}
	return applyChargeDelta(ctx, store, *p.Trainee, p.PeriodMonth, -models.Cents(p.Amount))
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
		return badField("body", "invalid body")
	}
	prev, err := findPayment(ctx, h.store, id)
	if err != nil {
		return err
	}
	if prev.VoidedAt != nil {
		return apiErr(http.StatusConflict, CodeVoidedLocked, "payment is voided and can no longer be edited")
	}
	locked := ""
	if prev.SaleID != nil || prev.Type == models.PaySale {
		locked = prev.Type
	}
	next, trainee, err := h.normalizePayment(ctx, in, locked)
	if err != nil {
		return err
	}
	next.ID, next.CreatedAt, next.CreatedBy, next.SaleID = prev.ID, prev.CreatedAt, prev.CreatedBy, prev.SaleID
	next.Date = prev.Date
	if in.Date != nil || in.Day != "" {
		if next.Date, err = resolveRecordDate(in.Date, in.Day); err != nil {
			return err
		}
	}
	reason := strings.TrimSpace(in.Reason)
	if models.Cents(next.Amount) != models.Cents(prev.Amount) {
		if reason, err = requireReason(in.Reason, "change a payment's amount"); err != nil {
			return err
		}
	}
	actor := actorOf(c)

	err = h.store.WithTx(ctx, func(tx context.Context) error {
		// Re-read inside the transaction: the correction applies to the
		// payment as it is now, and conflicts with a concurrent void.
		live, err := findPayment(tx, h.store, id)
		if err != nil {
			return err
		}
		if live.VoidedAt != nil {
			return apiErr(http.StatusConflict, CodeVoidedLocked, "payment is voided and can no longer be edited")
		}
		if err := reverseFromCharge(tx, h.store, live); err != nil {
			return err
		}
		if next.Type == models.PaySubscription {
			ch, err := ensureCharge(tx, h.store, *trainee, next.PeriodMonth)
			if err != nil {
				return err
			}
			if ch == nil {
				return explainChargeRefusal(tx, h.store, trainee.ID, next.PeriodMonth, 1)
			}
			if err := applyChargeDelta(tx, h.store, trainee.ID, next.PeriodMonth, models.Cents(next.Amount)); err != nil {
				return err
			}
		}
		set := bson.M{
			"amount": next.Amount, "type": next.Type, "periodMonth": next.PeriodMonth,
			"note": next.Note, "date": next.Date, "trainee": next.Trainee, "traineeName": next.TraineeName,
			"method": next.Method, "reference": next.Reference,
		}
		res, err := h.store.Coll(models.CollPayments).UpdateOne(tx, bson.M{"_id": id, "voidedAt": nil}, bson.M{"$set": set})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return apiErr(http.StatusConflict, CodeVoidedLocked, "payment is voided and can no longer be edited")
		}
		return writeAudit(tx, h.store, auditLine{Entity: "payment", Ref: id, Action: "update",
			Before: live, After: next, Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	p, err := findPayment(ctx, h.store, id)
	if err != nil {
		return err
	}
	return c.JSON(p)
}

// voidPayment marks a payment void rather than deleting it: the record stays
// in the books forever, stops counting toward revenue and dues, and its
// amount comes back off the month's charge — all in one transaction.
func voidPayment(ctx context.Context, store *db.Store, id primitive.ObjectID, reason, actor string) error {
	return store.WithTx(ctx, func(tx context.Context) error {
		prev, err := findPayment(tx, store, id)
		if err != nil {
			return err
		}
		if prev.VoidedAt != nil {
			return apiErr(http.StatusConflict, CodeAlreadyVoided, "payment is already voided")
		}
		now := time.Now()
		res, err := store.Coll(models.CollPayments).UpdateOne(tx, bson.M{"_id": id, "voidedAt": nil},
			bson.M{"$set": bson.M{"voidedAt": now, "voidReason": reason, "voidedBy": actor}})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return apiErr(http.StatusConflict, CodeAlreadyVoided, "payment is already voided")
		}
		if err := reverseFromCharge(tx, store, prev); err != nil {
			return err
		}
		if err := failpoint("paymentVoid.afterCharge"); err != nil {
			return err
		}
		after := prev
		after.VoidedAt, after.VoidReason, after.VoidedBy = &now, reason, actor
		return writeAudit(tx, store, auditLine{Entity: "payment", Ref: id, Action: "void",
			Before: prev, After: after, Actor: actor, Reason: reason})
	})
}

func (h *paymentHandler) remove(c *fiber.Ctx) error {
	return h.doVoid(c, c.Query("reason"))
}

func (h *paymentHandler) voidPost(c *fiber.Ctx) error {
	var in struct {
		Reason string `json:"reason"`
	}
	_ = c.BodyParser(&in)
	return h.doVoid(c, in.Reason)
}

func (h *paymentHandler) doVoid(c *fiber.Ctx, rawReason string) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	reason, err := requireReason(rawReason, "void a payment")
	if err != nil {
		return err
	}
	if err := voidPayment(ctx, h.store, id, reason, actorOf(c)); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true, "voided": true})
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
	p, err := findPayment(ctx, h.store, id)
	if err != nil {
		return err
	}
	// Receipt number: stable, short, derived from the payment id.
	number := strings.ToUpper(p.ID.Hex()[len(p.ID.Hex())-8:])
	out := fiber.Map{
		"studio":     studio.Name, // legacy string field
		"studioInfo": studio,
		"number":     number,
		"payment":    p,
		"issued":     time.Now().UTC(),
		"void":       p.VoidedAt != nil,
		"method":     defaultStr(p.Method, "unspecified"),
		"cashDay":    models.DateKey(p.Date),
		"timezone":   time.Local.String(),
	}
	if p.Type == models.PaySubscription && p.Trainee != nil && p.PeriodMonth != "" {
		var ch models.SubscriptionCharge
		if err := h.store.Coll(models.CollCharges).FindOne(ctx, chargeKey(*p.Trainee, p.PeriodMonth)).Decode(&ch); err == nil {
			out["charge"] = viewCharge(ch)
		}
	}
	return c.JSON(out)
}

func (h *paymentHandler) export(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to, err := monthOrRange(c)
	if err != nil {
		return err
	}
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
	_ = w.Write([]string{"Date", "Trainee", "Type", "Period", "Amount", "Method", "Reference", "Note", "Status", "Void reason"})
	for _, p := range payments {
		status := ""
		if p.VoidedAt != nil {
			status = "VOID"
		}
		_ = w.Write([]string{
			models.DateKey(p.Date),
			p.TraineeName,
			p.Type,
			p.PeriodMonth,
			strconv.FormatFloat(p.Amount, 'f', 2, 64),
			defaultStr(p.Method, "unspecified"),
			p.Reference,
			p.Note,
			status,
			p.VoidReason,
		})
	}
	w.Flush()
	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=payments-%s.csv", models.DateKey(time.Now())))
	return c.Send(buf.Bytes())
}

func (h *paymentHandler) subscriptions(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	month := c.Query("m")
	if month == "" {
		month = models.MonthKey(time.Now())
	}
	if !models.ValidMonth(month) {
		return badField("m", "m must be YYYY-MM")
	}
	rows, err := listDues(ctx, h.store, month)
	if err != nil {
		return err
	}
	return c.JSON(rows)
}

// charges lists stored charges, e.g. one trainee's dues history
// (?trainee=ID) newest month first.
func (h *paymentHandler) charges(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	filter := bson.M{}
	if t := c.Query("trainee"); t != "" {
		tid, err := parseOID(t, "trainee")
		if err != nil {
			return err
		}
		filter["trainee"] = tid
	}
	if s := c.Query("state"); s != "" {
		filter["state"] = s
	}
	cur, err := h.store.Coll(models.CollCharges).Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "periodMonth", Value: -1}}).SetLimit(int64(min(max(atoiDefault(c.Query("limit"), 120), 1), 500))))
	if err != nil {
		return err
	}
	var rows []models.SubscriptionCharge
	if err := cur.All(ctx, &rows); err != nil {
		return err
	}
	out := make([]chargeView, len(rows))
	for i, r := range rows {
		out[i] = viewCharge(r)
	}
	return c.JSON(out)
}

func (h *paymentHandler) adjust(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in struct {
		Due    *float64 `json:"due"`
		Reason string   `json:"reason"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	if in.Due == nil {
		return badField("due", "due is required")
	}
	due, err := validMoney("due", *in.Due, true)
	if err != nil {
		return err
	}
	reason, err := requireReason(in.Reason, "adjust a month's dues")
	if err != nil {
		return err
	}
	var out *models.SubscriptionCharge
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		ch, err := adjustCharge(tx, h.store, id, models.Cents(due), reason, actorOf(c))
		out = ch
		return err
	})
	if err != nil {
		return err
	}
	return c.JSON(viewCharge(*out))
}

// computeMonthRevenue sums all live payments dated within the given month.
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
	var total int64
	for _, p := range ps {
		total += models.Cents(p.Amount)
	}
	return models.Amount(total), nil
}
