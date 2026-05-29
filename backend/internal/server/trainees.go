package server

import (
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
}

type traineeInput struct {
	Name       string  `json:"name"`
	Phone      string  `json:"phone"`
	SkillLevel string  `json:"skillLevel"`
	MonthlyFee float64 `json:"monthlyFee"`
	Status     string  `json:"status"`
	Notes      string  `json:"notes"`
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
		return fiber.NewError(fiber.StatusNotFound, "trainee not found")
	}
	return c.JSON(t)
}

func (h *traineeHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	var in traineeInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}
	now := time.Now()
	t := models.Trainee{
		Name:       in.Name,
		Phone:      in.Phone,
		SkillLevel: in.SkillLevel,
		MonthlyFee: in.MonthlyFee,
		Status:     defaultStr(in.Status, models.StatusActive),
		Notes:      in.Notes,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	res, err := h.store.Coll(models.CollTrainees).InsertOne(ctx, t)
	if err != nil {
		return err
	}
	t.ID = res.InsertedID.(primitive.ObjectID)
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
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	set := bson.M{
		"name":       strings.TrimSpace(in.Name),
		"phone":      in.Phone,
		"skillLevel": in.SkillLevel,
		"monthlyFee": in.MonthlyFee,
		"status":     defaultStr(in.Status, models.StatusActive),
		"notes":      in.Notes,
		"updatedAt":  time.Now(),
	}
	_, err = h.store.Coll(models.CollTrainees).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	if err != nil {
		return err
	}
	var t models.Trainee
	if err := h.store.Coll(models.CollTrainees).FindOne(ctx, bson.M{"_id": id}).Decode(&t); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "trainee not found")
	}
	return c.JSON(t)
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
