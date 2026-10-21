package server

import (
	"net/http"
	"testing"
	"time"

	"bronzeboxing/internal/models"
)

// Regression coverage for the bulk safeguards: series counters, completed class
// versus attended package credit, and all-or-nothing bulk actions.
func TestEightClassSeriesCreditsNeedAttendanceAndBulkKeepsCounters(t *testing.T) {
	e := newEnv(t)
	tr := e.addTrainee("Probe", 0)
	var plan planOut
	e.ok("POST", "/trainees/"+tr.ID+"/session-plans", map[string]any{
		"title": "8 privates", "targetCount": 8, "startDate": "2026-01-01", "sessionType": "private",
	}, &plan)
	var created struct {
		SeriesID     string       `json:"seriesId"`
		PlannedCount int          `json:"plannedCount"`
		Sessions     []sessionOut `json:"sessions"`
	}
	e.ok("POST", "/sessions/recurring", map[string]any{
		"title": "PT probe", "type": "private", "weekdays": []int{1}, "time": "10:00", "durationMin": 60,
		"fromDay": "2027-01-04", "count": 8,
		"attendees": []map[string]any{{"trainee": tr.ID, "planId": plan.ID}},
	}, &created)
	if created.PlannedCount != 8 || len(created.Sessions) != 8 {
		t.Fatalf("created %d planned %d", len(created.Sessions), created.PlannedCount)
	}
	s := created.Sessions // s[0..7]

	// classes 1-3: completed AND attended -> credit
	for _, c := range s[:3] {
		e.mark(c.ID, tr.ID, "attended")
		e.setStatus(c.ID, models.SessCompleted)
	}
	// class 4: completed but trainee never marked -> class counts for the series, NOT as a package credit
	e.setStatus(s[3].ID, models.SessCompleted)

	pp := e.plan(tr.ID).Progress
	if pp.Completed != 3 || pp.AttendanceNeeded != 1 {
		t.Fatalf("credit must need attended: plan %+v", pp)
	}
	sp := e.seriesProg(created.SeriesID).Progress
	if sp.Completed != 4 || sp.Planned != 8 || sp.AttendanceNeeded != 1 {
		t.Fatalf("series after 4 completed: %+v", sp)
	}

	// bulk delete including a completed class is refused and nothing is deleted
	e.fail("POST", "/sessions/bulk/delete", map[string]any{"ids": []string{s[3].ID, s[5].ID}, "reason": "oops"}, http.StatusConflict, CodeConflict)
	var chk models.Session
	e.ok("GET", "/sessions/"+s[5].ID, nil, &chk)
	if chk.Status != models.SessScheduled {
		t.Fatalf("class 6 changed by refused bulk: %s", chk.Status)
	}

	// a SCHEDULED class with attendance already marked must be kept
	e.mark(s[4].ID, tr.ID, "attended")
	e.fail("POST", "/sessions/bulk/delete", map[string]any{"ids": []string{s[4].ID, s[5].ID}, "reason": "oops"}, http.StatusConflict, CodeConflict)
	e.fail("POST", "/sessions/series/"+created.SeriesID+"/delete", map[string]any{"scope": "all", "reason": "oops"}, http.StatusConflict, CodeConflict)
	if got := e.seriesProg(created.SeriesID).Progress; got.Created != 8 || got.Planned != 8 {
		t.Fatalf("refused deletes changed the series: %+v", got)
	}
	e.mark(s[4].ID, tr.ID, "booked")

	// scattered bulk delete of two scheduled classes lowers planned and created together
	e.ok("POST", "/sessions/bulk/delete", map[string]any{"ids": []string{s[5].ID, s[6].ID}, "reason": "holiday"}, nil)
	sp = e.seriesProg(created.SeriesID).Progress
	if sp.Planned != 6 || sp.Created != 6 || sp.Completed != 4 || sp.Scheduled != 2 {
		t.Fatalf("after bulk delete: %+v", sp)
	}
	// the plan still holds 4 credited-or-needed + upcoming; remaining counts only real credits
	pp = e.plan(tr.ID).Progress
	if pp.Completed != 3 || pp.Remaining != 5 || pp.UpcomingBooked != 2 || pp.AttendanceNeeded != 1 || pp.UnassignedSlots != 2 {
		t.Fatalf("plan after bulk delete: %+v", pp)
	}

	// delete the rest of the unfinished series; completed history stays
	var out struct {
		Deleted, Retained, Planned int
	}
	e.ok("POST", "/sessions/series/"+created.SeriesID+"/delete", map[string]any{"scope": "all", "reason": "stopped"}, &out)
	sp = e.seriesProg(created.SeriesID).Progress
	if sp.Planned != 4 || sp.Created != 4 || sp.Completed != 4 || sp.Scheduled != 0 {
		t.Fatalf("after series delete: %+v (resp %+v)", sp, out)
	}
	// extending later continues from the retained count
	e.ok("POST", "/sessions/series/"+created.SeriesID+"/extend", map[string]any{"toDay": "2027-02-08", "reason": "resume", "planOverride": true}, nil)
	sp = e.seriesProg(created.SeriesID).Progress
	if sp.Planned != sp.Created {
		t.Fatalf("planned and created drifted after extend: %+v", sp)
	}
}

func TestBulkGuardsAreAtomicAndNeedAReason(t *testing.T) {
	e := newEnv(t)
	a := e.addTrainee("A", 0)
	b := e.addTrainee("B", 0)
	mk := func(title string, day int, cap int, att ...string) sessionOut {
		var out sessionOut
		var at []map[string]any
		for _, id := range att {
			at = append(at, map[string]any{"trainee": id})
		}
		e.ok("POST", "/sessions", map[string]any{"title": title, "type": "group", "start": time.Date(2031, 3, day, 18, 0, 0, 0, time.Local), "durationMin": 60, "capacity": cap, "attendees": at}, &out)
		return out
	}
	full := mk("Full", 1, 1, a.ID)
	free := mk("Free", 2, 0)
	two := mk("Two", 3, 0, a.ID, b.ID)

	// waitlist history protects a class from bulk delete
	e.ok("POST", "/sessions/"+full.ID+"/waitlist", map[string]any{"trainee": b.ID}, nil)
	e.fail("POST", "/sessions/bulk/delete", map[string]any{"ids": []string{full.ID, free.ID}, "reason": "x"}, http.StatusConflict, CodeConflict)
	var chk models.Session
	e.ok("GET", "/sessions/"+free.ID, nil, &chk) // still there

	// capacity below booked count: refused, sibling unchanged
	e.fail("PATCH", "/sessions/bulk", map[string]any{"ids": []string{free.ID, two.ID}, "reason": "cap", "changes": map[string]any{"capacity": 1}}, http.StatusConflict, CodeCapacity)
	e.ok("GET", "/sessions/"+free.ID, nil, &chk)
	if chk.Capacity != 0 {
		t.Fatalf("sibling capacity changed despite refusal: %d", chk.Capacity)
	}

	// reason is mandatory, empty selection / duplicates / >200 are refused
	e.fail("POST", "/sessions/bulk/delete", map[string]any{"ids": []string{free.ID}, "reason": ""}, http.StatusBadRequest, "REASON_REQUIRED")
	e.fail("POST", "/sessions/bulk/delete", map[string]any{"ids": []string{}, "reason": "x"}, http.StatusBadRequest, CodeValidation)
	e.fail("POST", "/sessions/bulk/delete", map[string]any{"ids": []string{free.ID, free.ID}, "reason": "x"}, http.StatusBadRequest, CodeValidation)
	// cancelled classes are not bulk-actionable
	e.setStatus(two.ID, models.SessCancelled)
	e.fail("PATCH", "/sessions/bulk", map[string]any{"ids": []string{free.ID, two.ID}, "reason": "x", "changes": map[string]any{"location": "Z"}}, http.StatusConflict, CodeConflict)
}
