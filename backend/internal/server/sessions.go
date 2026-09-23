package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

type sessionHandler struct{ store *db.Store }

func registerSessions(r fiber.Router, store *db.Store) {
	h := &sessionHandler{store}
	g := r.Group("/sessions")
	g.Get("/", h.list)
	g.Post("/", h.create)
	// Static series/recurring routes come before /:id.
	g.Post("/recurring/preview", h.recurringPreview)
	g.Post("/recurring", h.recurring)
	g.Get("/series", h.seriesSummaries)
	g.Get("/series/:seriesId/progress", h.seriesProgress)
	g.Patch("/series/:seriesId", h.seriesEdit)
	g.Post("/series/:seriesId/extend", h.seriesExtend)
	g.Post("/series/:seriesId/end", h.seriesEnd)
	g.Get("/:id", h.get)
	g.Put("/:id", h.update)
	g.Delete("/:id", h.remove)
	g.Patch("/:id/attendance", h.attendance)
	g.Post("/:id/attendance/bulk", h.attendanceBulk)
}

type attendeeInput struct {
	Trainee string `json:"trainee"`
	Status  string `json:"status"`
	// PlanID: absent keeps the attendee's existing plan link (on update),
	// "" clears it, an id links the booking to that session plan.
	PlanID *string `json:"planId"`
}

type sessionInput struct {
	Title       string          `json:"title"`
	Type        string          `json:"type"`
	Start       *time.Time      `json:"start"`
	DurationMin int             `json:"durationMin"`
	Location    *string         `json:"location"`
	Capacity    *int            `json:"capacity"` // 0 = unlimited
	Fee         *float64        `json:"fee"`
	Status      string          `json:"status"`
	Attendees   []attendeeInput `json:"attendees"`
	// PlanOverride books past a plan's target (after the UI warned).
	PlanOverride bool `json:"planOverride"`
}

var (
	sessionTypes    = []string{models.SessionGroup, models.SessionPrivate}
	sessionStatuses = []string{models.SessScheduled, models.SessCompleted, models.SessCancelled}
	attendStatuses  = []string{models.AttendBooked, models.AttendAttended, models.AttendNoShow}
)

// occurrence is where and when a booking lands, for checking plan fit.
type occurrence struct {
	Start time.Time
	Type  string
}

// buildAttendees resolves attendee trainee ids + names from input. Every id
// must be a trainee that exists, nobody may be booked twice, and each plan
// link must fit (see linkPlans). prev supplies existing plan links to keep
// when the input leaves planId out.
func (h *sessionHandler) buildAttendees(ctx context.Context, in []attendeeInput, prev []models.Attendee) ([]models.Attendee, error) {
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
	prevPlan := map[primitive.ObjectID]*primitive.ObjectID{}
	for _, p := range prev {
		prevPlan[p.Trainee] = p.PlanID
	}
	out := make([]models.Attendee, 0, len(in))
	for i, a := range in {
		name, ok := names[ids[i]]
		if !ok {
			return nil, apiErr(http.StatusBadRequest, CodeTraineeNotFound, "a booked trainee no longer exists").withField("attendees")
		}
		att := models.Attendee{Trainee: ids[i], TraineeName: name, Status: defaultStr(a.Status, models.AttendBooked)}
		switch {
		case a.PlanID == nil:
			att.PlanID = prevPlan[ids[i]]
		case *a.PlanID != "":
			pid, err := parseOID(*a.PlanID, "planId")
			if err != nil {
				return nil, err
			}
			att.PlanID = &pid
		}
		out = append(out, att)
	}
	return out, nil
}

// planFitErr explains why a booking can't be credited to a plan, or nil.
func planFitErr(p models.SessionPlan, trainee primitive.ObjectID, occ occurrence) error {
	mismatch := func(msg string) error {
		return apiErr(http.StatusBadRequest, CodePlanMismatch, msg).withField("planId")
	}
	switch {
	case p.Trainee != trainee:
		return mismatch("that session plan belongs to another trainee")
	case p.Status != "active":
		return mismatch(fmt.Sprintf("the plan %q is %s", p.Title, p.Status))
	case p.SessionType != "" && p.SessionType != occ.Type:
		return mismatch(fmt.Sprintf("the plan %q is for %s sessions", p.Title, p.SessionType))
	}
	day := models.DateKey(occ.Start)
	if day < p.StartDate || (p.EndDate != "" && day > p.EndDate) {
		return mismatch(fmt.Sprintf("%s is outside the plan %q (%s – %s)", day, p.Title, p.StartDate, defaultStr(p.EndDate, "open")))
	}
	return nil
}

// linkPlans validates every newly linked plan against the occurrences it is
// being booked into, and — unless override — refuses to commit more
// bookings to a plan than its target has room for. Links that were already
// in place (unchanged) are not re-counted.
func (h *sessionHandler) linkPlans(ctx context.Context, sessionID *primitive.ObjectID, attendees []models.Attendee, prev []models.Attendee, occs []occurrence, override bool) error {
	had := map[primitive.ObjectID]primitive.ObjectID{}
	for _, p := range prev {
		if p.PlanID != nil {
			had[p.Trainee] = *p.PlanID
		}
	}
	perPlan := map[primitive.ObjectID]int{}
	for _, a := range attendees {
		if a.PlanID == nil || had[a.Trainee] == *a.PlanID {
			continue
		}
		plan, err := findPlan(ctx, h.store, *a.PlanID)
		if err != nil {
			return err
		}
		for _, o := range occs {
			if err := planFitErr(plan, a.Trainee, o); err != nil {
				return err
			}
		}
		perPlan[plan.ID] += len(occs)
	}
	if override {
		return nil
	}
	for pid, n := range perPlan {
		plan, err := findPlan(ctx, h.store, pid)
		if err != nil {
			return err
		}
		prog, err := planProgress(ctx, h.store, []models.SessionPlan{plan}, sessionID)
		if err != nil {
			return err
		}
		pr := prog[pid]
		committed := pr.Completed + pr.UpcomingBooked + pr.AttendanceNeeded
		if committed+n > plan.TargetCount {
			return apiErr(http.StatusConflict, CodePlanFull, fmt.Sprintf(
				"%s's plan %q has %d of %d sessions used or booked — this would add %d. Book anyway?",
				plan.TraineeName, plan.Title, committed, plan.TargetCount, n)).
				withField("planId").
				withDetails(map[string]any{"plan": plan.ID.Hex(), "target": plan.TargetCount, "committed": committed, "requested": n})
		}
	}
	return nil
}

func capacityErr(capacity, booked int) error {
	return apiErr(http.StatusConflict, CodeCapacity, fmt.Sprintf(
		"this class holds %d and %d would be booked", capacity, booked)).
		withField("attendees").withDetails(map[string]int{"capacity": capacity, "booked": booked})
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
	if t := c.Query("trainee"); t != "" {
		tid, err := parseOID(t, "trainee")
		if err != nil {
			return err
		}
		filter["attendees.trainee"] = tid
	}
	if pl := c.Query("plan"); pl != "" {
		pid, err := parseOID(pl, "plan")
		if err != nil {
			return err
		}
		filter["attendees.planId"] = pid
	}
	if s := c.Query("series"); s != "" {
		filter["seriesId"] = s
	}
	sort := bson.D{{Key: "start", Value: 1}}
	if c.Query("order") == "desc" {
		sort = bson.D{{Key: "start", Value: -1}}
	}
	if len(filter) == 0 && c.Query("limit") == "" {
		return badField("from", "give a date range, a trainee, a plan or a series (or page with limit)")
	}
	return pagedFind[models.Session](c, ctx, h.store.Coll(models.CollSessions), filter, sort)
}

func findSession(ctx context.Context, store *db.Store, id primitive.ObjectID) (models.Session, error) {
	var s models.Session
	if err := store.Coll(models.CollSessions).FindOne(ctx, bson.M{"_id": id}).Decode(&s); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return s, notFound("session")
		}
		return s, err
	}
	return s, nil
}

func (h *sessionHandler) get(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	s, err := findSession(ctx, h.store, id)
	if err != nil {
		return err
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
	capacity := 0
	if in.Capacity != nil {
		capacity = *in.Capacity
	}
	if capacity < 0 {
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
	attendees, err := h.buildAttendees(ctx, in.Attendees, nil)
	if err != nil {
		return err
	}
	if capacity > 0 && len(attendees) > capacity {
		return capacityErr(capacity, len(attendees))
	}
	// Logging a past class with its outcome is fine; a future one can't have one.
	if err := notStartedErr(*in.Start, status); err != nil {
		return err
	}
	for _, a := range attendees {
		if err := notStartedErr(*in.Start, a.Status); err != nil {
			return err
		}
	}
	if err := h.linkPlans(ctx, nil, attendees, nil, []occurrence{{*in.Start, typ}}, in.PlanOverride); err != nil {
		return err
	}
	now := time.Now()
	s := models.Session{
		ID:          primitive.NewObjectID(),
		Title:       strings.TrimSpace(in.Title),
		Type:        typ,
		Start:       *in.Start,
		DurationMin: in.DurationMin,
		Location:    strings.TrimSpace(derefStr(in.Location)),
		Capacity:    capacity,
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
	if _, err := h.store.Coll(models.CollSessions).InsertOne(ctx, s); err != nil {
		return err
	}
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
	cur, err := findSession(ctx, h.store, id)
	if err != nil {
		return err
	}
	set := bson.M{"updatedAt": time.Now()}
	next := cur // the session as it will be, for validation
	if in.Title != "" {
		next.Title = strings.TrimSpace(in.Title)
		set["title"] = next.Title
	}
	if in.Type != "" {
		if err := oneOf("type", in.Type, sessionTypes...); err != nil {
			return err
		}
		next.Type = in.Type
		set["type"] = in.Type
	}
	if in.Start != nil {
		if err := sanityTime("start", *in.Start); err != nil {
			return err
		}
		next.Start = *in.Start
		set["start"] = *in.Start
	}
	if in.DurationMin < 0 || in.DurationMin > 24*60 {
		return badField("durationMin", "duration must be between 1 and 1440 minutes")
	}
	if in.DurationMin > 0 {
		next.DurationMin = in.DurationMin
		set["durationMin"] = in.DurationMin
	}
	// Only touch location/fee/capacity when the caller actually sent them —
	// otherwise a partial update (e.g. just changing the time) would blank
	// the location and zero the fee.
	if in.Location != nil {
		set["location"] = strings.TrimSpace(*in.Location)
	}
	if in.Capacity != nil {
		if *in.Capacity < 0 {
			return badField("capacity", "capacity can't be negative (0 means unlimited)")
		}
		next.Capacity = *in.Capacity
		set["capacity"] = *in.Capacity
	}
	if in.Fee != nil {
		if *in.Fee < 0 {
			return badField("fee", "fee can't be negative")
		}
		set["fee"] = round2(*in.Fee)
	}
	if in.Status != "" {
		if err := oneOf("status", in.Status, sessionStatuses...); err != nil {
			return err
		}
		set["status"] = in.Status
	}
	if in.Attendees != nil {
		attendees, err := h.buildAttendees(ctx, in.Attendees, cur.Attendees)
		if err != nil {
			return err
		}
		if err := h.linkPlans(ctx, &id, attendees, cur.Attendees, []occurrence{{next.Start, next.Type}}, in.PlanOverride); err != nil {
			return err
		}
		next.Attendees = attendees
		set["attendees"] = attendees
	}
	if next.Capacity > 0 && len(next.Attendees) > next.Capacity {
		return capacityErr(next.Capacity, len(next.Attendees))
	}
	// Outcomes only once the class has begun: marking it done, moving a done
	// class into the future, or newly recording someone attended/no-show.
	status := defaultStr(in.Status, cur.Status)
	if (in.Status != "" && in.Status != cur.Status) || in.Start != nil {
		if err := notStartedErr(next.Start, status); err != nil {
			return err
		}
	}
	prevStatus := map[primitive.ObjectID]string{}
	for _, a := range cur.Attendees {
		prevStatus[a.Trainee] = a.Status
	}
	for _, a := range next.Attendees {
		if a.Status != prevStatus[a.Trainee] || in.Start != nil {
			if err := notStartedErr(next.Start, a.Status); err != nil {
				return err
			}
		}
	}
	// A moved booking must still fit the plans it is credited to.
	if in.Start != nil || in.Type != "" {
		for _, a := range next.Attendees {
			if a.PlanID == nil {
				continue
			}
			plan, err := findPlan(ctx, h.store, *a.PlanID)
			if err != nil {
				return err
			}
			if err := planFitErr(plan, a.Trainee, occurrence{next.Start, next.Type}); err != nil {
				return err
			}
		}
	}
	// Re-check overlaps only when a time-affecting field moved — a status-only
	// or attendee-only edit can't create a clash and shouldn't pay for the query.
	if in.Start != nil || in.Type != "" || in.DurationMin > 0 {
		if clash, err := h.findOverlap(ctx, next.Type, next.Start, next.DurationMin, &id); err != nil {
			return err
		} else if clash != nil {
			return apiErr(http.StatusConflict, CodeScheduleClash, overlapMsg(next.Type, clash))
		}
	}
	if _, err := h.store.Coll(models.CollSessions).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
		return err
	}
	s, err := findSession(ctx, h.store, id)
	if err != nil {
		return err
	}
	return c.JSON(s)
}

// remove hard-deletes a session only while nothing has been recorded on it.
// Once attendance is marked, the class is part of trainees' histories and
// plan counts — cancel it instead, which keeps the record.
func (h *sessionHandler) remove(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	res, err := h.store.Coll(models.CollSessions).DeleteOne(ctx, bson.M{
		"_id":              id,
		"attendees.status": bson.M{"$nin": bson.A{models.AttendAttended, models.AttendNoShow}},
		"status":           bson.M{"$ne": models.SessCompleted},
	})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		if _, err := findSession(ctx, h.store, id); err != nil {
			return err
		}
		return apiErr(http.StatusConflict, CodeConflict,
			"this class has attendance recorded — cancel it instead so trainees' history and plan counts stay intact")
	}
	return c.JSON(fiber.Map{"ok": true})
}

type attendanceInput struct {
	Trainee  string  `json:"trainee"`
	Status   string  `json:"status"`
	PlanID   *string `json:"planId"`   // absent: keep; "": clear; id: link
	Override bool    `json:"override"` // book past the plan's target
}

// attendance sets one attendee's status (booked/attended/no_show) and
// optionally their plan link, adding the attendee to the session if not
// already present — refused when the class is full. Repeating the same
// request changes nothing, so it can never double count.
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
	sess, err := findSession(ctx, h.store, id)
	if err != nil {
		return err
	}
	if err := notStartedErr(sess.Start, status); err != nil {
		return err
	}
	var prevAtt *models.Attendee
	for i := range sess.Attendees {
		if sess.Attendees[i].Trainee == tid {
			prevAtt = &sess.Attendees[i]
		}
	}
	// Resolve the plan link: unchanged, cleared, or (validated) set.
	var planID *primitive.ObjectID
	if prevAtt != nil {
		planID = prevAtt.PlanID
	}
	if in.PlanID != nil {
		planID = nil
		if *in.PlanID != "" {
			pid, err := parseOID(*in.PlanID, "planId")
			if err != nil {
				return err
			}
			planID = &pid
			cand := models.Attendee{Trainee: tid, PlanID: planID}
			var prevList []models.Attendee
			if prevAtt != nil {
				prevList = []models.Attendee{*prevAtt}
			}
			if err := h.linkPlans(ctx, &id, []models.Attendee{cand}, prevList, []occurrence{{sess.Start, sess.Type}}, in.Override); err != nil {
				return err
			}
		}
	}
	coll := h.store.Coll(models.CollSessions)
	now := time.Now()
	set := bson.M{"attendees.$.status": status, "updatedAt": now}
	unset := bson.M{}
	if planID != nil {
		set["attendees.$.planId"] = *planID
	} else {
		unset["attendees.$.planId"] = ""
	}
	upd := bson.M{"$set": set}
	if len(unset) > 0 {
		upd["$unset"] = unset
	}

	// 1) Already booked: update in place atomically (positional $).
	res, err := coll.UpdateOne(ctx, bson.M{"_id": id, "attendees.trainee": tid}, upd)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		// 2) Not present — push a new attendee, but only while still absent
		// and while the class has room (capacity 0 = unlimited). Both guards
		// live in the filter, so concurrent bookings can't overfill it.
		att := models.Attendee{Trainee: tid, TraineeName: trainee.Name, Status: status, PlanID: planID}
		push, err := coll.UpdateOne(ctx, bson.M{
			"_id":               id,
			"attendees.trainee": bson.M{"$ne": tid},
			"$or": bson.A{
				bson.M{"capacity": bson.M{"$in": bson.A{0, nil}}},
				bson.M{"$expr": bson.M{"$lt": bson.A{bson.M{"$size": bson.M{"$ifNull": bson.A{"$attendees", bson.A{}}}}, "$capacity"}}},
			},
		}, bson.M{"$push": bson.M{"attendees": att}, "$set": bson.M{"updatedAt": now}})
		if err != nil {
			return err
		}
		if push.MatchedCount == 0 {
			fresh, err := findSession(ctx, h.store, id)
			if err != nil {
				return err
			}
			present := false
			for _, a := range fresh.Attendees {
				if a.Trainee == tid {
					present = true
				}
			}
			if !present {
				return capacityErr(fresh.Capacity, len(fresh.Attendees)+1)
			}
			// A concurrent call added them first: apply this status on top.
			if _, err := coll.UpdateOne(ctx, bson.M{"_id": id, "attendees.trainee": tid}, upd); err != nil {
				return err
			}
		}
	}
	s, err := findSession(ctx, h.store, id)
	if err != nil {
		return err
	}
	return c.JSON(s)
}

// recordGrace lets a coach take attendance as people walk in, shortly
// before the start.
const recordGrace = 30 * time.Minute

// notStartedErr refuses recording an outcome — the class done, someone
// attended or a no-show — before the class has begun: it would credit plans
// and series for sessions that haven't been held.
func notStartedErr(start time.Time, outcome string) error {
	if outcome == models.AttendBooked || outcome == models.SessScheduled || outcome == models.SessCancelled {
		return nil
	}
	if start.After(clockNow().Add(recordGrace)) {
		return &APIError{Status: http.StatusUnprocessableEntity, Code: CodeNotStarted, Field: "status",
			Message: "this class hasn't started yet — record it once it begins"}
	}
	return nil
}

// attendanceBulk marks many attendees at once — e.g. after a class, "mark
// everyone still booked as attended" — leaving anyone already marked (a
// no-show stays a no-show) unless they are listed explicitly.
func (h *sessionHandler) attendanceBulk(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in struct {
		Status   string   `json:"status"`
		Only     string   `json:"only"`     // e.g. "booked": change only attendees in that state
		Trainees []string `json:"trainees"` // or exactly these
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	if err := oneOf("status", in.Status, attendStatuses...); err != nil {
		return err
	}
	if in.Only != "" {
		if err := oneOf("only", in.Only, attendStatuses...); err != nil {
			return err
		}
	}
	listed := map[primitive.ObjectID]bool{}
	for _, t := range in.Trainees {
		tid, err := parseOID(t, "trainees")
		if err != nil {
			return err
		}
		listed[tid] = true
	}
	if in.Only == "" && len(listed) == 0 {
		return badField("only", "say which attendees: only=booked, or a trainees list")
	}
	var out models.Session
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		s, err := findSession(tx, h.store, id)
		if err != nil {
			return err
		}
		if err := notStartedErr(s.Start, in.Status); err != nil {
			return err
		}
		changed := 0
		for i := range s.Attendees {
			a := &s.Attendees[i]
			if (len(listed) > 0 && !listed[a.Trainee]) || (in.Only != "" && a.Status != in.Only) {
				continue
			}
			if a.Status != in.Status {
				a.Status = in.Status
				changed++
			}
		}
		if changed > 0 {
			if _, err := h.store.Coll(models.CollSessions).UpdateOne(tx, bson.M{"_id": id},
				bson.M{"$set": bson.M{"attendees": s.Attendees, "updatedAt": time.Now()}}); err != nil {
				return err
			}
		}
		out = s
		return nil
	})
	if err != nil {
		return err
	}
	return c.JSON(out)
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
	var exclude []primitive.ObjectID
	if excludeID != nil {
		exclude = append(exclude, *excludeID)
	}
	return h.findOverlapExcluding(ctx, typ, start, dur, exclude)
}

func (h *sessionHandler) findOverlapExcluding(ctx context.Context, typ string, start time.Time, dur int, exclude []primitive.ObjectID) (*models.Session, error) {
	end := start.Add(time.Duration(dur) * time.Minute)
	filter := bson.M{
		"status": bson.M{"$ne": models.SessCancelled},
		"start":  bson.M{"$lt": end, "$gte": start.Add(-24 * time.Hour)},
	}
	if typ != models.SessionGroup {
		filter["type"] = models.SessionGroup // a private booking only clashes with group classes
	}
	if len(exclude) > 0 {
		filter["_id"] = bson.M{"$nin": exclude}
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
