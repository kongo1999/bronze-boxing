package server

import (
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"bronzeboxing/internal/models"
)

func TestCoachPromiseTracksLaterPaymentsAndFollowUp(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(time.Now())
	tid := e.addTrainee("Promise trainee", 100).ID
	e.pay(tid, 20, month)
	dueDay := models.DateKey(clockNow().AddDate(0, 0, 1))
	var promise promiseView
	e.ok("POST", "/payment-promises", map[string]any{"trainee": tid, "periodMonth": month, "amount": 80, "dueDay": dueDay}, &promise)
	e.fail("POST", "/payment-promises", map[string]any{"trainee": tid, "periodMonth": month, "amount": 80, "dueDay": dueDay}, http.StatusConflict, CodeConflict)
	e.pay(tid, 30, month)
	var rows []promiseView
	e.ok("GET", "/payment-promises?m="+month, nil, &rows)
	if len(rows) != 1 || rows[0].State != "open" || rows[0].Outstanding != 50 {
		t.Fatalf("after part payment: %+v", rows)
	}
	e.pay(tid, 50, month)
	e.ok("GET", "/payment-promises?m="+month, nil, &rows)
	if rows[0].State != "fulfilled" || rows[0].Outstanding != 0 {
		t.Fatalf("after full payment: %+v", rows)
	}
}

func TestCoachWaitlistCancelOfferAndAccept(t *testing.T) {
	e := newEnv(t)
	clockNow = time.Now
	a := e.addTrainee("Booked", 0).ID
	b := e.addTrainee("Next in line", 0).ID
	start := time.Now().Add(48 * time.Hour)
	var s sessionOut
	e.ok("POST", "/sessions", map[string]any{"title": "Full class", "start": start, "durationMin": 60, "capacity": 1, "attendees": []map[string]any{{"trainee": a}}}, &s)
	var entry models.WaitlistEntry
	e.ok("POST", "/sessions/"+s.ID+"/waitlist", map[string]any{"trainee": b}, &entry)
	e.fail("POST", "/sessions/"+s.ID+"/waitlist", map[string]any{"trainee": b}, http.StatusConflict, CodeDuplicate)
	e.fail("DELETE", "/sessions/"+s.ID, nil, http.StatusConflict, CodeConflict)
	var cancelled struct {
		Session sessionOut `json:"session"`
		Late    bool       `json:"late"`
	}
	e.ok("POST", "/sessions/"+s.ID+"/cancel-booking", map[string]any{"trainee": a, "reason": "Away"}, &cancelled)
	if cancelled.Late || len(cancelled.Session.Attendees) != 0 {
		t.Fatalf("cancel: %+v", cancelled)
	}
	var queue []models.WaitlistEntry
	e.ok("GET", "/sessions/"+s.ID+"/waitlist", nil, &queue)
	if len(queue) != 1 || queue[0].Status != "offered" {
		t.Fatalf("waitlist: %+v", queue)
	}
	e.ok("POST", "/sessions/"+s.ID+"/waitlist/"+entry.ID.Hex()+"/accept", nil, &s)
	if len(s.Attendees) != 1 || s.Attendees[0].Trainee != b {
		t.Fatalf("accepted: %+v", s)
	}
	if e.count(models.CollCancellations, bson.M{"session": oid(t, s.ID)}) != 1 {
		t.Fatal("cancellation not recorded")
	}
}

func TestCoachCloseoutRequiresAllAttendanceAndLinksClassPayment(t *testing.T) {
	e := newEnv(t)
	a := e.addTrainee("Attended", 0).ID
	b := e.addTrainee("Absent", 0).ID
	var s sessionOut
	e.ok("POST", "/sessions", map[string]any{"title": "Drop in class", "start": time.Now().Add(-2 * time.Hour), "durationMin": 60, "fee": 15, "attendees": []map[string]any{{"trainee": a}, {"trainee": b}}}, &s)
	e.fail("POST", "/sessions/"+s.ID+"/closeout", map[string]any{"attendance": []map[string]any{{"trainee": a, "status": "attended"}}}, http.StatusBadRequest, CodeValidation)
	body := map[string]any{"attendance": []map[string]any{{"trainee": a, "status": "attended"}, {"trainee": b, "status": "no_show"}}, "note": "Work on footwork"}
	var closed struct {
		Status       string `json:"status"`
		CloseoutNote string `json:"closeoutNote"`
		Attendees    []struct {
			Status string `json:"status"`
		} `json:"attendees"`
	}
	e.ok("POST", "/sessions/"+s.ID+"/closeout", body, &closed)
	if closed.Status != "completed" || closed.CloseoutNote != "Work on footwork" || closed.Attendees[1].Status != "no_show" {
		t.Fatalf("closeout: %+v", closed)
	}
	var pay paymentOut
	e.ok("POST", "/payments", map[string]any{"trainee": a, "type": "dropin", "amount": 15, "sessionId": s.ID}, &pay)
	var linked []paymentOut
	e.ok("GET", "/payments?sessionId="+s.ID, nil, &linked)
	if len(linked) != 1 || linked[0].ID != pay.ID {
		t.Fatalf("class payments: %+v", linked)
	}
}

func TestCoachLinkedPaymentPreservesClassBooking(t *testing.T) {
	e := newEnv(t)
	tid := e.addTrainee("Paid drop in", 0).ID
	var s sessionOut
	e.ok("POST", "/sessions", map[string]any{
		"title": "Paid class", "start": time.Now().Add(48 * time.Hour), "durationMin": 60,
		"fee": 15, "attendees": []map[string]any{{"trainee": tid}},
	}, &s)
	e.ok("POST", "/payments", map[string]any{
		"trainee": tid, "type": "dropin", "amount": 15, "sessionId": s.ID,
	}, nil)
	e.fail("POST", "/sessions/"+s.ID+"/cancel-booking", map[string]any{"trainee": tid}, http.StatusConflict, CodeConflict)
	e.fail("PUT", "/sessions/"+s.ID, map[string]any{"attendees": []any{}}, http.StatusConflict, CodeConflict)
	e.fail("DELETE", "/sessions/"+s.ID, nil, http.StatusConflict, CodeConflict)
	var current sessionOut
	e.ok("GET", "/sessions/"+s.ID, nil, &current)
	if len(current.Attendees) != 1 || current.Attendees[0].Trainee != tid {
		t.Fatalf("paid booking was removed: %+v", current)
	}
}

func TestCoachTrialsProgressAndMonthReview(t *testing.T) {
	e := newEnv(t)
	var lead models.TrialLead
	e.ok("POST", "/trial-leads", map[string]any{"name": "Trial visitor", "followUpDay": models.DateKey(clockNow())}, &lead)
	e.fail("PATCH", "/trial-leads/"+lead.ID.Hex(), map[string]any{"status": "converted"}, http.StatusBadRequest, CodeValidation)
	var follow []followUp
	e.ok("GET", "/follow-ups", nil, &follow)
	found := false
	for _, f := range follow {
		if f.Kind == "trial" && f.Title == "Trial visitor" {
			found = true
		}
	}
	if !found {
		t.Fatalf("trial absent from queue: %+v", follow)
	}
	e.ok("POST", "/follow-ups/action", map[string]any{"key": "trial:" + lead.ID.Hex(), "action": "contacted", "days": 7}, nil)
	e.ok("GET", "/follow-ups", nil, &follow)
	for _, f := range follow {
		if f.Key == "trial:"+lead.ID.Hex() {
			t.Fatal("contacted trial remained in active queue")
		}
	}
	e.ok("GET", "/follow-ups?show=snoozed", nil, &follow)
	found = false
	for _, f := range follow {
		if f.Key == "trial:"+lead.ID.Hex() {
			found = true
		}
	}
	if !found {
		t.Fatal("contacted trial absent from snoozed queue")
	}
	e.ok("PATCH", "/trial-leads/"+lead.ID.Hex(), map[string]any{"status": "attended"}, &lead)
	if lead.TrialAttendedAt == nil {
		t.Fatal("trial attendance was not recorded")
	}
	tid := e.addTrainee("Trial visitor", 0).ID
	e.ok("PATCH", "/trial-leads/"+lead.ID.Hex(), map[string]any{"status": "converted", "trainee": tid}, &lead)
	if lead.Status != "converted" || lead.TrialAttendedAt == nil {
		t.Fatalf("conversion: %+v", lead)
	}
	var note models.ProgressNote
	e.ok("POST", "/trainees/"+tid+"/progress", map[string]any{"goal": "Defence", "nextFocus": "Slip and counter"}, &note)
	var notes []models.ProgressNote
	e.ok("GET", "/trainees/"+tid+"/progress", nil, &notes)
	if len(notes) != 1 || notes[0].NextFocus != "Slip and counter" {
		t.Fatalf("notes: %+v", notes)
	}
	month := models.MonthKey(time.Now())
	var review struct {
		Month  string              `json:"month"`
		Review *models.MonthReview `json:"review"`
	}
	e.ok("POST", "/month-review", map[string]any{"month": month, "note": "Checked"}, nil)
	e.ok("GET", "/month-review?m="+month, nil, &review)
	if review.Review == nil || review.Review.Note != "Checked" {
		t.Fatalf("review: %+v", review)
	}
}

func TestCoachLateCancellationRule(t *testing.T) {
	e := newEnv(t)
	clockNow = time.Now
	tid := e.addTrainee("Late cancellation", 0).ID
	e.ok("PUT", "/studio-policy", map[string]any{"cancelBeforeHours": 12, "lateCancellationAction": "no_show"}, nil)
	var future sessionOut
	e.ok("POST", "/sessions", map[string]any{"title": "Future class", "start": time.Now().Add(2 * time.Hour), "durationMin": 45, "attendees": []map[string]any{{"trainee": tid}}}, &future)
	var out struct {
		Session sessionOut `json:"session"`
		Late    bool       `json:"late"`
	}
	e.ok("POST", "/sessions/"+future.ID+"/cancel-booking", map[string]any{"trainee": tid}, &out)
	if !out.Late || len(out.Session.Attendees) != 0 {
		t.Fatalf("before class should release without premature no-show: %+v", out)
	}
	var past sessionOut
	e.ok("POST", "/sessions", map[string]any{"title": "Started class", "start": time.Now().Add(-time.Hour), "durationMin": 45, "attendees": []map[string]any{{"trainee": tid}}}, &past)
	e.ok("POST", "/sessions/"+past.ID+"/cancel-booking", map[string]any{"trainee": tid}, &out)
	if !out.Late || len(out.Session.Attendees) != 1 || out.Session.Attendees[0].Status != models.AttendNoShow {
		t.Fatalf("started class should record no-show: %+v", out)
	}
}
