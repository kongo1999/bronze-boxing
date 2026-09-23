package server

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"bronzeboxing/internal/models"
)

type sessionOut struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Start     string `json:"start"`
	Title     string `json:"title"`
	SeriesID  string `json:"seriesId"`
	Capacity  int    `json:"capacity"`
	Attendees []struct {
		Trainee string `json:"trainee"`
		Status  string `json:"status"`
		PlanID  string `json:"planId"`
	} `json:"attendees"`
}

type planOut struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Progress struct {
		Target           int `json:"target"`
		Completed        int `json:"completed"`
		Remaining        int `json:"remaining"`
		UpcomingBooked   int `json:"upcomingBooked"`
		AttendanceNeeded int `json:"attendanceNeeded"`
		NoShows          int `json:"noShows"`
		Cancelled        int `json:"cancelled"`
		UnassignedSlots  int `json:"unassignedSlots"`
	} `json:"progress"`
}

type seriesProgOut struct {
	Progress struct {
		Planned          int  `json:"planned"`
		Created          int  `json:"created"`
		Completed        int  `json:"completed"`
		Scheduled        int  `json:"scheduled"`
		Cancelled        int  `json:"cancelled"`
		AttendanceNeeded int  `json:"attendanceNeeded"`
		Inferred         bool `json:"inferred"`
	} `json:"progress"`
	Occurrences []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Start  string `json:"start"`
	} `json:"occurrences"`
}

func (e *testEnv) plan(traineeID string) planOut {
	e.t.Helper()
	var plans []planOut
	e.ok("GET", "/trainees/"+traineeID+"/session-plans", nil, &plans)
	if len(plans) == 0 {
		e.t.Fatal("no plans")
	}
	return plans[0]
}

func (e *testEnv) seriesProg(seriesID string) seriesProgOut {
	e.t.Helper()
	var p seriesProgOut
	e.ok("GET", "/sessions/series/"+seriesID+"/progress", nil, &p)
	return p
}

func (e *testEnv) mark(sessionID, trainee, status string) {
	e.t.Helper()
	e.ok("PATCH", "/sessions/"+sessionID+"/attendance", map[string]any{"trainee": trainee, "status": status}, nil)
}

func (e *testEnv) setStatus(sessionID, status string) {
	e.t.Helper()
	e.ok("PUT", "/sessions/"+sessionID, map[string]any{"status": status}, nil)
}

// The review's acceptance scenario: a 12-occurrence series and a 12-session
// plan both read 9/12 once nine classes are completed with the trainee
// attended. A completed class where they're still "booked" is flagged and
// not credited; a no-show and a cancellation show separately; repeating an
// update, reopening a class and reassigning a booking never double count.
func TestSeriesAndPlanCountersReachNineOfTwelve(t *testing.T) {
	e := newEnv(t)
	tr := e.addTrainee("Jad", 0)
	var plan planOut
	e.ok("POST", "/trainees/"+tr.ID+"/session-plans", map[string]any{
		"title": "12 private sessions", "targetCount": 12, "startDate": "2026-01-01", "sessionType": "private",
	}, &plan)

	var created struct {
		SeriesID     string       `json:"seriesId"`
		PlannedCount int          `json:"plannedCount"`
		Sessions     []sessionOut `json:"sessions"`
	}
	e.ok("POST", "/sessions/recurring", map[string]any{
		"title": "PT — Jad", "type": "private", "weekdays": []int{1, 4}, "time": "17:00", "durationMin": 60,
		"fromDay": "2027-01-04", "toDay": "2027-02-11", // Mon+Thu for six weeks = 12
		"attendees": []map[string]any{{"trainee": tr.ID, "planId": plan.ID}},
	}, &created)
	if created.PlannedCount != 12 || len(created.Sessions) != 12 {
		t.Fatalf("created %d, planned %d, want 12", len(created.Sessions), created.PlannedCount)
	}
	for _, s := range created.Sessions {
		if s.Status != models.SessScheduled {
			t.Fatalf("new occurrences start scheduled, got %s", s.Status)
		}
	}
	ids := make([]string, 12)
	for i, s := range created.Sessions {
		ids[i] = s.ID
	}
	p := e.plan(tr.ID).Progress
	if p.Completed != 0 || p.UpcomingBooked != 12 || p.UnassignedSlots != 0 {
		t.Fatalf("fresh plan = %+v", p)
	}

	// Nine classes happen, Jad attends all nine.
	for _, id := range ids[:9] {
		e.setStatus(id, models.SessCompleted)
		e.mark(id, tr.ID, models.AttendAttended)
	}
	sp := e.seriesProg(created.SeriesID).Progress
	p = e.plan(tr.ID).Progress
	if sp.Completed != 9 || sp.Planned != 12 || p.Completed != 9 || p.Remaining != 3 {
		t.Fatalf("after nine: series %d/%d, plan %d/%d remaining %d", sp.Completed, sp.Planned, p.Completed, p.Target, p.Remaining)
	}
	// Repeating the same marks changes nothing.
	e.mark(ids[0], tr.ID, models.AttendAttended)
	e.setStatus(ids[0], models.SessCompleted)
	if got := e.plan(tr.ID).Progress.Completed; got != 9 {
		t.Fatalf("repeat updates double counted: %d", got)
	}

	// Class 10 is completed but Jad is left "booked": flagged, not credited.
	e.setStatus(ids[9], models.SessCompleted)
	sp = e.seriesProg(created.SeriesID).Progress
	p = e.plan(tr.ID).Progress
	if sp.Completed != 10 || sp.AttendanceNeeded != 1 || p.Completed != 9 || p.AttendanceNeeded != 1 {
		t.Fatalf("unmarked class: series %+v plan %+v", sp, p)
	}
	// Class 11: a no-show. Class 12: cancelled. Both separate, neither credited.
	e.setStatus(ids[10], models.SessCompleted)
	e.mark(ids[10], tr.ID, models.AttendNoShow)
	e.setStatus(ids[11], models.SessCancelled)
	sp = e.seriesProg(created.SeriesID).Progress
	p = e.plan(tr.ID).Progress
	if p.Completed != 9 || p.NoShows != 1 || p.Cancelled != 1 || p.Remaining != 3 {
		t.Fatalf("no-show/cancel: plan %+v", p)
	}
	if sp.Planned != 12 || sp.Cancelled != 1 || sp.Completed != 11 {
		t.Fatalf("series after cancel: %+v (planned must not shrink on a cancellation)", sp)
	}

	// Reopening a class removes its credit; completing it again restores it.
	e.setStatus(ids[0], models.SessScheduled)
	if got := e.plan(tr.ID).Progress.Completed; got != 8 {
		t.Fatalf("reopened class still credited: %d", got)
	}
	e.setStatus(ids[0], models.SessCompleted)

	// Reassigning one attended booking to a second plan moves the credit.
	var plan2 planOut
	e.ok("POST", "/trainees/"+tr.ID+"/session-plans", map[string]any{"title": "Bonus", "targetCount": 2, "startDate": "2026-01-01"}, &plan2)
	e.ok("PATCH", "/sessions/"+ids[1]+"/attendance", map[string]any{"trainee": tr.ID, "status": "attended", "planId": plan2.ID}, nil)
	var plans []planOut
	e.ok("GET", "/trainees/"+tr.ID+"/session-plans", nil, &plans)
	byID := map[string]planOut{}
	for _, pl := range plans {
		byID[pl.ID] = pl
	}
	if byID[plan.ID].Progress.Completed != 8 || byID[plan2.ID].Progress.Completed != 1 {
		t.Fatalf("after reassign: %d / %d, want 8 / 1", byID[plan.ID].Progress.Completed, byID[plan2.ID].Progress.Completed)
	}
}

func TestPlanRulesRefuseMismatchAndOverbooking(t *testing.T) {
	e := newEnv(t)
	jad := e.addTrainee("Jad", 0)
	lara := e.addTrainee("Lara", 0)
	var plan planOut
	e.ok("POST", "/trainees/"+jad.ID+"/session-plans", map[string]any{
		"title": "3 sessions", "targetCount": 3, "startDate": "2027-03-01", "endDate": "2027-03-31",
	}, &plan)
	at := func(day string) time.Time {
		t0, _ := time.ParseInLocation("2006-01-02 15:04", day+" 10:00", time.Local)
		return t0
	}
	post := func(day, trainee string, override bool) (int, []byte) {
		return e.req("POST", "/sessions", map[string]any{
			"title": "PT", "type": "private", "start": at(day), "durationMin": 60, "planOverride": override,
			"attendees": []map[string]any{{"trainee": trainee, "planId": plan.ID}},
		})
	}
	// Someone else's plan, or a date outside the plan: refused.
	if st, _ := post("2027-03-02", lara.ID, false); st != http.StatusBadRequest {
		t.Fatalf("other trainee's plan: %d", st)
	}
	if st, _ := post("2027-04-02", jad.ID, false); st != http.StatusBadRequest {
		t.Fatalf("outside plan window: %d", st)
	}
	for _, d := range []string{"2027-03-02", "2027-03-03", "2027-03-04"} {
		if st, raw := post(d, jad.ID, false); st != http.StatusCreated {
			t.Fatalf("booking %s: %d %s", d, st, raw)
		}
	}
	// A fourth booking exceeds the target: refused unless explicitly overridden.
	st, raw := post("2027-03-05", jad.ID, false)
	if st != http.StatusConflict || !strings.Contains(string(raw), CodePlanFull) {
		t.Fatalf("fourth booking: %d %s", st, raw)
	}
	if st, _ := post("2027-03-05", jad.ID, true); st != http.StatusCreated {
		t.Fatalf("override: %d", st)
	}
	if p := e.plan(jad.ID).Progress; p.UpcomingBooked != 4 || p.Remaining != 3 {
		t.Fatalf("plan = %+v", p)
	}
}

// Capacity holds under concurrent bookings; 0 means unlimited; a capacity
// can't drop below who's already booked.
func TestCapacityIsEnforcedEverywhere(t *testing.T) {
	e := newEnv(t)
	var trainees []traineeOut
	for i := 0; i < 8; i++ {
		trainees = append(trainees, e.addTrainee("T"+string(rune('A'+i)), 0))
	}
	start := time.Date(2027, 5, 3, 18, 0, 0, 0, time.Local)
	var s sessionOut
	e.ok("POST", "/sessions", map[string]any{"title": "Group", "type": "group", "start": start, "durationMin": 60, "capacity": 3}, &s)
	e.fail("POST", "/sessions", map[string]any{"title": "Full", "type": "group", "start": start.Add(3 * time.Hour), "durationMin": 60, "capacity": 1,
		"attendees": []map[string]any{{"trainee": trainees[0].ID}, {"trainee": trainees[1].ID}}}, http.StatusConflict, CodeCapacity)

	var wg sync.WaitGroup
	codes := make([]int, len(trainees))
	for i, tr := range trainees {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			codes[i], _ = e.req("PATCH", "/sessions/"+s.ID+"/attendance", map[string]any{"trainee": id, "status": "booked"})
		}(i, tr.ID)
	}
	wg.Wait()
	ok := 0
	for _, c := range codes {
		if c == http.StatusOK {
			ok++
		}
	}
	var got sessionOut
	e.ok("GET", "/sessions/"+s.ID, nil, &got)
	if ok != 3 || len(got.Attendees) != 3 {
		t.Fatalf("booked %d (responses ok %d), want exactly 3", len(got.Attendees), ok)
	}
	// Re-marking someone already booked still works when full.
	e.mark(s.ID, got.Attendees[0].Trainee, models.AttendAttended)
	e.fail("PUT", "/sessions/"+s.ID, map[string]any{"capacity": 2}, http.StatusConflict, CodeCapacity)
	e.ok("PUT", "/sessions/"+s.ID, map[string]any{"capacity": 0}, nil) // unlimited
	// Who won the race is random: book someone who isn't already in.
	in := map[string]bool{}
	for _, a := range got.Attendees {
		in[a.Trainee] = true
	}
	for _, tr := range trainees {
		if !in[tr.ID] {
			e.mark(s.ID, tr.ID, models.AttendBooked)
			break
		}
	}

	// Attendance recorded: the class can be cancelled, not deleted.
	e.fail("DELETE", "/sessions/"+s.ID, nil, http.StatusConflict, CodeConflict)
	e.setStatus(s.ID, models.SessCancelled)
}

func TestRecurringPreviewListsClashesAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	blocker := time.Date(2027, 6, 9, 18, 30, 0, 0, time.Local) // a Wednesday
	e.ok("POST", "/sessions", map[string]any{"title": "Existing group", "type": "group", "start": blocker, "durationMin": 60}, nil)
	body := map[string]any{"title": "Evening", "type": "group", "weekdays": []int{1, 3}, "time": "18:00", "durationMin": 60,
		"fromDay": "2027-06-07", "toDay": "2027-06-20"}
	var pv struct {
		Count       int `json:"count"`
		Conflicts   int `json:"conflicts"`
		Occurrences []struct {
			Day      string `json:"day"`
			Conflict *struct {
				Title string `json:"title"`
			} `json:"conflict"`
		} `json:"occurrences"`
	}
	before := e.count(models.CollSessions, bson.M{})
	e.ok("POST", "/sessions/recurring/preview", body, &pv)
	if pv.Count != 4 || pv.Conflicts != 1 || e.count(models.CollSessions, bson.M{}) != before {
		t.Fatalf("preview = %+v (sessions %d→%d)", pv, before, e.count(models.CollSessions, bson.M{}))
	}
	for _, o := range pv.Occurrences {
		if (o.Day == "2027-06-09") != (o.Conflict != nil) {
			t.Fatalf("conflict flagged on the wrong day: %+v", o)
		}
	}
	eb := e.fail("POST", "/sessions/recurring", body, http.StatusConflict, CodeScheduleClash)
	if cs, _ := eb.Details["conflicts"].([]any); len(cs) != 1 {
		t.Fatalf("create should list the clashing date: %v", eb.Details)
	}
	if e.count(models.CollSessions, bson.M{}) != before || e.count(models.CollSeries, bson.M{}) != 0 {
		t.Fatal("a rejected series must write nothing")
	}
}

// "This and future" edits move every later occurrence together, leave
// completed classes (and their attendance) untouched, and ending a series
// early lowers its planned count by what it cancelled.
func TestSeriesFutureEditsPreserveThePast(t *testing.T) {
	e := newEnv(t)
	tr := e.addTrainee("Nour", 0)
	var created struct {
		SeriesID string       `json:"seriesId"`
		Sessions []sessionOut `json:"sessions"`
	}
	e.ok("POST", "/sessions/recurring", map[string]any{
		"title": "Morning", "type": "group", "weekdays": []int{2}, "time": "07:30", "durationMin": 50,
		"fromDay": "2027-09-07", "toDay": "2027-10-26", "attendees": []map[string]any{{"trainee": tr.ID}},
	}, &created)
	occ := created.Sessions // 8 Tuesdays
	if len(occ) != 8 {
		t.Fatalf("got %d occurrences", len(occ))
	}
	e.setStatus(occ[0].ID, models.SessCompleted)
	e.mark(occ[0].ID, tr.ID, models.AttendAttended)
	e.setStatus(occ[1].ID, models.SessCompleted)
	e.mark(occ[1].ID, tr.ID, models.AttendAttended)

	e.fail("PATCH", "/sessions/series/"+created.SeriesID, map[string]any{"scope": "future", "fromOccurrence": occ[0].ID,
		"changes": map[string]any{"time": "08:00"}}, http.StatusBadRequest, CodeReasonRequired)
	var res struct {
		Changed int `json:"changed"`
	}
	e.ok("PATCH", "/sessions/series/"+created.SeriesID, map[string]any{"scope": "future", "fromOccurrence": occ[0].ID,
		"changes": map[string]any{"time": "08:00", "title": "Morning (new time)"}, "reason": "gym opens later"}, &res)
	if res.Changed != 6 {
		t.Fatalf("changed %d, want the 6 not-yet-completed occurrences", res.Changed)
	}
	var first, third sessionOut
	e.ok("GET", "/sessions/"+occ[0].ID, nil, &first)
	e.ok("GET", "/sessions/"+occ[2].ID, nil, &third)
	firstStart, _ := time.Parse(time.RFC3339, first.Start)
	thirdStart, _ := time.Parse(time.RFC3339, third.Start)
	if firstStart.In(time.Local).Format("15:04") != "07:30" || first.Attendees[0].Status != models.AttendAttended {
		t.Fatalf("completed class changed: %s %s", firstStart.In(time.Local), first.Attendees[0].Status)
	}
	if thirdStart.In(time.Local).Format("15:04") != "08:00" || models.DateKey(thirdStart) != "2027-09-21" || third.Title != "Morning (new time)" {
		t.Fatalf("future class = %s %q", thirdStart.In(time.Local), third.Title)
	}

	// End the series from occurrence 6: three cancelled, planned 8 → 5.
	var end struct {
		Cancelled int `json:"cancelled"`
	}
	e.ok("POST", "/sessions/series/"+created.SeriesID+"/end", map[string]any{"fromOccurrence": occ[5].ID, "reason": "summer break"}, &end)
	sp := e.seriesProg(created.SeriesID).Progress
	if end.Cancelled != 3 || sp.Planned != 5 || sp.Cancelled != 3 || sp.Completed != 2 {
		t.Fatalf("after end: cancelled %d, progress %+v", end.Cancelled, sp)
	}
	// Extend it again by two weeks: planned grows by what was added.
	var ext struct {
		Added int `json:"added"`
	}
	e.ok("POST", "/sessions/series/"+created.SeriesID+"/extend", map[string]any{"toDay": "2027-11-09", "reason": "back on"}, &ext)
	if sp = e.seriesProg(created.SeriesID).Progress; ext.Added != 2 || sp.Planned != 7 {
		t.Fatalf("after extend: added %d, planned %d", ext.Added, sp.Planned)
	}
	var trail []struct {
		Action string `json:"action"`
	}
	var seriesDoc models.SessionSeries
	if err := e.store.Coll(models.CollSeries).FindOne(e.ctx, bson.M{"seriesId": created.SeriesID}).Decode(&seriesDoc); err != nil {
		t.Fatal(err)
	}
	e.ok("GET", "/audit/series/"+seriesDoc.ID.Hex(), nil, &trail)
	if len(trail) != 3 {
		t.Fatalf("series audit = %+v, want edit/end/extend", trail)
	}
}

// A legacy series (occurrences only, from before series were recorded) gets
// an inferred record from the migration.
func TestLegacySeriesIsInferred(t *testing.T) {
	e := newEnv(t)
	start := time.Date(2027, 1, 5, 18, 0, 0, 0, time.Local)
	var docs []any
	for i := 0; i < 4; i++ {
		docs = append(docs, models.Session{Title: "Old series", Type: "group", Start: start.AddDate(0, 0, 7*i),
			DurationMin: 60, SeriesID: "legacy-1", Status: models.SessScheduled, Attendees: []models.Attendee{}})
	}
	if _, err := e.store.Coll(models.CollSessions).InsertMany(e.ctx, docs); err != nil {
		t.Fatal(err)
	}
	var sum []struct {
		Planned  int  `json:"planned"`
		Inferred bool `json:"inferred"`
	}
	e.ok("GET", "/sessions/series?ids=legacy-1", nil, &sum)
	if len(sum) != 1 || sum[0].Planned != 4 || !sum[0].Inferred {
		t.Fatalf("summary = %+v", sum)
	}
	r := runMigration(t, e, "2026-09-006-series-and-plans")
	if r["series_inferred"] != 1 {
		t.Fatalf("migration result = %v", r)
	}
}

// A class can't be marked done, nor anyone attended or a no-show, before it
// starts (a short grace allows taking attendance at the door). Bookings,
// cancelling and logging a past class with its outcome stay allowed.
func TestOutcomesWaitForTheClassToStart(t *testing.T) {
	e := newEnv(t)
	now := time.Date(2027, 3, 10, 12, 0, 0, 0, time.Local)
	clockNow = func() time.Time { return now }
	tr := e.addTrainee("Jad", 0)

	var future sessionOut
	e.ok("POST", "/sessions", map[string]any{
		"title": "Evening", "type": "group", "start": now.Add(6 * time.Hour), "durationMin": 60,
		"attendees": []map[string]any{{"trainee": tr.ID}},
	}, &future)
	e.fail("PUT", "/sessions/"+future.ID, map[string]any{"status": models.SessCompleted}, http.StatusUnprocessableEntity, CodeNotStarted)
	e.fail("PATCH", "/sessions/"+future.ID+"/attendance", map[string]any{"trainee": tr.ID, "status": models.AttendAttended}, http.StatusUnprocessableEntity, CodeNotStarted)
	e.fail("PATCH", "/sessions/"+future.ID+"/attendance", map[string]any{"trainee": tr.ID, "status": models.AttendNoShow}, http.StatusUnprocessableEntity, CodeNotStarted)
	e.fail("POST", "/sessions/"+future.ID+"/attendance/bulk", map[string]any{"status": models.AttendAttended, "only": models.AttendBooked}, http.StatusUnprocessableEntity, CodeNotStarted)
	e.fail("PUT", "/sessions/"+future.ID, map[string]any{"attendees": []map[string]any{{"trainee": tr.ID, "status": models.AttendAttended}}}, http.StatusUnprocessableEntity, CodeNotStarted)
	e.fail("POST", "/sessions", map[string]any{
		"title": "Later", "type": "private", "start": now.Add(48 * time.Hour), "durationMin": 60, "status": models.SessCompleted,
	}, http.StatusUnprocessableEntity, CodeNotStarted)
	// Still fine before it starts: booking, cancelling, reopening.
	e.mark(future.ID, tr.ID, models.AttendBooked)
	e.setStatus(future.ID, models.SessCancelled)
	e.setStatus(future.ID, models.SessScheduled)

	// Within the grace window, attendance can be taken at the door.
	now = now.Add(6*time.Hour - 20*time.Minute) // twenty minutes before the start
	e.mark(future.ID, tr.ID, models.AttendAttended)
	e.setStatus(future.ID, models.SessCompleted)
	// A completed class can't be moved into the future.
	e.fail("PUT", "/sessions/"+future.ID, map[string]any{"start": now.Add(72 * time.Hour)}, http.StatusUnprocessableEntity, CodeNotStarted)

	// Logging yesterday's class with its outcome is allowed.
	e.ok("POST", "/sessions", map[string]any{
		"title": "Yesterday", "type": "private", "start": now.Add(-24 * time.Hour), "durationMin": 60, "status": models.SessCompleted,
		"attendees": []map[string]any{{"trainee": tr.ID, "status": models.AttendAttended}},
	}, nil)
}
