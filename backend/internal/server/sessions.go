package server

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

type sessionHandler struct{ store *db.Store }

func registerSessions(r fiber.Router, store *db.Store) {
	h := &sessionHandler{store}
	g := r.Group("/sessions")
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Post("/recurring", h.recurring)
	g.Get("/:id", h.get)
	g.Put("/:id", h.update)
	g.Delete("/:id", h.remove)
	g.Patch("/:id/attendance", h.attendance)
}

type attendeeInput struct {
	Trainee string `json:"trainee"`
	Status  string `json:"status"`
}

type sessionInput struct {
	Title       string          `json:"title"`
	Type        string          `json:"type"`
	Start       *time.Time      `json:"start"`
	DurationMin int             `json:"durationMin"`
	Location    *string         `json:"location"`
	Capacity    int             `json:"capacity"`
	Fee         *float64        `json:"fee"`
	Status      string          `json:"status"`
	Attendees   []attendeeInput `json:"attendees"`
}

// buildAttendees resolves attendee trainee ids + names from input.
func (h *sessionHandler) buildAttendees(ctx context.Context, in []attendeeInput) []models.Attendee {
	ids := make([]primitive.ObjectID, 0, len(in))
	parsed := make([]primitive.ObjectID, len(in))
	for i, a := range in {
		id, err := primitive.ObjectIDFromHex(a.Trainee)
		if err == nil {
			parsed[i] = id
			ids = append(ids, id)
		}
	}
	names := traineeNames(ctx, h.store, ids)
	out := make([]models.Attendee, 0, len(in))
	for i, a := range in {
		if parsed[i].IsZero() {
			continue
		}
		out = append(out, models.Attendee{
			Trainee:     parsed[i],
			TraineeName: names[parsed[i]],
			Status:      defaultStr(a.Status, models.AttendBooked),
		})
	}
	return out
}

func (h *sessionHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	filter := bson.M{}
	if c.Query("from") != "" || c.Query("to") != "" {
		from, to := parseRange(c)
		filter["start"] = bson.M{"$gte": from, "$lte": to}
	}
	cur, err := h.store.Coll(models.CollSessions).
		Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "start", Value: 1}}))
	if err != nil {
		return err
	}
	out := []models.Session{}
	if err := cur.All(ctx, &out); err != nil {
		return err
	}
	return c.JSON(out)
}

func (h *sessionHandler) get(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var s models.Session
	if err := h.store.Coll(models.CollSessions).FindOne(ctx, bson.M{"_id": id}).Decode(&s); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "session not found")
	}
	return c.JSON(s)
}

func (h *sessionHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	var in sessionInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if strings.TrimSpace(in.Title) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "title is required")
	}
	if in.Start == nil {
		return fiber.NewError(fiber.StatusBadRequest, "start is required")
	}
	if in.DurationMin < 0 || in.Capacity < 0 || derefF64(in.Fee) < 0 {
		return fiber.NewError(fiber.StatusBadRequest, "duration, capacity and fee must be non-negative")
	}
	now := time.Now()
	s := models.Session{
		Title:       strings.TrimSpace(in.Title),
		Type:        defaultStr(in.Type, models.SessionGroup),
		Start:       *in.Start,
		DurationMin: in.DurationMin,
		Location:    derefStr(in.Location),
		Capacity:    in.Capacity,
		Fee:         derefF64(in.Fee),
		Status:      defaultStr(in.Status, models.SessScheduled),
		Attendees:   h.buildAttendees(ctx, in.Attendees),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	res, err := h.store.Coll(models.CollSessions).InsertOne(ctx, s)
	if err != nil {
		return err
	}
	s.ID = res.InsertedID.(primitive.ObjectID)
	return c.Status(fiber.StatusCreated).JSON(s)
}

func (h *sessionHandler) update(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in sessionInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	set := bson.M{"updatedAt": time.Now()}
	if in.Title != "" {
		set["title"] = strings.TrimSpace(in.Title)
	}
	if in.Type != "" {
		set["type"] = in.Type
	}
	if in.Start != nil {
		set["start"] = *in.Start
	}
	if in.DurationMin > 0 {
		set["durationMin"] = in.DurationMin
	}
	// Only touch location/fee when the caller actually sent them — otherwise a
	// partial update (e.g. just changing the time) would blank the location and
	// zero the fee.
	if in.Location != nil {
		set["location"] = *in.Location
	}
	if in.Capacity > 0 {
		set["capacity"] = in.Capacity
	}
	if in.Fee != nil {
		if *in.Fee < 0 {
			return fiber.NewError(fiber.StatusBadRequest, "fee must be non-negative")
		}
		set["fee"] = *in.Fee
	}
	if in.Status != "" {
		set["status"] = in.Status
	}
	if in.Attendees != nil {
		set["attendees"] = h.buildAttendees(ctx, in.Attendees)
	}
	if _, err := h.store.Coll(models.CollSessions).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
		return err
	}
	var s models.Session
	if err := h.store.Coll(models.CollSessions).FindOne(ctx, bson.M{"_id": id}).Decode(&s); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "session not found")
	}
	return c.JSON(s)
}

func (h *sessionHandler) remove(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	if _, err := h.store.Coll(models.CollSessions).DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true})
}

type attendanceInput struct {
	Trainee string `json:"trainee"`
	Status  string `json:"status"`
}

// attendance sets one attendee's status (booked/attended/no_show), adding the
// attendee to the session if not already present.
func (h *sessionHandler) attendance(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in attendanceInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	tid, err := primitive.ObjectIDFromHex(in.Trainee)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid trainee id")
	}
	status := defaultStr(in.Status, models.AttendBooked)
	if status != models.AttendBooked && status != models.AttendAttended && status != models.AttendNoShow {
		return fiber.NewError(fiber.StatusBadRequest, "invalid attendance status")
	}
	coll := h.store.Coll(models.CollSessions)
	now := time.Now()

	// 1) If the attendee already exists, update in place atomically (positional $).
	res, err := coll.UpdateOne(ctx,
		bson.M{"_id": id, "attendees.trainee": tid},
		bson.M{"$set": bson.M{"attendees.$.status": status, "updatedAt": now}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		// 2) Not present — push a new attendee, but only while still absent. This
		// avoids the read-modify-write race where two concurrent calls would each
		// rewrite the whole array and lose one update.
		names := traineeNames(ctx, h.store, []primitive.ObjectID{tid})
		push, err := coll.UpdateOne(ctx,
			bson.M{"_id": id, "attendees.trainee": bson.M{"$ne": tid}},
			bson.M{
				"$push": bson.M{"attendees": models.Attendee{Trainee: tid, TraineeName: names[tid], Status: status}},
				"$set":  bson.M{"updatedAt": now},
			})
		if err != nil {
			return err
		}
		if push.MatchedCount == 0 {
			// Either the session is gone, or a concurrent call added the attendee
			// between our two writes. Distinguish, and if it was a race, set status.
			if cnt, _ := coll.CountDocuments(ctx, bson.M{"_id": id}); cnt == 0 {
				return fiber.NewError(fiber.StatusNotFound, "session not found")
			}
			if _, err := coll.UpdateOne(ctx,
				bson.M{"_id": id, "attendees.trainee": tid},
				bson.M{"$set": bson.M{"attendees.$.status": status, "updatedAt": now}}); err != nil {
				return err
			}
		}
	}

	var s models.Session
	if err := coll.FindOne(ctx, bson.M{"_id": id}).Decode(&s); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "session not found")
	}
	return c.JSON(s)
}

type recurringInput struct {
	Title       string          `json:"title"`
	Type        string          `json:"type"`
	Weekdays    []int           `json:"weekdays"` // 0=Sunday .. 6=Saturday
	Time        string          `json:"time"`     // "HH:MM"
	DurationMin int             `json:"durationMin"`
	Location    string          `json:"location"`
	Capacity    int             `json:"capacity"`
	Fee         float64         `json:"fee"`
	From        *time.Time      `json:"from"`
	To          *time.Time      `json:"to"`
	Attendees   []attendeeInput `json:"attendees"`
}

// recurring generates a series of sessions across [from, to] on the given
// weekdays at the given time, linked by a shared seriesId.
func (h *sessionHandler) recurring(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	var in recurringInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if strings.TrimSpace(in.Title) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "title is required")
	}
	if in.From == nil || in.To == nil {
		return fiber.NewError(fiber.StatusBadRequest, "from and to are required")
	}
	if len(in.Weekdays) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "at least one weekday is required")
	}
	hh, mm := 18, 0
	if parts := strings.Split(in.Time, ":"); len(parts) == 2 {
		hh = atoiDefault(parts[0], 18)
		mm = atoiDefault(parts[1], 0)
	}
	want := map[int]bool{}
	for _, d := range in.Weekdays {
		want[d] = true
	}
	attendees := h.buildAttendees(ctx, in.Attendees)
	seriesID := primitive.NewObjectID().Hex()

	var docs []any
	var preview []models.Session
	now := time.Now()
	day := time.Date(in.From.Year(), in.From.Month(), in.From.Day(), 0, 0, 0, 0, time.Local)
	end := time.Date(in.To.Year(), in.To.Month(), in.To.Day(), 0, 0, 0, 0, time.Local)
	for !day.After(end) {
		if want[int(day.Weekday())] {
			start := time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, time.Local)
			status := models.SessScheduled
			if start.Before(now) {
				status = models.SessCompleted
			}
			s := models.Session{
				Title:       strings.TrimSpace(in.Title),
				Type:        defaultStr(in.Type, models.SessionGroup),
				Start:       start,
				DurationMin: in.DurationMin,
				Location:    in.Location,
				Capacity:    in.Capacity,
				Fee:         in.Fee,
				SeriesID:    seriesID,
				Status:      status,
				Attendees:   attendees,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			docs = append(docs, s)
			preview = append(preview, s)
		}
		day = day.AddDate(0, 0, 1)
	}
	if len(docs) == 0 {
		return c.JSON(fiber.Map{"created": 0, "seriesId": seriesID, "sessions": []models.Session{}})
	}
	if _, err := h.store.Coll(models.CollSessions).InsertMany(ctx, docs); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"created":  len(docs),
		"seriesId": seriesID,
		"sessions": preview,
	})
}
