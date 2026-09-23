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
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

// ── Recurring series: preview, create, progress, edit ─────────────────────

type recurringInput struct {
	Title        string          `json:"title"`
	Type         string          `json:"type"`
	Weekdays     []int           `json:"weekdays"` // 0=Sunday .. 6=Saturday
	Time         string          `json:"time"`     // "HH:MM" studio wall clock
	DurationMin  int             `json:"durationMin"`
	Location     string          `json:"location"`
	Capacity     int             `json:"capacity"`
	Fee          float64         `json:"fee"`
	FromDay      string          `json:"fromDay"` // studio-local YYYY-MM-DD (preferred)
	ToDay        string          `json:"toDay"`   // inclusive
	From         *time.Time      `json:"from"`    // legacy: UTC-midnight instants of the days
	To           *time.Time      `json:"to"`
	Attendees    []attendeeInput `json:"attendees"`
	PlanOverride bool            `json:"planOverride"`
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

type clashInfo struct {
	Title string    `json:"title"`
	Start time.Time `json:"start"`
	Type  string    `json:"type"`
}

type occurrencePreview struct {
	Start    time.Time  `json:"start"`
	Day      string     `json:"day"`
	Conflict *clashInfo `json:"conflict,omitempty"`
}

type seriesDraft struct {
	series    models.SessionSeries
	sessions  []models.Session
	preview   []occurrencePreview
	conflicts []time.Time
	planErr   error // plan fit / fullness problem (surfaced as a warning in preview)
}

// draftSeries validates a recurring request and lays out every occurrence
// with its clash (if any), without writing anything.
func (h *sessionHandler) draftSeries(ctx context.Context, in recurringInput) (*seriesDraft, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, badField("title", "title is required")
	}
	if in.DurationMin <= 0 || in.DurationMin > 24*60 {
		return nil, badField("durationMin", "duration must be between 1 and 1440 minutes")
	}
	if in.Capacity < 0 {
		return nil, badField("capacity", "capacity can't be negative (0 means unlimited)")
	}
	typ := defaultStr(in.Type, models.SessionGroup)
	if err := oneOf("type", typ, sessionTypes...); err != nil {
		return nil, err
	}
	spec := in.spec()
	starts, err := spec.occurrences()
	if err != nil {
		return nil, err
	}
	if len(starts) == 0 {
		return nil, badField("weekdays", "none of the chosen weekdays fall between those dates")
	}
	attendees, err := h.buildAttendees(ctx, in.Attendees, nil)
	if err != nil {
		return nil, err
	}
	for i := range attendees { // a new series is bookings only; outcomes come later
		attendees[i].Status = models.AttendBooked
	}
	if in.Capacity > 0 && len(attendees) > in.Capacity {
		return nil, capacityErr(in.Capacity, len(attendees))
	}
	occs := make([]occurrence, len(starts))
	for i, s := range starts {
		occs[i] = occurrence{s, typ}
	}
	d := &seriesDraft{}
	d.planErr = h.linkPlans(ctx, nil, attendees, nil, occs, in.PlanOverride)

	seriesID := primitive.NewObjectID().Hex()
	now := time.Now()
	d.series = models.SessionSeries{
		ID: primitive.NewObjectID(), SeriesID: seriesID, Title: strings.TrimSpace(in.Title), Type: typ,
		Weekdays: spec.Weekdays, Time: spec.Time, DurationMin: in.DurationMin, FromDay: spec.FromDay,
		ToDay: spec.ToDay, PlannedCount: len(starts), Status: "active", CreatedAt: now, UpdatedAt: now,
	}
	for _, start := range starts {
		// Every occurrence starts scheduled — even one dated in the past. A
		// class counts as completed only when the coach marks it so.
		d.sessions = append(d.sessions, models.Session{
			ID: primitive.NewObjectID(), Title: d.series.Title, Type: typ, Start: start,
			DurationMin: in.DurationMin, Location: strings.TrimSpace(in.Location), Capacity: in.Capacity,
			Fee: round2(in.Fee), SeriesID: seriesID, Status: models.SessScheduled,
			Attendees: attendees, CreatedAt: now, UpdatedAt: now,
		})
		p := occurrencePreview{Start: start, Day: models.DateKey(start)}
		clash, err := h.findOverlap(ctx, typ, start, in.DurationMin, nil)
		if err != nil {
			return nil, err
		}
		if clash != nil {
			p.Conflict = &clashInfo{clash.Title, clash.Start, clash.Type}
			d.conflicts = append(d.conflicts, start)
		}
		d.preview = append(d.preview, p)
	}
	return d, nil
}

// recurringPreview answers "what would this create?" — every date, which
// ones clash with existing bookings, and any plan problem — writing nothing.
// The real create revalidates everything.
func (h *sessionHandler) recurringPreview(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in recurringInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	d, err := h.draftSeries(ctx, in)
	if err != nil {
		return err
	}
	out := fiber.Map{"count": len(d.preview), "occurrences": d.preview, "conflicts": len(d.conflicts)}
	var ae *APIError
	if errors.As(d.planErr, &ae) {
		out["planProblem"] = fiber.Map{"code": ae.Code, "error": ae.Message, "details": ae.Details}
	} else if d.planErr != nil {
		return d.planErr
	}
	return c.JSON(out)
}

// recurring creates the series record and all its occurrences in one
// transaction. The whole series is rejected if any occurrence would clash
// (the error lists every clashing date so they can be adjusted) — no partial
// inserts.
func (h *sessionHandler) recurring(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in recurringInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	d, err := h.draftSeries(ctx, in)
	if err != nil {
		return err
	}
	if d.planErr != nil {
		return d.planErr
	}
	if len(d.conflicts) > 0 {
		dates := make([]string, len(d.conflicts))
		for i, t := range d.conflicts {
			dates[i] = t.In(time.Local).Format(time.RFC3339)
		}
		return apiErr(http.StatusConflict, CodeScheduleClash, fmt.Sprintf(
			"%d session(s) in this series overlap existing bookings (first: %s). %s",
			len(d.conflicts), d.conflicts[0].In(time.Local).Format("Mon Jan 2, 3:04 PM"), ruleHint(d.series.Type))).
			withDetails(map[string]any{"conflicts": dates, "occurrences": d.preview})
	}
	docs := make([]any, len(d.sessions))
	for i := range d.sessions {
		docs[i] = d.sessions[i]
	}
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		if _, err := h.store.Coll(models.CollSeries).InsertOne(tx, d.series); err != nil {
			return err
		}
		_, err := h.store.Coll(models.CollSessions).InsertMany(tx, docs)
		return err
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"created": len(docs), "seriesId": d.series.SeriesID, "plannedCount": d.series.PlannedCount, "sessions": d.sessions,
	})
}

// SeriesProgress counts a series' stored occurrences against its plan.
type SeriesProgress struct {
	SeriesID         string `json:"seriesId"`
	Title            string `json:"title"`
	Planned          int    `json:"planned"`
	Created          int    `json:"created"`
	Completed        int    `json:"completed"`
	Scheduled        int    `json:"scheduled"`
	Cancelled        int    `json:"cancelled"`
	AttendanceNeeded int    `json:"attendanceNeeded"`
	Inferred         bool   `json:"inferred,omitempty"`
	Status           string `json:"status"`
}

func needsAttendance(s models.Session) bool {
	if s.Status != models.SessCompleted {
		return false
	}
	for _, a := range s.Attendees {
		if a.Status == models.AttendBooked {
			return true
		}
	}
	return false
}

// seriesProgressFor computes progress for series ids. A series with no
// record (created before series were recorded, not yet migrated) reports its
// stored occurrences as planned, marked inferred.
func seriesProgressFor(ctx context.Context, store *db.Store, ids []string) (map[string]*SeriesProgress, map[string][]models.Session, error) {
	out := map[string]*SeriesProgress{}
	occ := map[string][]models.Session{}
	if len(ids) == 0 {
		return out, occ, nil
	}
	cur, err := store.Coll(models.CollSeries).Find(ctx, bson.M{"seriesId": bson.M{"$in": ids}})
	if err != nil {
		return nil, nil, err
	}
	var series []models.SessionSeries
	if err := cur.All(ctx, &series); err != nil {
		return nil, nil, err
	}
	for _, s := range series {
		out[s.SeriesID] = &SeriesProgress{SeriesID: s.SeriesID, Title: s.Title, Planned: s.PlannedCount, Inferred: s.Inferred, Status: s.Status}
	}
	scur, err := store.Coll(models.CollSessions).Find(ctx, bson.M{"seriesId": bson.M{"$in": ids}},
		options.Find().SetSort(bson.D{{Key: "start", Value: 1}}))
	if err != nil {
		return nil, nil, err
	}
	var sessions []models.Session
	if err := scur.All(ctx, &sessions); err != nil {
		return nil, nil, err
	}
	for _, s := range sessions {
		p := out[s.SeriesID]
		if p == nil {
			p = &SeriesProgress{SeriesID: s.SeriesID, Title: s.Title, Inferred: true, Status: "active"}
			out[s.SeriesID] = p
		}
		p.Created++
		switch s.Status {
		case models.SessCompleted:
			p.Completed++
		case models.SessCancelled:
			p.Cancelled++
		default:
			p.Scheduled++
		}
		if needsAttendance(s) {
			p.AttendanceNeeded++
		}
		occ[s.SeriesID] = append(occ[s.SeriesID], s)
	}
	for _, p := range out {
		if p.Planned == 0 && p.Inferred {
			p.Planned = p.Created
		}
	}
	return out, occ, nil
}

// seriesSummaries is GET /sessions/series?ids=a,b — progress for the series
// visible on a schedule week, in one request.
func (h *sessionHandler) seriesSummaries(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var ids []string
	for _, s := range strings.Split(c.Query("ids"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			ids = append(ids, s)
		}
	}
	if len(ids) > 100 {
		return badField("ids", "at most 100 series at a time")
	}
	prog, _, err := seriesProgressFor(ctx, h.store, ids)
	if err != nil {
		return err
	}
	out := make([]*SeriesProgress, 0, len(prog))
	for _, id := range ids {
		if p := prog[id]; p != nil {
			out = append(out, p)
		}
	}
	return c.JSON(out)
}

type occurrenceView struct {
	ID               string    `json:"id"`
	Start            time.Time `json:"start"`
	Status           string    `json:"status"`
	Booked           int       `json:"booked"`
	AttendanceNeeded bool      `json:"attendanceNeeded"`
}

func (h *sessionHandler) seriesProgress(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id := c.Params("seriesId")
	prog, occ, err := seriesProgressFor(ctx, h.store, []string{id})
	if err != nil {
		return err
	}
	p := prog[id]
	if p == nil {
		return notFound("series")
	}
	views := make([]occurrenceView, 0, len(occ[id]))
	for _, s := range occ[id] {
		views = append(views, occurrenceView{ID: s.ID.Hex(), Start: s.Start, Status: s.Status, Booked: len(s.Attendees), AttendanceNeeded: needsAttendance(s)})
	}
	var series models.SessionSeries
	_ = h.store.Coll(models.CollSeries).FindOne(ctx, bson.M{"seriesId": id}).Decode(&series)
	return c.JSON(fiber.Map{"progress": p, "series": series, "occurrences": views})
}

// seriesRecord loads (or, for a legacy series, infers and stores) the
// series document so edits can update its plan in the same transaction.
func (h *sessionHandler) seriesRecord(ctx context.Context, seriesID string) (models.SessionSeries, error) {
	var s models.SessionSeries
	err := h.store.Coll(models.CollSeries).FindOne(ctx, bson.M{"seriesId": seriesID}).Decode(&s)
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return s, err
	}
	prog, occ, err := seriesProgressFor(ctx, h.store, []string{seriesID})
	if err != nil {
		return s, err
	}
	p := prog[seriesID]
	if p == nil || len(occ[seriesID]) == 0 {
		return s, notFound("series")
	}
	s = inferSeries(seriesID, occ[seriesID], time.Now())
	if _, err := h.store.Coll(models.CollSeries).InsertOne(ctx, s); err != nil && !isDup(err) {
		return s, err
	}
	return s, nil
}

// inferSeries rebuilds a series record from its stored occurrences (sorted).
func inferSeries(seriesID string, occ []models.Session, now time.Time) models.SessionSeries {
	first, last := occ[0], occ[len(occ)-1]
	days := map[int]bool{}
	var weekdays []int
	for _, s := range occ {
		wd := int(s.Start.In(time.Local).Weekday())
		if !days[wd] {
			days[wd] = true
			weekdays = append(weekdays, wd)
		}
	}
	return models.SessionSeries{
		ID: primitive.NewObjectID(), SeriesID: seriesID, Title: first.Title, Type: first.Type, Weekdays: weekdays,
		Time: first.Start.In(time.Local).Format("15:04"), DurationMin: first.DurationMin,
		FromDay: models.DateKey(first.Start), ToDay: models.DateKey(last.Start),
		PlannedCount: len(occ), Status: "active", Inferred: true, CreatedAt: now, UpdatedAt: now,
	}
}

type seriesChanges struct {
	Title       *string `json:"title"`
	Time        *string `json:"time"` // "HH:MM" studio wall clock
	DurationMin *int    `json:"durationMin"`
	Location    *string `json:"location"`
	Capacity    *int    `json:"capacity"`
	Type        *string `json:"type"`
	Status      *string `json:"status"` // "cancelled" or back to "scheduled"
}

type seriesEditInput struct {
	Scope          string        `json:"scope"`          // "future": this occurrence and every later one
	FromOccurrence string        `json:"fromOccurrence"` // session id within the series
	Changes        seriesChanges `json:"changes"`
	Reason         string        `json:"reason"`
}

// seriesTargets are the occurrences a "this and future" edit touches: from
// the chosen occurrence on, never one already completed — recorded classes
// and their attendance are history and stay exactly as they were.
func (h *sessionHandler) seriesTargets(ctx context.Context, seriesID, fromHex string) ([]models.Session, error) {
	fromID, err := parseOID(fromHex, "fromOccurrence")
	if err != nil {
		return nil, err
	}
	from, err := findSession(ctx, h.store, fromID)
	if err != nil {
		return nil, err
	}
	if from.SeriesID != seriesID {
		return nil, badField("fromOccurrence", "that session isn't part of this series")
	}
	cur, err := h.store.Coll(models.CollSessions).Find(ctx, bson.M{
		"seriesId": seriesID, "start": bson.M{"$gte": from.Start}, "status": bson.M{"$ne": models.SessCompleted},
	}, options.Find().SetSort(bson.D{{Key: "start", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var out []models.Session
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func validateSeriesChanges(ch seriesChanges) (hh, mm int, err error) {
	if ch.Time != nil {
		m := hhmmRe.FindStringSubmatch(*ch.Time)
		if m == nil {
			return 0, 0, badField("changes.time", "time must be HH:MM (24-hour)")
		}
		hh, mm = atoiDefault(m[1], 0), atoiDefault(m[2], 0)
	}
	if ch.DurationMin != nil && (*ch.DurationMin <= 0 || *ch.DurationMin > 24*60) {
		return 0, 0, badField("changes.durationMin", "duration must be between 1 and 1440 minutes")
	}
	if ch.Capacity != nil && *ch.Capacity < 0 {
		return 0, 0, badField("changes.capacity", "capacity can't be negative (0 means unlimited)")
	}
	if ch.Type != nil {
		if err := oneOf("changes.type", *ch.Type, sessionTypes...); err != nil {
			return 0, 0, err
		}
	}
	if ch.Status != nil {
		if err := oneOf("changes.status", *ch.Status, models.SessScheduled, models.SessCancelled); err != nil {
			return 0, 0, err
		}
	}
	if ch.Title != nil && strings.TrimSpace(*ch.Title) == "" {
		return 0, 0, badField("changes.title", "title can't be empty")
	}
	return hh, mm, nil
}

// seriesEdit applies one change to "this occurrence and every later one",
// atomically: every target moves together or none does. Time changes keep
// each occurrence's studio day; clashes and capacity are checked for every
// target first and reported with their dates.
func (h *sessionHandler) seriesEdit(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	seriesID := c.Params("seriesId")
	var in seriesEditInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	if in.Scope != "future" {
		return badField("scope", `scope must be "future" (edit a single occurrence with PUT /sessions/:id)`)
	}
	reason, err := requireReason(in.Reason, "change a whole series")
	if err != nil {
		return err
	}
	ch := in.Changes
	hh, mm, err := validateSeriesChanges(ch)
	if err != nil {
		return err
	}
	actor := actorOf(c)
	var changed int
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		changed = 0
		series, err := h.seriesRecord(tx, seriesID)
		if err != nil {
			return err
		}
		targets, err := h.seriesTargets(tx, seriesID, in.FromOccurrence)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			return badField("fromOccurrence", "no upcoming occurrences to change from there")
		}
		ids := make([]primitive.ObjectID, len(targets))
		for i, s := range targets {
			ids[i] = s.ID
		}
		var clashes, full []string
		for i := range targets {
			s := &targets[i]
			before := *s
			if ch.Title != nil {
				s.Title = strings.TrimSpace(*ch.Title)
			}
			if ch.Time != nil {
				d := s.Start.In(time.Local)
				s.Start = time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, time.Local)
			}
			if ch.DurationMin != nil {
				s.DurationMin = *ch.DurationMin
			}
			if ch.Location != nil {
				s.Location = strings.TrimSpace(*ch.Location)
			}
			if ch.Capacity != nil {
				s.Capacity = *ch.Capacity
			}
			if ch.Type != nil {
				s.Type = *ch.Type
			}
			if ch.Status != nil {
				s.Status = *ch.Status
			}
			if s.Capacity > 0 && len(s.Attendees) > s.Capacity {
				full = append(full, models.DateKey(s.Start))
			}
			if s.Status != models.SessCancelled && (ch.Time != nil || ch.DurationMin != nil || ch.Type != nil || ch.Status != nil) {
				clash, err := h.findOverlapExcluding(tx, s.Type, s.Start, s.DurationMin, ids)
				if err != nil {
					return err
				}
				if clash != nil {
					clashes = append(clashes, s.Start.In(time.Local).Format(time.RFC3339))
				}
			}
			if s.Title == before.Title && s.Start.Equal(before.Start) && s.DurationMin == before.DurationMin &&
				s.Location == before.Location && s.Capacity == before.Capacity && s.Type == before.Type && s.Status == before.Status {
				continue
			}
			if _, err := h.store.Coll(models.CollSessions).UpdateOne(tx, bson.M{"_id": s.ID}, bson.M{"$set": bson.M{
				"title": s.Title, "start": s.Start, "durationMin": s.DurationMin, "location": s.Location,
				"capacity": s.Capacity, "type": s.Type, "status": s.Status, "updatedAt": time.Now(),
			}}); err != nil {
				return err
			}
			changed++
		}
		if len(full) > 0 {
			return apiErr(http.StatusConflict, CodeCapacity, fmt.Sprintf("%d occurrence(s) already have more bookings than that capacity", len(full))).
				withField("changes.capacity").withDetails(map[string]any{"days": full})
		}
		if len(clashes) > 0 {
			return apiErr(http.StatusConflict, CodeScheduleClash, fmt.Sprintf("%d occurrence(s) would overlap other bookings", len(clashes))).
				withDetails(map[string]any{"conflicts": clashes})
		}
		set := bson.M{"updatedAt": time.Now()}
		if ch.Title != nil {
			set["title"] = strings.TrimSpace(*ch.Title)
		}
		if ch.Time != nil {
			set["time"] = *ch.Time
		}
		if ch.DurationMin != nil {
			set["durationMin"] = *ch.DurationMin
		}
		if ch.Type != nil {
			set["type"] = *ch.Type
		}
		if _, err := h.store.Coll(models.CollSeries).UpdateOne(tx, bson.M{"_id": series.ID}, bson.M{"$set": set}); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "series", Ref: series.ID, Action: "edit_future",
			After: fiber.Map{"fromOccurrence": in.FromOccurrence, "changes": in.Changes, "occurrences": changed}, Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true, "changed": changed})
}

// seriesExtend adds occurrences after the series' current end, on the same
// weekdays and time, and raises its planned count — an explicit, audited
// change to the denominator.
func (h *sessionHandler) seriesExtend(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	seriesID := c.Params("seriesId")
	var in struct {
		ToDay  string `json:"toDay"`
		Reason string `json:"reason"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	reason, err := requireReason(in.Reason, "extend a series")
	if err != nil {
		return err
	}
	actor := actorOf(c)
	added := 0
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		added = 0
		series, err := h.seriesRecord(tx, seriesID)
		if err != nil {
			return err
		}
		_, occ, err := seriesProgressFor(tx, h.store, []string{seriesID})
		if err != nil {
			return err
		}
		all := occ[seriesID]
		if len(all) == 0 || len(series.Weekdays) == 0 {
			return badField("seriesId", "this series has no occurrences to continue from")
		}
		last := all[len(all)-1]
		fromDay := models.DateKey(last.Start.AddDate(0, 0, 1))
		starts, err := seriesSpec{Weekdays: series.Weekdays, Time: defaultStr(series.Time, last.Start.In(time.Local).Format("15:04")), FromDay: fromDay, ToDay: in.ToDay}.occurrences()
		if err != nil {
			return err
		}
		if len(starts) == 0 {
			return badField("toDay", "no more occurrences fall before that date")
		}
		now := time.Now()
		var docs []any
		var clashes []string
		for _, st := range starts {
			clash, err := h.findOverlap(tx, last.Type, st, last.DurationMin, nil)
			if err != nil {
				return err
			}
			if clash != nil {
				clashes = append(clashes, st.In(time.Local).Format(time.RFC3339))
				continue
			}
			// New occurrences carry the roster's bookings, not their marks.
			att := make([]models.Attendee, len(last.Attendees))
			for i, a := range last.Attendees {
				att[i] = models.Attendee{Trainee: a.Trainee, TraineeName: a.TraineeName, Status: models.AttendBooked, PlanID: a.PlanID}
			}
			docs = append(docs, models.Session{
				ID: primitive.NewObjectID(), Title: last.Title, Type: last.Type, Start: st, DurationMin: last.DurationMin,
				Location: last.Location, Capacity: last.Capacity, Fee: last.Fee, SeriesID: seriesID,
				Status: models.SessScheduled, Attendees: att, CreatedAt: now, UpdatedAt: now,
			})
		}
		if len(clashes) > 0 {
			return apiErr(http.StatusConflict, CodeScheduleClash, fmt.Sprintf("%d new occurrence(s) would overlap other bookings", len(clashes))).
				withDetails(map[string]any{"conflicts": clashes})
		}
		if _, err := h.store.Coll(models.CollSessions).InsertMany(tx, docs); err != nil {
			return err
		}
		added = len(docs)
		before := series
		if _, err := h.store.Coll(models.CollSeries).UpdateOne(tx, bson.M{"_id": series.ID}, bson.M{"$set": bson.M{
			"plannedCount": series.PlannedCount + added, "toDay": in.ToDay, "status": "active", "updatedAt": now,
		}}); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "series", Ref: series.ID, Action: "extend",
			Before: fiber.Map{"plannedCount": before.PlannedCount, "toDay": before.ToDay},
			After:  fiber.Map{"plannedCount": before.PlannedCount + added, "toDay": in.ToDay}, Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true, "added": added})
}

// seriesEnd ends a series early: every scheduled occurrence from the chosen
// one (default: from now) is cancelled — kept, greyed out — and the planned
// count drops by that many, recorded with the reason.
func (h *sessionHandler) seriesEnd(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	seriesID := c.Params("seriesId")
	var in struct {
		FromOccurrence string `json:"fromOccurrence"`
		Reason         string `json:"reason"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	reason, err := requireReason(in.Reason, "end a series early")
	if err != nil {
		return err
	}
	actor := actorOf(c)
	ended := 0
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		ended = 0
		series, err := h.seriesRecord(tx, seriesID)
		if err != nil {
			return err
		}
		filter := bson.M{"seriesId": seriesID, "status": models.SessScheduled}
		if in.FromOccurrence != "" {
			targets, err := h.seriesTargets(tx, seriesID, in.FromOccurrence)
			if err != nil {
				return err
			}
			ids := make([]primitive.ObjectID, 0, len(targets))
			for _, s := range targets {
				ids = append(ids, s.ID)
			}
			filter["_id"] = bson.M{"$in": ids}
		} else {
			filter["start"] = bson.M{"$gte": time.Now()}
		}
		res, err := h.store.Coll(models.CollSessions).UpdateMany(tx, filter,
			bson.M{"$set": bson.M{"status": models.SessCancelled, "updatedAt": time.Now()}})
		if err != nil {
			return err
		}
		ended = int(res.ModifiedCount)
		after := max(series.PlannedCount-ended, 0)
		if _, err := h.store.Coll(models.CollSeries).UpdateOne(tx, bson.M{"_id": series.ID}, bson.M{"$set": bson.M{
			"plannedCount": after, "status": "ended", "updatedAt": time.Now(),
		}}); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "series", Ref: series.ID, Action: "end",
			Before: fiber.Map{"plannedCount": series.PlannedCount}, After: fiber.Map{"plannedCount": after, "cancelled": ended},
			Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true, "cancelled": ended})
}
