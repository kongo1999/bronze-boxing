package server

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

type traineeHandler struct{ store *db.Store }

func registerTrainees(r fiber.Router, store *db.Store) {
	h := &traineeHandler{store}
	g := r.Group("/trainees")
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Get("/:id", h.get)
	g.Put("/:id", h.update)
	g.Delete("/:id", h.remove)
	g.Get("/:id/terms", h.terms)
	g.Post("/:id/terms", h.addTerms)
}

var (
	skillLevels     = []string{"", models.SkillBeginner, models.SkillIntermediate, models.SkillAdvanced}
	traineeStatuses = []string{models.StatusActive, models.StatusInactive}
)

type traineeInput struct {
	Name       string  `json:"name"`
	Phone      string  `json:"phone"`
	SkillLevel string  `json:"skillLevel"`
	MonthlyFee float64 `json:"monthlyFee"`
	Status     string  `json:"status"`
	Notes      string  `json:"notes"`
	// When an edit changes the fee or status: does it price this month
	// ("this_month", adjusting an already-issued charge — needs termsReason)
	// or start next month ("next_month", the default)?
	TermsApplyFrom string `json:"termsApplyFrom"`
	TermsReason    string `json:"termsReason"`
	TermsConfirm   bool   `json:"termsConfirm"` // add even if a change was already recorded today
}

func (h *traineeHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	filter := bson.M{}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		filter["name"] = bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}
	}
	cur, err := h.store.Coll(models.CollTrainees).
		Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return err
	}
	out := []models.Trainee{}
	if err := cur.All(ctx, &out); err != nil {
		return err
	}
	return c.JSON(out)
}

func (h *traineeHandler) get(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var t models.Trainee
	if err := h.store.Coll(models.CollTrainees).FindOne(ctx, bson.M{"_id": id}).Decode(&t); err != nil {
		return notFound("trainee")
	}
	return c.JSON(t)
}

func validateTrainee(in traineeInput) (traineeInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Phone = strings.TrimSpace(in.Phone)
	in.Notes = strings.TrimSpace(in.Notes)
	in.Status = defaultStr(in.Status, models.StatusActive)
	if in.Name == "" {
		return in, badField("name", "name is required")
	}
	if err := oneOf("skillLevel", in.SkillLevel, skillLevels...); err != nil {
		return in, err
	}
	if err := oneOf("status", in.Status, traineeStatuses...); err != nil {
		return in, err
	}
	fee, err := validMoney("monthlyFee", in.MonthlyFee, true)
	if err != nil {
		return in, err
	}
	in.MonthlyFee = fee
	return in, nil
}

func (h *traineeHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	var in traineeInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	in, err := validateTrainee(in)
	if err != nil {
		return err
	}
	now := time.Now()
	month := models.MonthKey(now)
	actor := actorOf(c)
	t := models.Trainee{
		ID:           primitive.NewObjectID(),
		Name:         in.Name,
		Phone:        in.Phone,
		SkillLevel:   in.SkillLevel,
		MonthlyFee:   in.MonthlyFee,
		Status:       in.Status,
		FeeFromMonth: month,
		Notes:        in.Notes,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	// Joining mid-month owes the month's full fee (no implicit proration):
	// the first term bills from this month, and this month's charge is
	// issued with the trainee so it is priced at the fee they joined on.
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		if _, err := h.store.Coll(models.CollTrainees).InsertOne(tx, t); err != nil {
			return err
		}
		term := models.SubscriptionTerm{
			Trainee: t.ID, EffectiveDate: models.DateKey(now), BillingFromMonth: month,
			MonthlyFee: t.MonthlyFee, Status: t.Status, CreatedAt: now, Actor: actor,
		}
		if _, err := h.store.Coll(models.CollTerms).InsertOne(tx, term); err != nil {
			return err
		}
		_, err := ensureCharge(tx, h.store, t, month)
		return err
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(t)
}

func (h *traineeHandler) update(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in traineeInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	if in, err = validateTrainee(in); err != nil {
		return err
	}
	prev, err := loadTrainee(ctx, h.store, id.Hex(), "id")
	if err != nil {
		return notFound("trainee")
	}
	termsChanged := models.Cents(in.MonthlyFee) != models.Cents(prev.MonthlyFee) || in.Status != prev.Status
	actor := actorOf(c)
	now := time.Now()
	current := models.MonthKey(now)

	err = h.store.WithTx(ctx, func(tx context.Context) error {
		set := bson.M{
			"name":       in.Name,
			"phone":      in.Phone,
			"skillLevel": in.SkillLevel,
			"notes":      in.Notes,
			"updatedAt":  now,
		}
		if _, err := h.store.Coll(models.CollTrainees).UpdateOne(tx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
			return err
		}
		if !termsChanged {
			return nil
		}
		from := models.ShiftMonth(current, 1)
		switch in.TermsApplyFrom {
		case "", "next_month":
		case "this_month":
			from = current
		default:
			return badField("termsApplyFrom", "termsApplyFrom must be this_month or next_month")
		}
		_, err := applyTermsChange(tx, h.store, prev, termsChange{
			EffectiveDate: models.DateKey(now), BillingFromMonth: from,
			MonthlyFee: in.MonthlyFee, Status: in.Status,
			Reason: strings.TrimSpace(in.TermsReason), Confirm: in.TermsConfirm,
		}, actor)
		return err
	})
	if err != nil {
		return err
	}
	var t models.Trainee
	if err := h.store.Coll(models.CollTrainees).FindOne(ctx, bson.M{"_id": id}).Decode(&t); err != nil {
		return notFound("trainee")
	}
	return c.JSON(t)
}

type termsChange struct {
	EffectiveDate    string  `json:"effectiveDate"`
	BillingFromMonth string  `json:"billingFromMonth"`
	MonthlyFee       float64 `json:"monthlyFee"`
	Status           string  `json:"status"`
	Reason           string  `json:"reason"`
	Confirm          bool    `json:"confirm"`
}

// applyTermsChange appends an effective-dated term (inside the caller's
// transaction) and reconciles charges that already exist from its billing
// month on:
//
//   - past months are never touched (the API refuses a billing month before
//     the current one — change a specific past month with an adjustment);
//   - the current month's charge is re-priced only when the new terms still
//     bill the trainee and the fee differs, and only with a reason, as an
//     audited adjustment; going inactive leaves an issued charge standing;
//   - charges for future months (issued early by a prepayment) follow the new
//     terms, but never below what has been paid toward them.
func applyTermsChange(ctx context.Context, store *db.Store, t models.Trainee, ch termsChange, actor string) (*models.SubscriptionTerm, error) {
	now := time.Now()
	current := models.MonthKey(now)
	if ch.EffectiveDate == "" {
		ch.EffectiveDate = models.DateKey(now)
	}
	if _, err := models.ParseDay(ch.EffectiveDate); err != nil {
		return nil, badField("effectiveDate", "effectiveDate must be YYYY-MM-DD")
	}
	if !models.ValidMonth(ch.BillingFromMonth) {
		return nil, badField("billingFromMonth", "billingFromMonth must be YYYY-MM")
	}
	if ch.BillingFromMonth < current {
		return nil, badField("billingFromMonth",
			"a fee or status change can bill from this month at the earliest — past months keep their recorded dues (adjust a single month instead)")
	}
	if err := oneOf("status", ch.Status, traineeStatuses...); err != nil {
		return nil, err
	}
	fee, err := validMoney("monthlyFee", ch.MonthlyFee, true)
	if err != nil {
		return nil, err
	}
	if !ch.Confirm {
		var same models.SubscriptionTerm
		if err := store.Coll(models.CollTerms).FindOne(ctx, bson.M{"trainee": t.ID, "effectiveDate": ch.EffectiveDate},
			options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: -1}})).Decode(&same); err == nil {
			return nil, apiErr(http.StatusConflict, CodeDuplicate, fmt.Sprintf(
				"%s already has a fee/status change recorded for %s (%s, %s from %s). Review it, then confirm to add another.",
				t.Name, ch.EffectiveDate, fmtAmt(same.MonthlyFee), same.Status, same.BillingFromMonth)).
				withDetails(map[string]any{"existing": same})
		}
	}
	term := models.SubscriptionTerm{
		ID: primitive.NewObjectID(), Trainee: t.ID, EffectiveDate: ch.EffectiveDate, BillingFromMonth: ch.BillingFromMonth,
		MonthlyFee: fee, Status: ch.Status, Reason: ch.Reason, CreatedAt: now, Actor: actor,
	}
	if _, err := store.Coll(models.CollTerms).InsertOne(ctx, term); err != nil {
		return nil, err
	}
	if err := writeAudit(ctx, store, auditLine{Entity: "trainee", Ref: t.ID, Action: "terms",
		After: term, Actor: actor, Reason: ch.Reason}); err != nil {
		return nil, err
	}

	all, err := loadTerms(ctx, store, bson.M{"trainee": t.ID})
	if err != nil {
		return nil, err
	}
	cur, err := store.Coll(models.CollCharges).Find(ctx, bson.M{"trainee": t.ID, "periodMonth": bson.M{"$gte": ch.BillingFromMonth}})
	if err != nil {
		return nil, err
	}
	var charges []models.SubscriptionCharge
	if err := cur.All(ctx, &charges); err != nil {
		return nil, err
	}
	for _, c := range charges {
		gov := billingTermFor(all[t.ID], c.PeriodMonth)
		switch {
		case c.PeriodMonth == current:
			if !billable(gov) || models.Cents(gov.MonthlyFee) == c.DueCents {
				continue // an issued charge stands when the trainee stops being billed
			}
			reason, err := requireReason(ch.Reason, "re-price this month's already-issued dues")
			if err != nil {
				return nil, err
			}
			if _, err := adjustCharge(ctx, store, c.ID, models.Cents(gov.MonthlyFee), reason, actor); err != nil {
				return nil, err
			}
		case c.PeriodMonth > current:
			due := int64(0)
			if billable(gov) {
				due = models.Cents(gov.MonthlyFee)
			}
			if due < c.PaidCents {
				due = c.PaidCents // never strand a prepayment
			}
			if due == c.DueCents {
				continue
			}
			if _, err := adjustCharge(ctx, store, c.ID, due, defaultStr(ch.Reason, "terms change from "+ch.BillingFromMonth), actor); err != nil {
				return nil, err
			}
		}
	}

	latest := latestTerm(all[t.ID])
	if _, err := store.Coll(models.CollTrainees).UpdateOne(ctx, bson.M{"_id": t.ID}, bson.M{"$set": bson.M{
		"monthlyFee": latest.MonthlyFee, "status": latest.Status, "feeFromMonth": latest.BillingFromMonth, "updatedAt": now,
	}}); err != nil {
		return nil, err
	}
	return &term, nil
}

func (h *traineeHandler) terms(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	cur, err := h.store.Coll(models.CollTerms).Find(ctx, bson.M{"trainee": id},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return err
	}
	out := []models.SubscriptionTerm{}
	if err := cur.All(ctx, &out); err != nil {
		return err
	}
	return c.JSON(out)
}

// addTerms is POST /trainees/:id/terms — an explicit, effective-dated fee or
// status change with its own billing start month.
func (h *traineeHandler) addTerms(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in termsChange
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	t, err := loadTrainee(ctx, h.store, id.Hex(), "id")
	if err != nil {
		return notFound("trainee")
	}
	var term *models.SubscriptionTerm
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		term, err = applyTermsChange(tx, h.store, t, in, actorOf(c))
		return err
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(term)
}

func (h *traineeHandler) remove(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	if _, err := h.store.Coll(models.CollTrainees).DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true})
}
