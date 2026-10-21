package server

import (
	"net/http"
	"testing"
	"time"

	"bronzeboxing/internal/models"
)

func TestBulkClassesMixedSelectionIsAtomicAndUpdatesSeriesCount(t *testing.T) {
	e := newEnv(t)
	member := e.addTrainee("Bulk student", 0)
	var series struct {
		SeriesID string       `json:"seriesId"`
		Sessions []sessionOut `json:"sessions"`
	}
	e.ok("POST", "/sessions/recurring", map[string]any{
		"title": "Bulk PT", "type": "private", "weekdays": []int{2}, "time": "08:00", "durationMin": 45,
		"fromDay": "2027-09-07", "count": 3, "fee": 20,
		"attendees": []map[string]any{{"trainee": member.ID}},
	}, &series)
	if len(series.Sessions) != 3 {
		t.Fatalf("created %d", len(series.Sessions))
	}
	var solo sessionOut
	e.ok("POST", "/sessions", map[string]any{
		"title": "Standalone", "type": "group", "start": time.Date(2027, 9, 9, 11, 0, 0, 0, time.Local), "durationMin": 60,
	}, &solo)
	ids := []string{solo.ID, series.Sessions[0].ID}
	var result struct {
		Changed int `json:"changed"`
	}
	e.ok("PATCH", "/sessions/bulk", map[string]any{"ids": ids, "reason": "Move upstairs", "changes": map[string]any{"location": "Upstairs", "durationMin": 50}}, &result)
	if result.Changed != 2 {
		t.Fatalf("changed %d", result.Changed)
	}
	for _, id := range ids {
		var s models.Session
		e.ok("GET", "/sessions/"+id, nil, &s)
		if s.Location != "Upstairs" || s.DurationMin != 50 {
			t.Fatalf("class %s: %+v", id, s)
		}
	}
	var untouched models.Session
	e.ok("GET", "/sessions/"+series.Sessions[1].ID, nil, &untouched)
	if untouched.Location == "Upstairs" {
		t.Fatal("unselected series occurrence changed")
	}

	e.ok("POST", "/payments", map[string]any{"trainee": member.ID, "type": "private", "amount": 20, "sessionId": series.Sessions[1].ID}, nil)
	e.fail("PATCH", "/sessions/bulk", map[string]any{"ids": []string{solo.ID, series.Sessions[1].ID}, "reason": "Change price", "changes": map[string]any{"fee": 30}}, http.StatusConflict, CodeConflict)
	var check models.Session
	e.ok("GET", "/sessions/"+solo.ID, nil, &check)
	if check.Fee != 0 {
		t.Fatalf("partial fee edit persisted: %v", check.Fee)
	}
	e.fail("POST", "/sessions/bulk/delete", map[string]any{"ids": []string{solo.ID, series.Sessions[1].ID}, "reason": "Wrong classes"}, http.StatusConflict, CodeConflict)
	e.ok("GET", "/sessions/"+solo.ID, nil, &check)

	var deleted struct {
		Deleted int `json:"deleted"`
	}
	e.ok("POST", "/sessions/bulk/delete", map[string]any{"ids": ids, "reason": "Wrong classes"}, &deleted)
	if deleted.Deleted != 2 {
		t.Fatalf("deleted %d", deleted.Deleted)
	}
	if p := e.seriesProg(series.SeriesID).Progress; p.Planned != 2 || p.Created != 2 {
		t.Fatalf("series after mixed delete: %+v", p)
	}
}

func TestBulkTimeEditRejectsClashesWithinSelection(t *testing.T) {
	e := newEnv(t)
	makeClass := func(hour int) sessionOut {
		var s sessionOut
		e.ok("POST", "/sessions", map[string]any{
			"title": "Group", "type": "group", "start": time.Date(2027, 7, 8, hour, 0, 0, 0, time.Local), "durationMin": 60,
		}, &s)
		return s
	}
	a, b := makeClass(9), makeClass(11)
	e.fail("PATCH", "/sessions/bulk", map[string]any{"ids": []string{a.ID, b.ID}, "reason": "Wrong time", "changes": map[string]any{"time": "10:00"}}, http.StatusConflict, CodeScheduleClash)
	var actual models.Session
	e.ok("GET", "/sessions/"+a.ID, nil, &actual)
	if actual.Start.In(time.Local).Hour() != 9 {
		t.Fatalf("first class changed despite clash: %v", actual.Start)
	}
}
