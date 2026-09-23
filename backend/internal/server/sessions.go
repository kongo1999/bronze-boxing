package server

import (
	"context"
	"fmt"
	"net/http"
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

var (
	sessionTypes    = []string{models.SessionGroup, models.SessionPrivate}
	sessionStatuses = []string{models.SessScheduled, models.SessCompleted, models.SessCancelled}
	attendStatuses  = []string{models.AttendBooked, models.AttendAttended, models.AttendNoShow}
)

// buildAttendees resolves attendee trainee ids + names from input. Every id
// must be a trainee that exists, and nobody may be booked twice.
func (h *sessionHandler) buildAttendees(ctx context.Context, in []attendeeInput) ([]models.Attendee, error) {
	ids := make([]primitive.ObjectID, 0, len(in))
	seen := map[primitive.ObjectID]bool{}
	for _, a := range in {
		id, err := primitive.ObjectIDFromHex(a.Trainee)
		if err != nil {
			return nil, apiErr(http.StatusBadRequest, CodeTraineeNotFound, "unknown trainee in attendees").withField("attendees")
		}
		if seen[id] {
			return nil, apiErr(http.StatusBadRequest, CodeDuplicate, "a trainee is listed twice in attendees").withField("attendees")
		}
		seen[id] = true
		if err := oneOf("attendees.status", defaultStr(a.Status, models.AttendBooked), attendStatuses...); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	names := traineeNames(ctx, h.store, ids)
	out := make([]models.Attendee, 0, len(in))
	for i, a := range in {
		name, ok := names[ids[i]]
		if !ok {
			return nil, apiErr(http.StatusBadRequest, CodeTraineeNotFound, "a booked trainee no longer exists").withField("attendees")
		}
		out = append(out, models.Attendee{
			Trainee:     ids[i],
			TraineeName: name,
			Status:      defaultStr(a.Status, models.AttendBooked),
		})
	}
	return out, nil
}

func (h *sessionHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	filter := bson.M{}
	if c.Query("from") != "" || c.Query("to") != "" {
		from, to, err := parseRange(c)
		if err != nil {
			return err
		}
		filter["start"] = bson.M{"$gte": from, "$lt": to}
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
		return badField("body", "invalid body")
	}
	if strings.TrimSpace(in.Title) == "" {
		return badField("title", "title is required")
	}
	if in.Start == nil {
		return badField("start", "start is required")
	}
	if err := sanityTime("start", *in.Start); err != nil {
		return err
	}
	if in.DurationMin <= 0 || in.DurationMin > 24*60 {
		return badField("durationMin", "duration must be between 1 and 1440 minutes")
	}
	if in.Capacity < 0 {
		return badField("capacity", "capacity can't be negative (0 means unlimited)")
	}
	if derefF64(in.Fee) < 0 {
		return badField("fee", "fee can't be negative")
	}
	typ := defaultStr(in.Type, models.SessionGroup)
	status := defaultStr(in.Status, models.SessScheduled)
	if err := oneOf("type", typ, sessionTypes...); err != nil {
		return err
	}
	if err := oneOf("status", status, sessionStatuses...); err != nil {
		return err
	}
	attendees, err := h.buildAttendees(ctx, in.Attendees)
	if err != nil {
		return err
	}
	now := time.Now()
	s := models.Session{
		Title:       strings.TrimSpace(in.Title),
		Type:        typ,
		Start:       *in.Start,
		DurationMin: in.DurationMin,
		Location:    derefStr(in.Location),
		Capacity:    in.Capacity,
		Fee:         round2(derefF64(in.Fee)),
		Status:      status,
		Attendees:   attendees,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if clash, err := h.findOverlap(ctx, s.Type, s.Start, s.DurationMin, nil); err != nil {
		return err
	} else if clash != nil {
		return apiErr(http.StatusConflict, CodeScheduleClash, overlapMsg(s.Type, clash))
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
		return badField("body", "invalid body")
	}
	set := bson.M{"updatedAt": time.Now()}
	if in.Title != "" {
		set["title"] = strings.TrimSpace(in.Title)
	}
	if in.Type != "" {
		if err := oneOf("type", in.Type, sessionTypes...); err != nil {
			return err
		}
		set["type"] = in.Type
	}
	if in.Start != nil {
		if err := sanityTime("start", *in.Start); err != nil {
			return err
		}
		set["start"] = *in.Start
	}
	if in.DurationMin < 0 || in.DurationMin > 24*60 {
		return badField("durationMin", "duration must be between 1 and 1440 minutes")
	}
	if in.DurationMin > 0 {
		set["durationMin"] = in.DurationMin
	}
	if in.Capacity < 0 {
		return badField("capacity", "capacity can't be negative (0 means unlimited)")
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
			return badField("fee", "fee can't be negative")
		}
		set["fee"] = *in.Fee
	}
	if in.Status != "" {
		if err := oneOf("status", in.Status, sessionStatuses...); err != nil {
			return err
		}
		set["status"] = in.Status
	}
	if in.Attendees != nil {
		attendees, err := h.buildAttendees(ctx, in.Attendees)
		if err != nil {
			return err
		}
		set["attendees"] = attendees
	}
	// Re-check overlaps only when a time-affecting field moved — a status-only
	// or attendee-only edit can't create a clash and shouldn't pay for the query.
	if in.Start != nil || in.Type != "" || in.DurationMin > 0 {
		var cur models.Session
		if err := h.store.Coll(models.CollSessions).FindOne(ctx, bson.M{"_id": id}).Decode(&cur); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "session not found")
		}
		typ, start, dur := cur.Type, cur.Start, cur.DurationMin
		if in.Type != "" {
			typ = in.Type
		}
		if in.Start != nil {
			start = *in.Start
		}
		if in.DurationMin > 0 {
			dur = in.DurationMin
		}
		if clash, err := h.findOverlap(ctx, typ, start, dur, &id); err != nil {
			return err
		} else if clash != nil {
			return apiErr(http.StatusConflict, CodeScheduleClash, overlapMsg(typ, clash))
		}
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
		return badField("body", "invalid body")
	}
	status := defaultStr(in.Status, models.AttendBooked)
	if err := oneOf("status", status, attendStatuses...); err != nil {
		return err
	}
	trainee, err := loadTrainee(ctx, h.store, in.Trainee, "trainee")
	if err != nil {
		return err
	}
	tid := trainee.ID
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
		push, err := coll.UpdateOne(ctx,
			bson.M{"_id": id, "attendees.trainee": bson.M{"$ne": tid}},
			bson.M{
				"$push": bson.M{"attendees": models.Attendee{Trainee: tid, TraineeName: trainee.Name, Status: status}},
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
	Time        string          `json:"time"`     // "HH:MM" studio wall clock
	DurationMin int             `json:"durationMin"`
	Location    string          `json:"location"`
	Capacity    int             `json:"capacity"`
	Fee         float64         `json:"fee"`
	FromDay     string          `json:"fromDay"` // studio-local YYYY-MM-DD (preferred)
	ToDay       string          `json:"toDay"`   // inclusive
	From        *time.Time      `json:"from"`    // legacy: UTC-midnight instants of the days
	To          *time.Time      `json:"to"`
	Attendees   []attendeeInput `json:"attendees"`
}

// spec turns the request into a recurrence, accepting the legacy from/to
// instants (the old form sent each day's UTC midnight).
func (in recurringInput) spec() seriesSpec {
	s := seriesSpec{Weekdays: in.Weekdays, Time: defaultStr(in.Time, "18:00"), FromDay: in.FromDay, ToDay: in.ToDay}
	if s.FromDay == "" && in.From != nil {
		s.FromDay = in.From.UTC().Format("2006-01-02")
	}
	if s.ToDay == "" && in.To != nil {
		s.ToDay = in.To.UTC().Format("2006-01-02")
	}
	return s
}

// recurring generates a series of sessions on the given weekdays at the
// given studio time across [fromDay, toDay], linked by a shared seriesId.
func (h *sessionHandler) recurring(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()

	var in recurringInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	if strings.TrimSpace(in.Title) == "" {
		return badField("title", "title is required")
	}
	if in.DurationMin <= 0 || in.DurationMin > 24*60 {
		return badField("durationMin", "duration must be between 1 and 1440 minutes")
	}
	typ := defaultStr(in.Type, models.SessionGroup)
	if err := oneOf("type", typ, sessionTypes...); err != nil {
		return err
	}
	starts, err := in.spec().occurrences()
	if err != nil {
		return err
	}
	attendees, err := h.buildAttendees(ctx, in.Attendees)
	if err != nil {
		return err
	}
	seriesID := primitive.NewObjectID().Hex()
	now := time.Now()
	docs := make([]any, 0, len(starts))
	preview := make([]models.Session, 0, len(starts))
	for _, start := range starts {
		// Every occurrence starts scheduled — even one dated in the past. A
		// class counts as completed only when the coach marks it so.
		s := models.Session{
			Title:       strings.TrimSpace(in.Title),
			Type:        typ,
			Start:       start,
			DurationMin: in.DurationMin,
			Location:    in.Location,
			Capacity:    in.Capacity,
			Fee:         round2(in.Fee),
			SeriesID:    seriesID,
			Status:      models.SessScheduled,
			Attendees:   attendees,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		docs = append(docs, s)
		preview = append(preview, s)
	}
	if len(docs) == 0 {
		return badField("weekdays", "none of the chosen weekdays fall between those dates")
	}
	// Reject the whole series if any generated session would clash — no partial
	// inserts. Sessions within a series never overlap each other (one per day),
	// so we only check against existing bookings.
	var conflicts []time.Time
	for i := range preview {
		clash, err := h.findOverlap(ctx, preview[i].Type, preview[i].Start, preview[i].DurationMin, nil)
		if err != nil {
			return err
		}
		if clash != nil {
			conflicts = append(conflicts, preview[i].Start)
		}
	}
	if len(conflicts) > 0 {
		dates := make([]string, len(conflicts))
		for i, t := range conflicts {
			dates[i] = t.In(time.Local).Format(time.RFC3339)
		}
		return apiErr(http.StatusConflict, CodeScheduleClash, fmt.Sprintf(
			"%d session(s) in this series overlap existing bookings (first: %s). %s",
			len(conflicts), conflicts[0].In(time.Local).Format("Mon Jan 2, 3:04 PM"), ruleHint(typ))).
			withDetails(map[string]any{"conflicts": dates})
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

// findOverlap returns the first existing session that a booking of the given
// type/start/duration would illegally overlap, or nil if the slot is clear.
//
// Rule: a group class needs exclusive time — nothing (group or private) may
// overlap it. A private (PT) session may overlap other private sessions, but
// never a group class. So a new private booking only needs to be checked
// against group classes; a new group booking against everything.
//
// Cancelled sessions never block. excludeID skips a session against itself
// (used when editing). We over-fetch a 24h lower window and refine in Go since
// per-session durations aren't expressible in the Mongo range query.
func (h *sessionHandler) findOverlap(ctx context.Context, typ string, start time.Time, dur int, excludeID *primitive.ObjectID) (*models.Session, error) {
	end := start.Add(time.Duration(dur) * time.Minute)
	filter := bson.M{
		"status": bson.M{"$ne": models.SessCancelled},
		"start":  bson.M{"$lt": end, "$gte": start.Add(-24 * time.Hour)},
	}
	if typ != models.SessionGroup {
		filter["type"] = models.SessionGroup // a private booking only clashes with group classes
	}
	if excludeID != nil {
		filter["_id"] = bson.M{"$ne": *excludeID}
	}
	cur, err := h.store.Coll(models.CollSessions).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var candidates []models.Session
	if err := cur.All(ctx, &candidates); err != nil {
		return nil, err
	}
	for i := range candidates {
		if overlapForbidden(typ, start, dur, candidates[i]) {
			return &candidates[i], nil
		}
	}
	return nil, nil
}

// overlapForbidden decides, in isolation, whether a booking of newType over
// [start, start+dur) illegally collides with an existing session. Pure and
// DB-free so the rule can be unit-tested. Rule: two private sessions may
// overlap; any overlap involving a group class is forbidden. Cancelled
// sessions never collide.
func overlapForbidden(newType string, start time.Time, dur int, ex models.Session) bool {
	if ex.Status == models.SessCancelled {
		return false
	}
	if newType != models.SessionGroup && ex.Type != models.SessionGroup {
		return false // private vs private is allowed to overlap
	}
	newEnd := start.Add(time.Duration(dur) * time.Minute)
	exEnd := ex.Start.Add(time.Duration(ex.DurationMin) * time.Minute)
	return start.Before(exEnd) && ex.Start.Before(newEnd) // half-open overlap
}

func ruleHint(typ string) string {
	if typ == models.SessionGroup {
		return "Group classes can't overlap other sessions."
	}
	return "Private sessions can't overlap a group class."
}

func overlapMsg(newType string, clash *models.Session) string {
	// clash.Start comes back from Mongo in UTC; render it in the studio's wall
	// clock (time.Local is set to STUDIO_TZ) so the time matches the schedule.
	when := clash.Start.Local().Format("Mon Jan 2, 3:04 PM")
	if newType == models.SessionGroup {
		return fmt.Sprintf("Group classes need exclusive time — this overlaps %q at %s.", clash.Title, when)
	}
	return fmt.Sprintf("This overlaps the group class %q at %s — private sessions can't overlap a group class.", clash.Title, when)
}
