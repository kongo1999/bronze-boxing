package server

import (
	"context"
	"errors"
	"net/http"
	"sort"
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

// Session plans: a trainee's allowance ("12 private sessions"). A booking is
// credited to a plan through the attendee's planId; a credit counts only
// when the session is completed AND that trainee is marked attended — a
// completed class where they're still "booked" is flagged, not credited; a
// no-show or a cancelled class never reduces what remains. Progress is always
// recomputed from the session records, so nothing can count twice.

type planHandler struct{ store *db.Store }

func registerPlans(r fiber.Router, store *db.Store) {
	h := &planHandler{store}
	r.Get("/trainees/:id/session-plans", h.forTrainee)
	r.Post("/trainees/:id/session-plans", h.create)
	r.Get("/session-plans", h.list)
	r.Get("/session-plans/:id", h.get)
	r.Patch("/session-plans/:id", h.update)
}

var planStatuses = []string{"active", "completed", "cancelled"}

func findPlan(ctx context.Context, store *db.Store, id primitive.ObjectID) (models.SessionPlan, error) {
	var p models.SessionPlan
	if err := store.Coll(models.CollPlans).FindOne(ctx, bson.M{"_id": id}).Decode(&p); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return p, apiErr(http.StatusBadRequest, CodePlanMismatch, "that session plan doesn't exist").withField("planId")
		}
		return p, err
	}
	return p, nil
}

// PlanProgress is a plan's counters, computed from sessions.
type PlanProgress struct {
	Target           int      `json:"target"`
	Completed        int      `json:"completed"` // credited: session completed + attended
	Remaining        int      `json:"remaining"` // target − completed (never negative)
	UpcomingBooked   int      `json:"upcomingBooked"`
	AttendanceNeeded int      `json:"attendanceNeeded"` // completed class, trainee still "booked"
	NoShows          int      `json:"noShows"`
	Cancelled        int      `json:"cancelled"`
	UnassignedSlots  int      `json:"unassignedSlots"` // target − completed − upcoming − needs marking
	Credited         []string `json:"credited"`        // session ids that count
	Upcoming         []string `json:"upcoming"`
}

// planProgress computes progress for the given plans. excludeSession leaves
// one session out (the one being re-booked, so it isn't counted against
// itself).
func planProgress(ctx context.Context, store *db.Store, plans []models.SessionPlan, excludeSession *primitive.ObjectID) (map[primitive.ObjectID]*PlanProgress, error) {
	out := map[primitive.ObjectID]*PlanProgress{}
	owner := map[primitive.ObjectID]primitive.ObjectID{}
	ids := make([]primitive.ObjectID, 0, len(plans))
	for _, p := range plans {
		out[p.ID] = &PlanProgress{Target: p.TargetCount, Credited: []string{}, Upcoming: []string{}}
		owner[p.ID] = p.Trainee
		ids = append(ids, p.ID)
	}
	if len(ids) == 0 {
		return out, nil
	}
	filter := bson.M{"attendees.planId": bson.M{"$in": ids}}
	if excludeSession != nil {
		filter["_id"] = bson.M{"$ne": *excludeSession}
	}
	cur, err := store.Coll(models.CollSessions).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "start", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var sessions []models.Session
	if err := cur.All(ctx, &sessions); err != nil {
		return nil, err
	}
	for _, s := range sessions {
		for _, a := range s.Attendees {
			if a.PlanID == nil {
				continue
			}
			pr, ok := out[*a.PlanID]
			if !ok || owner[*a.PlanID] != a.Trainee {
				continue // not one of ours, or a link to someone else's plan
			}
			switch {
			case s.Status == models.SessCancelled:
				pr.Cancelled++
			case a.Status == models.AttendNoShow:
				pr.NoShows++
			case s.Status == models.SessCompleted && a.Status == models.AttendAttended:
				pr.Completed++
				pr.Credited = append(pr.Credited, s.ID.Hex())
			case s.Status == models.SessCompleted:
				pr.AttendanceNeeded++
			default: // scheduled (and attended ahead of completion counts once completed)
				pr.UpcomingBooked++
				pr.Upcoming = append(pr.Upcoming, s.ID.Hex())
			}
		}
	}
	for _, pr := range out {
		pr.Remaining = max(pr.Target-pr.Completed, 0)
		pr.UnassignedSlots = max(pr.Target-pr.Completed-pr.UpcomingBooked-pr.AttendanceNeeded, 0)
	}
	return out, nil
}

type planView struct {
	models.SessionPlan
	Progress *PlanProgress `json:"progress"`
}

func (h *planHandler) views(ctx context.Context, plans []models.SessionPlan) ([]planView, error) {
	prog, err := planProgress(ctx, h.store, plans, nil)
	if err != nil {
		return nil, err
	}
	out := make([]planView, len(plans))
	for i, p := range plans {
		out[i] = planView{SessionPlan: p, Progress: prog[p.ID]}
	}
	return out, nil
}

func (h *planHandler) forTrainee(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	cur, err := h.store.Coll(models.CollPlans).Find(ctx, bson.M{"trainee": id}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return err
	}
	var plans []models.SessionPlan
	if err := cur.All(ctx, &plans); err != nil {
		return err
	}
	// Active plans first, then the most recent.
	sort.SliceStable(plans, func(i, j int) bool { return plans[i].Status == "active" && plans[j].Status != "active" })
	views, err := h.views(ctx, plans)
	if err != nil {
		return err
	}
	return c.JSON(views)
}

// list returns plans for several trainees at once (?trainees=a,b&status=active)
// — what the session form needs to offer a plan per attendee.
func (h *planHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	filter := bson.M{}
	if t := c.Query("trainees"); t != "" {
		var ids []primitive.ObjectID
		for _, s := range strings.Split(t, ",") {
			id, err := parseOID(strings.TrimSpace(s), "trainees")
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		filter["trainee"] = bson.M{"$in": ids}
	}
	if s := c.Query("status"); s != "" {
		filter["status"] = s
	}
	cur, err := h.store.Coll(models.CollPlans).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(500))
	if err != nil {
		return err
	}
	var plans []models.SessionPlan
	if err := cur.All(ctx, &plans); err != nil {
		return err
	}
	views, err := h.views(ctx, plans)
	if err != nil {
		return err
	}
	return c.JSON(views)
}

func (h *planHandler) get(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	p, err := findPlan(ctx, h.store, id)
	if err != nil {
		return notFound("session plan")
	}
	views, err := h.views(ctx, []models.SessionPlan{p})
	if err != nil {
		return err
	}
	return c.JSON(views[0])
}

type planInput struct {
	Title       *string `json:"title"`
	TargetCount *int    `json:"targetCount"`
	StartDate   *string `json:"startDate"`
	EndDate     *string `json:"endDate"`
	SessionType *string `json:"sessionType"`
	Status      *string `json:"status"`
	Notes       *string `json:"notes"`
	Reason      string  `json:"reason"`
}

func validatePlan(p *models.SessionPlan, in planInput) error {
	if in.Title != nil {
		p.Title = strings.TrimSpace(*in.Title)
	}
	if p.Title == "" {
		return badField("title", "give the plan a name, e.g. “12 private sessions”")
	}
	if in.TargetCount != nil {
		p.TargetCount = *in.TargetCount
	}
	if p.TargetCount <= 0 || p.TargetCount > 500 {
		return badField("targetCount", "the number of sessions must be between 1 and 500")
	}
	if in.StartDate != nil {
		p.StartDate = *in.StartDate
	}
	if _, err := models.ParseDay(p.StartDate); err != nil {
		return badField("startDate", "start date must be YYYY-MM-DD")
	}
	if in.EndDate != nil {
		p.EndDate = *in.EndDate
	}
	if p.EndDate != "" {
		if _, err := models.ParseDay(p.EndDate); err != nil {
			return badField("endDate", "end date must be YYYY-MM-DD")
		}
		if p.EndDate < p.StartDate {
			return badField("endDate", "the plan ends before it starts")
		}
	}
	if in.SessionType != nil {
		p.SessionType = *in.SessionType
	}
	if p.SessionType != "" {
		if err := oneOf("sessionType", p.SessionType, sessionTypes...); err != nil {
			return err
		}
	}
	if in.Status != nil {
		p.Status = *in.Status
	}
	if err := oneOf("status", p.Status, planStatuses...); err != nil {
		return err
	}
	if in.Notes != nil {
		p.Notes = strings.TrimSpace(*in.Notes)
	}
	return nil
}

func (h *planHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	t, err := loadTrainee(ctx, h.store, id.Hex(), "id")
	if err != nil {
		return notFound("trainee")
	}
	var in planInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	now := time.Now()
	p := models.SessionPlan{
		ID: primitive.NewObjectID(), Trainee: t.ID, TraineeName: t.Name,
		StartDate: models.DateKey(now), Status: "active", CreatedAt: now, UpdatedAt: now,
	}
	if err := validatePlan(&p, in); err != nil {
		return err
	}
	actor := actorOf(c)
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		if _, err := h.store.Coll(models.CollPlans).InsertOne(tx, p); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "plan", Ref: p.ID, Action: "create", After: p, Actor: actor})
	})
	if err != nil {
		return err
	}
	views, err := h.views(ctx, []models.SessionPlan{p})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(views[0])
}

// update edits a plan. Changing its target or dates, or closing it, is a
// material change and needs a reason; it is audited.
func (h *planHandler) update(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in planInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	prev, err := findPlan(ctx, h.store, id)
	if err != nil {
		return notFound("session plan")
	}
	next := prev
	if err := validatePlan(&next, in); err != nil {
		return err
	}
	reason := strings.TrimSpace(in.Reason)
	material := next.TargetCount != prev.TargetCount || next.StartDate != prev.StartDate ||
		next.EndDate != prev.EndDate || next.Status != prev.Status || next.SessionType != prev.SessionType
	if material {
		if reason, err = requireReason(in.Reason, "change a session plan's target, dates or status"); err != nil {
			return err
		}
	}
	next.UpdatedAt = time.Now()
	actor := actorOf(c)
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		if _, err := h.store.Coll(models.CollPlans).ReplaceOne(tx, bson.M{"_id": id}, next); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "plan", Ref: id, Action: "update", Before: prev, After: next, Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	views, err := h.views(ctx, []models.SessionPlan{next})
	if err != nil {
		return err
	}
	return c.JSON(views[0])
}
