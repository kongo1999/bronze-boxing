package server

import (
	"context"
	"errors"
	"fmt"
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

type coachHandler struct{ store *db.Store }

func linkedClassPayments(ctx context.Context, store *db.Store, session primitive.ObjectID, trainee *primitive.ObjectID) (int64, error) {
	filter := notVoided(bson.M{"sessionId": session})
	if trainee != nil {
		filter["trainee"] = *trainee
	}
	return store.Coll(models.CollPayments).CountDocuments(ctx, filter)
}

func registerCoachWorkflows(r fiber.Router, store *db.Store) {
	h := &coachHandler{store}
	r.Get("/payment-promises", h.promises)
	r.Post("/payment-promises", h.createPromise)
	r.Post("/payment-promises/:id/cancel", h.cancelPromise)
	r.Get("/trainees/:id/progress", h.progress)
	r.Post("/trainees/:id/progress", h.addProgress)
	r.Get("/trial-leads", h.leads)
	r.Post("/trial-leads", h.createLead)
	r.Patch("/trial-leads/:id", h.updateLead)
	r.Get("/follow-ups", h.followUps)
	r.Post("/follow-ups/action", h.followUpAction)
	r.Get("/month-review", h.monthReview)
	r.Post("/month-review", h.saveMonthReview)
	r.Get("/studio-policy", h.getPolicy)
	r.Put("/studio-policy", h.putPolicy)
	r.Get("/sessions/:id/waitlist", h.waitlist)
	r.Post("/sessions/:id/waitlist", h.joinWaitlist)
	r.Post("/sessions/:id/waitlist/:entry/accept", h.acceptWaitlist)
	r.Post("/sessions/:id/waitlist/:entry/decline", h.declineWaitlist)
	r.Post("/sessions/:id/cancel-booking", h.cancelBooking)
	r.Post("/sessions/:id/closeout", h.closeout)
}

func allDocs[T any](ctx context.Context, coll *mongo.Collection, filter bson.M, sortBy bson.D) ([]T, error) {
	opts := options.Find()
	if len(sortBy) > 0 {
		opts.SetSort(sortBy)
	}
	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var out []T
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []T{}
	}
	return out, nil
}

type promiseView struct {
	models.PaymentPromise
	PaidSince   float64 `json:"paidSince"`
	Outstanding float64 `json:"outstanding"`
	State       string  `json:"state"` // open | due | overdue | fulfilled | resolved | cancelled
}

func (h *coachHandler) promiseViews(ctx context.Context, ps []models.PaymentPromise) ([]promiseView, error) {
	out := make([]promiseView, 0, len(ps))
	today := models.DateKey(clockNow())
	for _, p := range ps {
		v := promiseView{PaymentPromise: p, State: "open", Outstanding: p.Amount}
		if p.CancelledAt != nil {
			v.State = "cancelled"
			v.Outstanding = 0
			out = append(out, v)
			continue
		}
		pays, err := allDocs[models.Payment](ctx, h.store.Coll(models.CollPayments), notVoided(bson.M{
			"trainee": p.Trainee, "type": models.PaySubscription, "periodMonth": p.PeriodMonth,
			"createdAt": bson.M{"$gte": p.CreatedAt},
		}), nil)
		if err != nil {
			return nil, err
		}
		for _, pay := range pays {
			v.PaidSince = round2(v.PaidSince + pay.Amount)
		}
		v.Outstanding = round2(max(0, p.Amount-v.PaidSince))
		var ch models.SubscriptionCharge
		err = h.store.Coll(models.CollCharges).FindOne(ctx, chargeKey(p.Trainee, p.PeriodMonth)).Decode(&ch)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		switch {
		case v.Outstanding == 0:
			v.State = "fulfilled"
		case err == nil && ch.DueCents <= ch.PaidCents:
			v.State = "resolved"
			v.Outstanding = 0
		case p.DueDay < today:
			v.State = "overdue"
		case p.DueDay == today:
			v.State = "due"
		}
		out = append(out, v)
	}
	return out, nil
}

func (h *coachHandler) promises(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	filter := bson.M{}
	if v := c.Query("trainee"); v != "" {
		id, err := parseOID(v, "trainee")
		if err != nil {
			return err
		}
		filter["trainee"] = id
	}
	if v := c.Query("m"); v != "" {
		if !models.ValidMonth(v) {
			return badField("m", "m must be YYYY-MM")
		}
		filter["periodMonth"] = v
	}
	ps, err := allDocs[models.PaymentPromise](ctx, h.store.Coll(models.CollPromises), filter, bson.D{{Key: "dueDay", Value: 1}, {Key: "createdAt", Value: 1}})
	if err != nil {
		return err
	}
	views, err := h.promiseViews(ctx, ps)
	if err != nil {
		return err
	}
	return c.JSON(views)
}

func (h *coachHandler) createPromise(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in struct {
		Trainee     string  `json:"trainee"`
		PeriodMonth string  `json:"periodMonth"`
		Amount      float64 `json:"amount"`
		DueDay      string  `json:"dueDay"`
		Note        string  `json:"note"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	t, err := loadTrainee(ctx, h.store, in.Trainee, "trainee")
	if err != nil {
		return err
	}
	if !models.ValidMonth(in.PeriodMonth) {
		return badField("periodMonth", "period must be YYYY-MM")
	}
	if _, err := models.ParseDay(in.DueDay); err != nil {
		return badField("dueDay", "pick a valid day")
	}
	if in.DueDay < models.DateKey(clockNow()) {
		return badField("dueDay", "promised day cannot be in the past")
	}
	amount, err := validMoney("amount", in.Amount, false)
	if err != nil {
		return err
	}
	rows, err := listDues(ctx, h.store, in.PeriodMonth)
	if err != nil {
		return err
	}
	remaining := 0.0
	for _, row := range rows {
		if row.Trainee.ID == t.ID && row.State != models.ChargeUnverified {
			remaining = row.Remaining
			break
		}
	}
	if models.Cents(amount) > models.Cents(remaining) {
		return badField("amount", fmt.Sprintf("only %.2f remains for that month", remaining))
	}
	prior, err := allDocs[models.PaymentPromise](ctx, h.store.Coll(models.CollPromises), bson.M{"trainee": t.ID, "periodMonth": in.PeriodMonth, "cancelledAt": nil}, nil)
	if err != nil {
		return err
	}
	views, err := h.promiseViews(ctx, prior)
	if err != nil {
		return err
	}
	for _, p := range views {
		if p.Outstanding > 0 {
			return apiErr(http.StatusConflict, CodeConflict, "there is already an open promise for this month; cancel it before replacing it")
		}
	}
	p := models.PaymentPromise{ID: primitive.NewObjectID(), Trainee: t.ID, TraineeName: t.Name, PeriodMonth: in.PeriodMonth, Amount: amount, DueDay: in.DueDay, Note: strings.TrimSpace(in.Note), CreatedAt: time.Now(), CreatedBy: actorOf(c)}
	if _, err := h.store.Coll(models.CollPromises).InsertOne(ctx, p); err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(p)
}

func (h *coachHandler) cancelPromise(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	now := time.Now()
	res, err := h.store.Coll(models.CollPromises).UpdateOne(ctx, bson.M{"_id": id, "cancelledAt": nil}, bson.M{"$set": bson.M{"cancelledAt": now}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return notFound("promise")
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *coachHandler) progress(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	if _, err := loadTrainee(ctx, h.store, id.Hex(), "id"); err != nil {
		return err
	}
	rows, err := allDocs[models.ProgressNote](ctx, h.store.Coll(models.CollProgress), bson.M{"trainee": id}, bson.D{{Key: "createdAt", Value: -1}})
	if err != nil {
		return err
	}
	return c.JSON(rows)
}

func (h *coachHandler) addProgress(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	if _, err := loadTrainee(ctx, h.store, id.Hex(), "id"); err != nil {
		return err
	}
	var in struct {
		Session   string `json:"session"`
		Goal      string `json:"goal"`
		Skills    string `json:"skills"`
		NextFocus string `json:"nextFocus"`
		Note      string `json:"note"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	p := models.ProgressNote{ID: primitive.NewObjectID(), Trainee: id, Goal: strings.TrimSpace(in.Goal), Skills: strings.TrimSpace(in.Skills), NextFocus: strings.TrimSpace(in.NextFocus), Note: strings.TrimSpace(in.Note), CreatedAt: time.Now(), CreatedBy: actorOf(c)}
	if p.Goal == "" && p.Skills == "" && p.NextFocus == "" && p.Note == "" {
		return badField("note", "add a goal, skill, focus or note")
	}
	if in.Session != "" {
		sid, err := parseOID(in.Session, "session")
		if err != nil {
			return err
		}
		s, err := findSession(ctx, h.store, sid)
		if err != nil {
			return err
		}
		found := false
		for _, a := range s.Attendees {
			if a.Trainee == id {
				found = true
			}
		}
		if !found {
			return badField("session", "trainee is not booked in that class")
		}
		p.Session = &sid
	}
	if _, err := h.store.Coll(models.CollProgress).InsertOne(ctx, p); err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(p)
}

func (h *coachHandler) leads(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	rows, err := allDocs[models.TrialLead](ctx, h.store.Coll(models.CollLeads), bson.M{}, bson.D{{Key: "updatedAt", Value: -1}})
	if err != nil {
		return err
	}
	return c.JSON(rows)
}

func validLeadDays(trial, follow string) error {
	if trial != "" {
		if _, err := models.ParseDay(trial); err != nil {
			return badField("trialDay", "pick a valid trial day")
		}
	}
	if follow != "" {
		if _, err := models.ParseDay(follow); err != nil {
			return badField("followUpDay", "pick a valid follow-up day")
		}
	}
	return nil
}

func (h *coachHandler) createLead(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in models.TrialLead
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return badField("name", "name is required")
	}
	if err := validLeadDays(in.TrialDay, in.FollowUpDay); err != nil {
		return err
	}
	if in.Status == "" {
		in.Status = "enquiry"
	}
	if err := oneOf("status", in.Status, "enquiry", "booked", "attended", "missed", "converted", "lost"); err != nil {
		return err
	}
	if in.Status == "converted" {
		return badField("status", "link a trainee before marking converted")
	}
	in.ID = primitive.NewObjectID()
	in.CreatedAt = time.Now()
	in.UpdatedAt = in.CreatedAt
	in.Trainee = nil
	in.TrialAttendedAt = nil
	if _, err := h.store.Coll(models.CollLeads).InsertOne(ctx, in); err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(in)
}

func (h *coachHandler) updateLead(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in struct {
		Name        *string `json:"name"`
		Phone       *string `json:"phone"`
		Source      *string `json:"source"`
		Status      *string `json:"status"`
		TrialDay    *string `json:"trialDay"`
		FollowUpDay *string `json:"followUpDay"`
		Notes       *string `json:"notes"`
		Trainee     *string `json:"trainee"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	var cur models.TrialLead
	if err := h.store.Coll(models.CollLeads).FindOne(ctx, bson.M{"_id": id}).Decode(&cur); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return notFound("lead")
		}
		return err
	}
	set := bson.M{"updatedAt": time.Now()}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return badField("name", "name is required")
		}
		set["name"] = strings.TrimSpace(*in.Name)
	}
	if in.Phone != nil {
		set["phone"] = strings.TrimSpace(*in.Phone)
	}
	if in.Source != nil {
		set["source"] = strings.TrimSpace(*in.Source)
	}
	if in.Notes != nil {
		set["notes"] = strings.TrimSpace(*in.Notes)
	}
	if in.TrialDay != nil {
		if err := validLeadDays(*in.TrialDay, ""); err != nil {
			return err
		}
		set["trialDay"] = *in.TrialDay
	}
	if in.FollowUpDay != nil {
		if err := validLeadDays("", *in.FollowUpDay); err != nil {
			return err
		}
		set["followUpDay"] = *in.FollowUpDay
	}
	if in.Status != nil {
		if err := oneOf("status", *in.Status, "enquiry", "booked", "attended", "missed", "converted", "lost"); err != nil {
			return err
		}
		set["status"] = *in.Status
		if *in.Status == "attended" && cur.TrialAttendedAt == nil {
			set["trialAttendedAt"] = time.Now()
		}
	}
	if in.Trainee != nil && *in.Trainee != "" {
		t, err := loadTrainee(ctx, h.store, *in.Trainee, "trainee")
		if err != nil {
			return err
		}
		set["trainee"] = t.ID
	}
	status := cur.Status
	if in.Status != nil {
		status = *in.Status
	}
	if status == "converted" && cur.Trainee == nil && set["trainee"] == nil {
		return badField("trainee", "link the new member before converting")
	}
	if _, err := h.store.Coll(models.CollLeads).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
		return err
	}
	if err := h.store.Coll(models.CollLeads).FindOne(ctx, bson.M{"_id": id}).Decode(&cur); err != nil {
		return err
	}
	return c.JSON(cur)
}

func (h *coachHandler) policy(ctx context.Context) (models.StudioPolicy, error) {
	p := models.StudioPolicy{ID: "default", CancelBeforeHours: 12, LateCancellationAction: "allow"}
	err := h.store.Coll(models.CollPolicy).FindOne(ctx, bson.M{"_id": "default"}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return p, nil
	}
	return p, err
}
func (h *coachHandler) getPolicy(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	p, err := h.policy(ctx)
	if err != nil {
		return err
	}
	return c.JSON(p)
}
func (h *coachHandler) putPolicy(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var p models.StudioPolicy
	if err := c.BodyParser(&p); err != nil {
		return badField("body", "invalid body")
	}
	if p.CancelBeforeHours < 0 || p.CancelBeforeHours > 168 {
		return badField("cancelBeforeHours", "choose 0 to 168 hours")
	}
	if err := oneOf("lateCancellationAction", p.LateCancellationAction, "allow", "no_show"); err != nil {
		return err
	}
	p.ID = "default"
	_, err := h.store.Coll(models.CollPolicy).ReplaceOne(ctx, bson.M{"_id": "default"}, p, options.Replace().SetUpsert(true))
	if err != nil {
		return err
	}
	return c.JSON(p)
}

func (h *coachHandler) waitlist(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	if _, err := findSession(ctx, h.store, id); err != nil {
		return err
	}
	rows, err := allDocs[models.WaitlistEntry](ctx, h.store.Coll(models.CollWaitlist), bson.M{"session": id}, bson.D{{Key: "createdAt", Value: 1}})
	if err != nil {
		return err
	}
	return c.JSON(rows)
}
func (h *coachHandler) joinWaitlist(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in struct {
		Trainee string `json:"trainee"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	t, err := loadTrainee(ctx, h.store, in.Trainee, "trainee")
	if err != nil {
		return err
	}
	if t.Status != models.StatusActive || t.ArchivedAt != nil {
		return badField("trainee", "choose an active trainee")
	}
	s, err := findSession(ctx, h.store, id)
	if err != nil {
		return err
	}
	if s.Status != models.SessScheduled || s.Start.Before(clockNow()) {
		return badField("session", "waitlist is only for an upcoming class")
	}
	if s.Capacity == 0 || len(s.Attendees) < s.Capacity {
		return badField("session", "this class has room; book the trainee directly")
	}
	for _, a := range s.Attendees {
		if a.Trainee == t.ID {
			return apiErr(http.StatusConflict, CodeDuplicate, "trainee is already booked")
		}
	}
	now := time.Now()
	e := models.WaitlistEntry{ID: primitive.NewObjectID(), Session: id, Trainee: t.ID, TraineeName: t.Name, Status: "waiting", CreatedAt: now, UpdatedAt: now}
	// One row per trainee/class. A declined or cancelled request may rejoin at
	// the end of the line by replacing its timestamps.
	res, err := h.store.Coll(models.CollWaitlist).UpdateOne(ctx, bson.M{"session": id, "trainee": t.ID, "status": bson.M{"$in": bson.A{"declined", "cancelled"}}}, bson.M{"$set": bson.M{"status": "waiting", "createdAt": now, "updatedAt": now}, "$unset": bson.M{"offeredAt": ""}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		if _, err := h.store.Coll(models.CollWaitlist).InsertOne(ctx, e); err != nil {
			if isDup(err) {
				return apiErr(http.StatusConflict, CodeDuplicate, "trainee is already on this waitlist")
			}
			return err
		}
	} else {
		if err := h.store.Coll(models.CollWaitlist).FindOne(ctx, bson.M{"session": id, "trainee": t.ID}).Decode(&e); err != nil {
			return err
		}
	}
	return c.Status(http.StatusCreated).JSON(e)
}
func (h *coachHandler) offerNext(ctx context.Context, session primitive.ObjectID) error {
	// Offer at most one person for the newly freed place. The transaction
	// serializes this with the cancellation that opened it.
	var e models.WaitlistEntry
	err := h.store.Coll(models.CollWaitlist).FindOne(ctx, bson.M{"session": session, "status": "waiting"}, options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: 1}})).Decode(&e)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = h.store.Coll(models.CollWaitlist).UpdateOne(ctx, bson.M{"_id": e.ID, "status": "waiting"}, bson.M{"$set": bson.M{"status": "offered", "offeredAt": now, "updatedAt": now}})
	return err
}
func (h *coachHandler) cancelBooking(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in struct {
		Trainee string `json:"trainee"`
		Reason  string `json:"reason"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	tid, err := parseOID(in.Trainee, "trainee")
	if err != nil {
		return err
	}
	p, err := h.policy(ctx)
	if err != nil {
		return err
	}
	var out models.Session
	late := false
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		s, err := findSession(tx, h.store, id)
		if err != nil {
			return err
		}
		if s.Status != models.SessScheduled {
			return apiErr(http.StatusConflict, CodeConflict, "only a scheduled class can release a booking")
		}
		pos := -1
		for i, a := range s.Attendees {
			if a.Trainee == tid {
				pos = i
				break
			}
		}
		if pos < 0 {
			return notFound("booking")
		}
		if s.Attendees[pos].Status != models.AttendBooked {
			return apiErr(http.StatusConflict, CodeConflict, "attendance is already recorded; correct it on the class")
		}
		late = clockNow().Add(time.Duration(p.CancelBeforeHours) * time.Hour).After(s.Start)
		outcome := "released"
		if late && p.LateCancellationAction == "no_show" && !clockNow().Before(s.Start) {
			s.Attendees[pos].Status = models.AttendNoShow
			outcome = "no_show"
		} else {
			n, err := linkedClassPayments(tx, h.store, id, &tid)
			if err != nil {
				return err
			}
			if n > 0 {
				return apiErr(http.StatusConflict, CodeConflict, "this booking has a linked payment; void or correct that payment before cancelling")
			}
			s.Attendees = append(s.Attendees[:pos], s.Attendees[pos+1:]...)
		}
		s.UpdatedAt = time.Now()
		if _, err := h.store.Coll(models.CollSessions).UpdateOne(tx, bson.M{"_id": id}, bson.M{"$set": bson.M{"attendees": s.Attendees, "updatedAt": s.UpdatedAt}}); err != nil {
			return err
		}
		log := models.SessionCancellation{ID: primitive.NewObjectID(), Session: id, Trainee: tid, Late: late, Outcome: outcome, Reason: strings.TrimSpace(in.Reason), At: time.Now()}
		if _, err := h.store.Coll(models.CollCancellations).InsertOne(tx, log); err != nil {
			return err
		}
		if outcome == "released" && s.Start.After(clockNow()) {
			if err := h.offerNext(tx, id); err != nil {
				return err
			}
		}
		out = s
		return nil
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"session": out, "late": late})
}
func (h *coachHandler) acceptWaitlist(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	eid, err := parseOID(c.Params("entry"), "entry")
	if err != nil {
		return err
	}
	var out models.Session
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		var e models.WaitlistEntry
		if err := h.store.Coll(models.CollWaitlist).FindOne(tx, bson.M{"_id": eid, "session": id, "status": "offered"}).Decode(&e); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return notFound("offer")
			}
			return err
		}
		s, err := findSession(tx, h.store, id)
		if err != nil {
			return err
		}
		if s.Status != models.SessScheduled || s.Start.Before(clockNow()) {
			return apiErr(http.StatusConflict, CodeConflict, "this class is no longer open for bookings")
		}
		if s.Capacity > 0 && len(s.Attendees) >= s.Capacity {
			return capacityErr(s.Capacity, len(s.Attendees)+1)
		}
		for _, a := range s.Attendees {
			if a.Trainee == e.Trainee {
				return apiErr(http.StatusConflict, CodeDuplicate, "trainee is already booked")
			}
		}
		s.Attendees = append(s.Attendees, models.Attendee{Trainee: e.Trainee, TraineeName: e.TraineeName, Status: models.AttendBooked})
		s.UpdatedAt = time.Now()
		if _, err := h.store.Coll(models.CollSessions).UpdateOne(tx, bson.M{"_id": id}, bson.M{"$set": bson.M{"attendees": s.Attendees, "updatedAt": s.UpdatedAt}}); err != nil {
			return err
		}
		if _, err := h.store.Coll(models.CollWaitlist).UpdateOne(tx, bson.M{"_id": eid}, bson.M{"$set": bson.M{"status": "booked", "updatedAt": time.Now()}}); err != nil {
			return err
		}
		out = s
		return nil
	})
	if err != nil {
		return err
	}
	return c.JSON(out)
}
func (h *coachHandler) declineWaitlist(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	eid, err := parseOID(c.Params("entry"), "entry")
	if err != nil {
		return err
	}
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		var previous models.WaitlistEntry
		if err := h.store.Coll(models.CollWaitlist).FindOne(tx, bson.M{"_id": eid, "session": id, "status": bson.M{"$in": bson.A{"waiting", "offered"}}}).Decode(&previous); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return notFound("waitlist entry")
			}
			return err
		}
		res, err := h.store.Coll(models.CollWaitlist).UpdateOne(tx, bson.M{"_id": eid, "session": id, "status": bson.M{"$in": bson.A{"waiting", "offered"}}}, bson.M{"$set": bson.M{"status": "declined", "updatedAt": time.Now()}})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return notFound("waitlist entry")
		}
		if previous.Status == "offered" {
			return h.offerNext(tx, id)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true})
}
func (h *coachHandler) closeout(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in struct {
		Attendance []struct {
			Trainee string `json:"trainee"`
			Status  string `json:"status"`
		} `json:"attendance"`
		Note string `json:"note"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	statuses := map[primitive.ObjectID]string{}
	for _, a := range in.Attendance {
		tid, err := parseOID(a.Trainee, "trainee")
		if err != nil {
			return err
		}
		if err := oneOf("status", a.Status, models.AttendAttended, models.AttendNoShow); err != nil {
			return err
		}
		if _, ok := statuses[tid]; ok {
			return badField("attendance", "a trainee is listed twice")
		}
		statuses[tid] = a.Status
	}
	var out models.Session
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		s, err := findSession(tx, h.store, id)
		if err != nil {
			return err
		}
		if s.Status == models.SessCancelled {
			return apiErr(http.StatusConflict, CodeConflict, "a cancelled class cannot be closed")
		}
		if err := notStartedErr(s.Start, models.SessCompleted); err != nil {
			return err
		}
		if len(statuses) != len(s.Attendees) {
			return badField("attendance", "mark every booked trainee attended or no-show")
		}
		for i := range s.Attendees {
			st, ok := statuses[s.Attendees[i].Trainee]
			if !ok {
				return badField("attendance", "attendance does not match this class")
			}
			s.Attendees[i].Status = st
		}
		now := time.Now()
		s.Status = models.SessCompleted
		s.CloseoutNote = strings.TrimSpace(in.Note)
		s.ClosedAt = &now
		s.UpdatedAt = now
		if _, err := h.store.Coll(models.CollSessions).UpdateOne(tx, bson.M{"_id": id}, bson.M{"$set": bson.M{"status": s.Status, "attendees": s.Attendees, "closeoutNote": s.CloseoutNote, "closedAt": now, "updatedAt": now}}); err != nil {
			return err
		}
		out = s
		return nil
	})
	if err != nil {
		return err
	}
	return c.JSON(out)
}

type followUp struct {
	Key          string  `json:"key"`
	Kind         string  `json:"kind"`
	Title        string  `json:"title"`
	Detail       string  `json:"detail"`
	Phone        string  `json:"phone,omitempty"`
	Trainee      string  `json:"trainee,omitempty"`
	Href         string  `json:"href"`
	DueDay       string  `json:"dueDay,omitempty"`
	Amount       float64 `json:"amount,omitempty"`
	SnoozedUntil string  `json:"snoozedUntil,omitempty"`
}

func (h *coachHandler) followUpAction(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in struct {
		Key    string `json:"key"`
		Action string `json:"action"`
		Days   int    `json:"days"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	validKey := false
	for _, prefix := range []string{"dues:", "promise:", "trial:", "plan:", "inactive:"} {
		if strings.HasPrefix(in.Key, prefix) {
			validKey = true
			break
		}
	}
	if len(in.Key) > 128 || !validKey {
		return badField("key", "invalid follow-up key")
	}
	if err := oneOf("action", in.Action, "contacted", "snooze", "clear"); err != nil {
		return err
	}
	set := bson.M{"updatedBy": actorOf(c)}
	if in.Action == "clear" {
		set["snoozedUntil"] = ""
	} else {
		days := in.Days
		if days == 0 {
			days = 7
		}
		if days < 1 || days > 30 {
			return badField("days", "choose 1 to 30 days")
		}
		set["snoozedUntil"] = models.DateKey(clockNow().AddDate(0, 0, days))
		if in.Action == "contacted" {
			set["lastContactedAt"] = time.Now()
		}
	}
	_, err := h.store.Coll(models.CollFollowUps).UpdateOne(ctx, bson.M{"_id": in.Key}, bson.M{"$set": set}, options.Update().SetUpsert(true))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *coachHandler) followUps(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	today := models.DateKey(clockNow())
	month := models.MonthKey(clockNow())
	people, err := allDocs[models.Trainee](ctx, h.store.Coll(models.CollTrainees), bson.M{"archivedAt": nil}, nil)
	if err != nil {
		return err
	}
	byID := map[primitive.ObjectID]models.Trainee{}
	for _, t := range people {
		byID[t.ID] = t
	}
	out := []followUp{}
	for _, m := range []string{month, models.MonthKey(clockNow().AddDate(0, -1, 0))} {
		rows, err := listDues(ctx, h.store, m)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.State != models.ChargePartial && r.State != models.ChargeUnpaid {
				continue
			}
			t := byID[r.Trainee.ID]
			if t.ID.IsZero() {
				continue
			}
			out = append(out, followUp{Key: "dues:" + t.ID.Hex() + ":" + m, Kind: "dues", Title: t.Name, Detail: fmt.Sprintf("%s dues: %.2f still owed", m, r.Remaining), Phone: t.Phone, Trainee: t.ID.Hex(), Href: "/payments?m=" + m + "&focus=" + t.ID.Hex(), Amount: r.Remaining})
		}
	}
	ps, err := allDocs[models.PaymentPromise](ctx, h.store.Coll(models.CollPromises), bson.M{"cancelledAt": nil}, nil)
	if err != nil {
		return err
	}
	pvs, err := h.promiseViews(ctx, ps)
	if err != nil {
		return err
	}
	for _, p := range pvs {
		if p.State != "due" && p.State != "overdue" {
			continue
		}
		t := byID[p.Trainee]
		out = append(out, followUp{Key: "promise:" + p.ID.Hex(), Kind: "promise", Title: p.TraineeName, Detail: fmt.Sprintf("Promised %.2f for %s", p.Outstanding, p.PeriodMonth), Phone: t.Phone, Trainee: p.Trainee.Hex(), Href: "/payments?m=" + p.PeriodMonth + "&focus=" + p.Trainee.Hex(), DueDay: p.DueDay, Amount: p.Outstanding})
	}
	leadRows, err := allDocs[models.TrialLead](ctx, h.store.Coll(models.CollLeads), bson.M{"status": bson.M{"$nin": bson.A{"converted", "lost"}}, "followUpDay": bson.M{"$ne": "", "$lte": today}}, nil)
	if err != nil {
		return err
	}
	for _, l := range leadRows {
		out = append(out, followUp{Key: "trial:" + l.ID.Hex(), Kind: "trial", Title: l.Name, Detail: "Follow up after trial · " + l.Status, Phone: l.Phone, Href: "/trial-leads?focus=" + l.ID.Hex(), DueDay: l.FollowUpDay})
	}
	planRows, err := allDocs[models.SessionPlan](ctx, h.store.Coll(models.CollPlans), bson.M{"status": "active"}, nil)
	if err != nil {
		return err
	}
	progress, err := planProgress(ctx, h.store, planRows, nil)
	if err != nil {
		return err
	}
	for _, p := range planRows {
		pr := progress[p.ID]
		if pr == nil || pr.Remaining > 2 && !(p.EndDate != "" && p.EndDate <= models.DateKey(clockNow().AddDate(0, 0, 7))) {
			continue
		}
		t := byID[p.Trainee]
		out = append(out, followUp{Key: "plan:" + p.ID.Hex(), Kind: "plan", Title: p.TraineeName, Detail: fmt.Sprintf("%s: %d of %d sessions left", p.Title, pr.Remaining, pr.Target), Phone: t.Phone, Trainee: p.Trainee.Hex(), Href: "/trainees/" + p.Trainee.Hex(), DueDay: p.EndDate})
	}
	// Last attendance is based on attended records, never just a booking.
	cutoff := clockNow().AddDate(0, 0, -21)
	sessions, err := allDocs[models.Session](ctx, h.store.Coll(models.CollSessions), bson.M{"start": bson.M{"$gte": cutoff}, "status": "completed", "attendees.status": "attended"}, nil)
	if err != nil {
		return err
	}
	activeRecently := map[primitive.ObjectID]bool{}
	for _, s := range sessions {
		for _, a := range s.Attendees {
			if a.Status == models.AttendAttended {
				activeRecently[a.Trainee] = true
			}
		}
	}
	for _, t := range people {
		if t.Status != models.StatusActive || activeRecently[t.ID] || t.CreatedAt.After(cutoff) {
			continue
		}
		out = append(out, followUp{Key: "inactive:" + t.ID.Hex(), Kind: "inactive", Title: t.Name, Detail: "No attended class in 21 days", Phone: t.Phone, Trainee: t.ID.Hex(), Href: "/trainees/" + t.ID.Hex()})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DueDay == out[j].DueDay {
			return out[i].Title < out[j].Title
		}
		if out[i].DueDay == "" {
			return false
		}
		if out[j].DueDay == "" {
			return true
		}
		return out[i].DueDay < out[j].DueDay
	})
	actions, err := allDocs[models.FollowUpAction](ctx, h.store.Coll(models.CollFollowUps), bson.M{}, nil)
	if err != nil {
		return err
	}
	byKey := map[string]string{}
	for _, a := range actions {
		byKey[a.ID] = a.SnoozedUntil
	}
	shown := make([]followUp, 0, len(out))
	for _, item := range out {
		item.SnoozedUntil = byKey[item.Key]
		if c.Query("show") == "snoozed" {
			if item.SnoozedUntil > today {
				shown = append(shown, item)
			}
		} else if item.SnoozedUntil <= today {
			shown = append(shown, item)
		}
	}
	return c.JSON(shown)
}

type reviewIssues struct {
	UnclosedClasses    int `json:"unclosedClasses"`
	UnmarkedAttendance int `json:"unmarkedAttendance"`
	UnresolvedDues     int `json:"unresolvedDues"`
	CashDifferences    int `json:"cashDifferences"`
	LowStock           int `json:"lowStock"`
}

func (h *coachHandler) reviewState(c *fiber.Ctx) (fiber.Map, error) {
	month := c.Query("m", models.MonthKey(clockNow()))
	if !models.ValidMonth(month) || month > models.MonthKey(clockNow()) {
		return nil, badField("m", "choose a current or past month")
	}
	rep, err := reportFromQuery(c, h.store)
	if err != nil {
		return nil, err
	}
	ctx, cancel := reqCtx()
	defer cancel()
	var reviewed *models.MonthReview
	var r models.MonthReview
	err = h.store.Coll(models.CollReviews).FindOne(ctx, bson.M{"month": month}).Decode(&r)
	if err == nil {
		reviewed = &r
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	issues := reviewIssues{UnclosedClasses: rep.Sessions.NotMarkedDone, UnmarkedAttendance: rep.Attendance.Unmarked, UnresolvedDues: rep.Subscriptions.Partial + rep.Subscriptions.Unpaid, LowStock: rep.Inventory.LowNow + rep.Inventory.OutNow}
	from, to, _ := models.MonthRange(month)
	ledger, err := loadLedger(ctx, h.store, from, to)
	if err != nil {
		return nil, err
	}
	expected := map[string]int64{}
	for _, row := range ledger {
		if row.Voided || row.Kind == "expense" {
			continue
		}
		method := methodKey(row)
		if method == models.MethodCash || method == "unspecified" {
			expected[row.Day] += row.inCents
		}
	}
	closings, err := allDocs[models.CashClosing](ctx, h.store.Coll(models.CollClosings), bson.M{"day": bson.M{"$gte": models.DateKey(from), "$lt": models.DateKey(to)}}, nil)
	if err != nil {
		return nil, err
	}
	for _, cl := range closings {
		if cl.CountedCents != expected[cl.Day] {
			issues.CashDifferences++
		}
	}
	return fiber.Map{"month": month, "issues": issues, "review": reviewed}, nil
}
func (h *coachHandler) monthReview(c *fiber.Ctx) error {
	state, err := h.reviewState(c)
	if err != nil {
		return err
	}
	return c.JSON(state)
}
func (h *coachHandler) saveMonthReview(c *fiber.Ctx) error {
	var in struct {
		Month string `json:"month"`
		Note  string `json:"note"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	if !models.ValidMonth(in.Month) || in.Month > models.MonthKey(clockNow()) {
		return badField("month", "choose a current or past month")
	}
	ctx, cancel := reqCtx()
	defer cancel()
	// A review is an acknowledgement of the current figures, including any
	// open issues. Its timestamp and note remain separate from the report.
	r := models.MonthReview{ID: primitive.NewObjectID(), Month: in.Month, ReviewedAt: time.Now(), ReviewedBy: actorOf(c), Note: strings.TrimSpace(in.Note)}
	_, err := h.store.Coll(models.CollReviews).UpdateOne(ctx, bson.M{"month": in.Month}, bson.M{"$set": bson.M{"reviewedAt": r.ReviewedAt, "reviewedBy": r.ReviewedBy, "note": r.Note}, "$setOnInsert": bson.M{"_id": r.ID, "month": r.Month}}, options.Update().SetUpsert(true))
	if err != nil {
		return err
	}
	return c.JSON(r)
}
