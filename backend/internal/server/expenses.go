package server

import (
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
	g.Delete("/:id", h.remove)
	r.Get("/financials", h.financials)
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

func (h *expenseHandler) remove(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	if _, err := h.store.Coll(models.CollExpenses).DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true})
}

// financials reports income (payments) vs outgoings (expenses) for a period,
// with breakdowns by payment type and expense category.
func (h *expenseHandler) financials(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	from, to := monthOrRange(c)

	pcur, err := h.store.Coll(models.CollPayments).Find(ctx, bson.M{"date": bson.M{"$gte": from, "$lt": to}})
	if err != nil {
		return err
	}
	var payments []models.Payment
	if err := pcur.All(ctx, &payments); err != nil {
		return err
	}
	ecur, err := h.store.Coll(models.CollExpenses).Find(ctx, bson.M{"date": bson.M{"$gte": from, "$lt": to}})
	if err != nil {
		return err
	}
	var expenses []models.Expense
	if err := ecur.All(ctx, &expenses); err != nil {
		return err
	}

	// Inventory sales are shop income too. Count them from the sales collection
	// (skip any legacy sale already mirrored as a payment to avoid double-count).
	scur, err := h.store.Coll(models.CollSales).Find(ctx, bson.M{"date": bson.M{"$gte": from, "$lt": to}})
	if err != nil {
		return err
	}
	var sales []models.Sale
	if err := scur.All(ctx, &sales); err != nil {
		return err
	}

	var income, outgoings float64
	byType := map[string]float64{}
	for _, p := range payments {
		income += p.Amount
		byType[p.Type] += p.Amount
	}
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
	return c.JSON(fiber.Map{
		"income":     income,
		"outgoings":  outgoings,
		"net":        income - outgoings,
		"byType":     byType,
		"byCategory": byCategory,
		"from":       from,
		"to":         to,
	})
}
