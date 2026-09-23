package server

import (
	"context"
	"errors"
	"net/http"
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

type reminderHandler struct{ store *db.Store }

func registerReminders(r fiber.Router, store *db.Store) {
	h := &reminderHandler{store}
	g := r.Group("/reminders")
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Get("/:id", h.get)
	g.Put("/:id", h.update)
	g.Post("/:id/snooze", h.snooze)
	g.Delete("/:id", h.remove)
}

var (
	priorities      = []string{models.PriorityLow, models.PriorityNormal, models.PriorityHigh}
	recurrences     = []string{"", "daily", "weekly", "monthly"}
	reminderRelated = []string{"", "trainee", "session", "item"}
)

type reminderInput struct {
	Title    string     `json:"title"`
	DueDay   string     `json:"dueDay"`  // studio-local YYYY-MM-DD (preferred)
	DueDate  *time.Time `json:"dueDate"` // legacy: an instant, mapped to its studio day
	Priority string     `json:"priority"`
	Done     *bool      `json:"done"`
	// Links and repeats. RelatedID "" clears the link on update.
	RelatedType *string `json:"relatedType"`
	RelatedID   *string `json:"relatedId"`
	Recurrence  *string `json:"recurrence"`
}

// withDay fills DueDay for reminders written before it existed.
func withDay(r models.Reminder) models.Reminder {
	if r.DueDay == "" {
		r.DueDay = models.DateKey(r.DueDate)
	}
	return r
}

// effectiveDay is when a reminder next wants attention: its due day, or the
// day it was snoozed to if later.
func effectiveDay(r models.Reminder) string {
	r = withDay(r)
	if r.SnoozedUntil > r.DueDay {
		return r.SnoozedUntil
	}
	return r.DueDay
}

func (h *reminderHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	filter := bson.M{}
	if c.Query("from") != "" || c.Query("to") != "" {
		from, to, err := parseRange(c)
		if err != nil {
			return err
		}
		filter["dueDate"] = bson.M{"$gte": from, "$lt": to}
	}
	switch c.Query("status") {
	case "open":
		filter["done"] = false
	case "done":
		filter["done"] = true
	}
	if p := c.Query("priority"); p != "" {
		if err := oneOf("priority", p, priorities...); err != nil {
			return err
		}
		filter["priority"] = p
	}
	if rt, rid := c.Query("relatedType"), c.Query("relatedId"); rt != "" && rid != "" {
		oid, err := parseOID(rid, "relatedId")
		if err != nil {
			return err
		}
		filter["relatedType"], filter["relatedId"] = rt, oid
	}
	cur, err := h.store.Coll(models.CollReminders).
		Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "dueDate", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return err
	}
	out := []models.Reminder{}
	if err := cur.All(ctx, &out); err != nil {
		return err
	}
	for i := range out {
		out[i] = withDay(out[i])
	}
	return c.JSON(out)
}

func findReminder(ctx context.Context, store *db.Store, id primitive.ObjectID) (models.Reminder, error) {
	var r models.Reminder
	if err := store.Coll(models.CollReminders).FindOne(ctx, bson.M{"_id": id}).Decode(&r); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return r, notFound("reminder")
		}
		return r, err
	}
	return withDay(r), nil
}

func (h *reminderHandler) get(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	r, err := findReminder(ctx, h.store, id)
	if err != nil {
		return err
	}
	return c.JSON(r)
}

// resolveDueDay accepts the preferred date-only dueDay, or a legacy instant.
func resolveDueDay(in reminderInput) (string, time.Time, error) {
	if in.DueDay != "" {
		start, err := models.ParseDay(in.DueDay)
		if err != nil {
			return "", time.Time{}, badField("dueDay", "dueDay must be YYYY-MM-DD")
		}
		return in.DueDay, start, nil
	}
	if in.DueDate != nil {
		if err := sanityTime("dueDate", *in.DueDate); err != nil {
			return "", time.Time{}, err
		}
		day := models.DateKey(*in.DueDate)
		start, _ := models.ParseDay(day)
		return day, start, nil
	}
	day := models.DateKey(time.Now())
	start, _ := models.ParseDay(day)
	return day, start, nil
}

// resolveRelated validates an optional record link and snapshots its label.
func (h *reminderHandler) resolveRelated(ctx context.Context, typ, hex string) (string, *primitive.ObjectID, string, error) {
	if err := oneOf("relatedType", typ, reminderRelated...); err != nil {
		return "", nil, "", err
	}
	if typ == "" || hex == "" {
		return "", nil, "", nil
	}
	id, err := parseOID(hex, "relatedId")
	if err != nil {
		return "", nil, "", err
	}
	var doc struct {
		Name  string `bson:"name"`
		Title string `bson:"title"`
	}
	coll := map[string]string{"trainee": models.CollTrainees, "session": models.CollSessions, "item": models.CollInventory}[typ]
	if err := h.store.Coll(coll).FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
		return "", nil, "", apiErr(http.StatusBadRequest, CodeNotFound, "the linked "+typ+" no longer exists").withField("relatedId")
	}
	return typ, &id, defaultStr(doc.Name, doc.Title), nil
}

func (h *reminderHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	var in reminderInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return badField("title", "title is required")
	}
	day, start, err := resolveDueDay(in)
	if err != nil {
		return err
	}
	priority := defaultStr(in.Priority, models.PriorityNormal)
	if err := oneOf("priority", priority, priorities...); err != nil {
		return err
	}
	rec := derefStr(in.Recurrence)
	if err := oneOf("recurrence", rec, recurrences...); err != nil {
		return err
	}
	rt, rid, label, err := h.resolveRelated(ctx, derefStr(in.RelatedType), derefStr(in.RelatedID))
	if err != nil {
		return err
	}
	r := models.Reminder{
		ID:           primitive.NewObjectID(),
		Title:        in.Title,
		DueDay:       day,
		DueDate:      start,
		Priority:     priority,
		Done:         in.Done != nil && *in.Done,
		RelatedType:  rt,
		RelatedID:    rid,
		RelatedLabel: label,
		Recurrence:   rec,
		CreatedAt:    time.Now(),
	}
	if rec != "" {
		r.SeriesID = r.ID.Hex()
	}
	if r.Done {
		now := time.Now()
		r.DoneAt = &now
	}
	if _, err := h.store.Coll(models.CollReminders).InsertOne(ctx, r); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(r)
}

// nextDay advances a due day by one recurrence step.
func nextDay(day, rec string) string {
	t, err := models.ParseDay(day)
	if err != nil {
		return day
	}
	switch rec {
	case "daily":
		t = t.AddDate(0, 0, 1)
	case "weekly":
		t = t.AddDate(0, 0, 7)
	case "monthly":
		t = t.AddDate(0, 1, 0)
	}
	return models.DateKey(t)
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
		return badField("body", "invalid body")
	}
	prev, err := findReminder(ctx, h.store, id)
	if err != nil {
		return err
	}
	set := bson.M{}
	unset := bson.M{}
	if t := strings.TrimSpace(in.Title); t != "" {
		set["title"] = t
	}
	if in.DueDay != "" || in.DueDate != nil {
		day, start, err := resolveDueDay(in)
		if err != nil {
			return err
		}
		set["dueDay"], set["dueDate"] = day, start
		unset["snoozedUntil"] = "" // a new due day replaces any snooze
	}
	if in.Priority != "" {
		if err := oneOf("priority", in.Priority, priorities...); err != nil {
			return err
		}
		set["priority"] = in.Priority
	}
	if in.Recurrence != nil {
		if err := oneOf("recurrence", *in.Recurrence, recurrences...); err != nil {
			return err
		}
		set["recurrence"] = *in.Recurrence
		if *in.Recurrence != "" && prev.SeriesID == "" {
			set["seriesId"] = prev.ID.Hex()
		}
	}
	if in.RelatedType != nil || in.RelatedID != nil {
		rt, rid, label, err := h.resolveRelated(ctx, derefStr(in.RelatedType), derefStr(in.RelatedID))
		if err != nil {
			return err
		}
		if rid == nil {
			unset["relatedType"], unset["relatedId"], unset["relatedLabel"] = "", "", ""
		} else {
			set["relatedType"], set["relatedId"], set["relatedLabel"] = rt, rid, label
		}
	}
	completing := in.Done != nil && *in.Done && !prev.Done
	reopening := in.Done != nil && !*in.Done && prev.Done
	if in.Done != nil {
		set["done"] = *in.Done
		if *in.Done {
			set["doneAt"] = time.Now()
		} else {
			unset["doneAt"] = ""
		}
	}
	if len(set) == 0 && len(unset) == 0 {
		return badField("body", "nothing to update")
	}
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		filter := bson.M{"_id": id}
		if completing {
			filter["done"] = false // complete (and spawn) at most once
		}
		upd := bson.M{}
		if len(set) > 0 {
			upd["$set"] = set
		}
		if len(unset) > 0 {
			upd["$unset"] = unset
		}
		res, err := h.store.Coll(models.CollReminders).UpdateOne(tx, filter, upd)
		if err != nil {
			return err
		}
		rec := prev.Recurrence
		if v, ok := set["recurrence"].(string); ok {
			rec = v
		}
		// Completing one instance of a repeating reminder schedules the next,
		// so the history of done instances stays and the chain keeps going.
		if completing && res.ModifiedCount == 1 && rec != "" && prev.NextID == nil {
			next := prev
			next.ID = primitive.NewObjectID()
			next.DueDay = nextDay(effectiveDay(prev), rec)
			next.DueDate, _ = models.ParseDay(next.DueDay)
			next.Done, next.DoneAt, next.SnoozedUntil, next.NextID = false, nil, "", nil
			next.Recurrence = rec
			next.SeriesID = defaultStr(prev.SeriesID, prev.ID.Hex())
			next.CreatedAt = time.Now()
			if t, ok := set["title"].(string); ok {
				next.Title = t
			}
			if _, err := h.store.Coll(models.CollReminders).InsertOne(tx, next); err != nil {
				return err
			}
			_, err := h.store.Coll(models.CollReminders).UpdateOne(tx, bson.M{"_id": id}, bson.M{"$set": bson.M{"nextId": next.ID}})
			return err
		}
		// Un-ticking by mistake withdraws the untouched next instance again.
		if reopening && prev.NextID != nil {
			del, err := h.store.Coll(models.CollReminders).DeleteOne(tx, bson.M{"_id": *prev.NextID, "done": false})
			if err != nil {
				return err
			}
			if del.DeletedCount == 1 {
				_, err = h.store.Coll(models.CollReminders).UpdateOne(tx, bson.M{"_id": id}, bson.M{"$unset": bson.M{"nextId": ""}})
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	r, err := findReminder(ctx, h.store, id)
	if err != nil {
		return err
	}
	return c.JSON(r)
}

// snooze pushes a reminder out of the way until a later day (tomorrow, next
// week, or an explicit day) without rewriting its original due day.
func (h *reminderHandler) snooze(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in struct {
		Until string `json:"until"`
		Days  int    `json:"days"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	until := in.Until
	if until == "" {
		if in.Days <= 0 || in.Days > 366 {
			return badField("days", "snooze by 1 to 366 days, or give until")
		}
		until = models.DateKey(time.Now().AddDate(0, 0, in.Days))
	}
	if _, err := models.ParseDay(until); err != nil {
		return badField("until", "until must be YYYY-MM-DD")
	}
	if until <= models.DateKey(time.Now()) {
		return badField("until", "snooze to a day after today")
	}
	if _, err := h.store.Coll(models.CollReminders).UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"snoozedUntil": until}}); err != nil {
		return err
	}
	r, err := findReminder(ctx, h.store, id)
	if err != nil {
		return err
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
