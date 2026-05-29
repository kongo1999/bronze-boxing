package server

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

type reminderHandler struct{ store *db.Store }

func registerReminders(r fiber.Router, store *db.Store) {
	h := &reminderHandler{store}
	g := r.Group("/reminders")
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Put("/:id", h.update)
	g.Delete("/:id", h.remove)
}

type reminderInput struct {
	Title    string     `json:"title"`
	DueDate  *time.Time `json:"dueDate"`
	Priority string     `json:"priority"`
	Done     *bool      `json:"done"`
}

func (h *reminderHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	filter := bson.M{}
	if c.Query("from") != "" || c.Query("to") != "" {
		from, to := parseRange(c)
		filter["dueDate"] = bson.M{"$gte": from, "$lte": to}
	}
	cur, err := h.store.Coll(models.CollReminders).
		Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "dueDate", Value: 1}}))
	if err != nil {
		return err
	}
	out := []models.Reminder{}
	if err := cur.All(ctx, &out); err != nil {
		return err
	}
	return c.JSON(out)
}

func (h *reminderHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	var in reminderInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return fiber.NewError(fiber.StatusBadRequest, "title is required")
	}
	due := time.Now()
	if in.DueDate != nil {
		due = *in.DueDate
	}
	r := models.Reminder{
		Title:     in.Title,
		DueDate:   due,
		Priority:  defaultStr(in.Priority, models.PriorityNormal),
		Done:      in.Done != nil && *in.Done,
		CreatedAt: time.Now(),
	}
	res, err := h.store.Coll(models.CollReminders).InsertOne(ctx, r)
	if err != nil {
		return err
	}
	r.ID = res.InsertedID.(primitive.ObjectID)
	return c.Status(fiber.StatusCreated).JSON(r)
}

func (h *reminderHandler) update(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in reminderInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	set := bson.M{}
	if in.Title != "" {
		set["title"] = strings.TrimSpace(in.Title)
	}
	if in.DueDate != nil {
		set["dueDate"] = *in.DueDate
	}
	if in.Priority != "" {
		set["priority"] = in.Priority
	}
	if in.Done != nil {
		set["done"] = *in.Done
	}
	if len(set) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "nothing to update")
	}
	_, err = h.store.Coll(models.CollReminders).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	if err != nil {
		return err
	}
	var r models.Reminder
	if err := h.store.Coll(models.CollReminders).FindOne(ctx, bson.M{"_id": id}).Decode(&r); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "reminder not found")
	}
	return c.JSON(r)
}

func (h *reminderHandler) remove(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	if _, err := h.store.Coll(models.CollReminders).DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true})
}
