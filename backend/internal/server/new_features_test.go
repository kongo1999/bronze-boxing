package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"bronzeboxing/internal/config"
	"bronzeboxing/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

type alertRoundTripper func(*http.Request) (*http.Response, error)

func (f alertRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSeriesBulkDeleteProtectsMoneyAndKeepsProgress(t *testing.T) {
	e := newEnv(t)
	member := e.addTrainee("Series member", 0)
	var made struct {
		SeriesID string       `json:"seriesId"`
		Sessions []sessionOut `json:"sessions"`
	}
	e.ok("POST", "/sessions/recurring", map[string]any{
		"title": "Three PTs", "type": "private", "weekdays": []int{2}, "time": "08:00", "durationMin": 45,
		"fromDay": "2027-09-07", "toDay": "2027-09-21", "fee": 20,
		"attendees": []map[string]any{{"trainee": member.ID}},
	}, &made)
	if len(made.Sessions) != 3 {
		t.Fatalf("created %d classes", len(made.Sessions))
	}
	e.setStatus(made.Sessions[0].ID, models.SessCompleted)
	e.mark(made.Sessions[0].ID, member.ID, models.AttendAttended)
	e.ok("POST", "/payments", map[string]any{"trainee": member.ID, "type": "private", "amount": 20, "sessionId": made.Sessions[1].ID}, nil)
	var edited struct {
		Changed int `json:"changed"`
	}
	e.ok("PATCH", "/sessions/series/"+made.SeriesID, map[string]any{"scope": "future", "fromOccurrence": made.Sessions[1].ID, "reason": "new room and rate", "changes": map[string]any{"fee": 25, "durationMin": 55, "location": "Upstairs"}}, &edited)
	if edited.Changed != 2 {
		t.Fatalf("edited %d scheduled classes", edited.Changed)
	}
	var changed models.Session
	e.ok("GET", "/sessions/"+made.Sessions[1].ID, nil, &changed)
	if changed.Fee != 25 || changed.DurationMin != 55 || changed.Location != "Upstairs" {
		t.Fatalf("bulk edit: %+v", changed)
	}
	e.fail("POST", "/sessions/series/"+made.SeriesID+"/delete", map[string]any{"scope": "all", "reason": "entered by mistake"}, http.StatusConflict, CodeConflict)
	var result struct{ Deleted, Retained, Planned int }
	e.ok("POST", "/sessions/series/"+made.SeriesID+"/delete", map[string]any{"scope": "future", "fromOccurrence": made.Sessions[2].ID, "reason": "schedule changed"}, &result)
	if result.Deleted != 1 || result.Retained != 2 || result.Planned != 2 {
		t.Fatalf("delete result: %+v", result)
	}
	progress := e.seriesProg(made.SeriesID).Progress
	if progress.Completed != 1 || progress.Scheduled != 1 || progress.Planned != 2 {
		t.Fatalf("progress: %+v", progress)
	}
	var stats struct {
		StudioCompleted  int            `json:"studioCompleted"`
		TraineeCompleted map[string]int `json:"traineeCompleted"`
	}
	e.ok("GET", "/sessions/attendance-stats?trainees="+member.ID, nil, &stats)
	if stats.StudioCompleted != 1 || stats.TraineeCompleted[member.ID] != 1 {
		t.Fatalf("attendance stats: %+v", stats)
	}
}

func TestRulesVersionsAndMemberAcknowledgement(t *testing.T) {
	e := newEnv(t)
	member := e.addTrainee("Rules member", 0)
	var first models.StudioRules
	e.ok("PUT", "/studio-rules", map[string]any{"groupPolicy": "Arrive early", "privatePolicy": "Book ahead", "clientAgreement": "Agreed"}, &first)
	if first.Version != 1 {
		t.Fatalf("first version %d", first.Version)
	}
	var ack models.RulesAcknowledgement
	e.ok("POST", "/trainees/"+member.ID+"/rules-acknowledgements", map[string]any{"version": 1, "method": "signed_paper"}, &ack)
	e.fail("POST", "/trainees/"+member.ID+"/rules-acknowledgements", map[string]any{"version": 1, "method": "signed_paper"}, http.StatusConflict, CodeConflict)
	var second models.StudioRules
	e.ok("PUT", "/studio-rules", map[string]any{"groupPolicy": "Arrive ten minutes early", "privatePolicy": "Book ahead", "clientAgreement": "Agreed"}, &second)
	if second.Version != 2 {
		t.Fatalf("second version %d", second.Version)
	}
	e.fail("POST", "/trainees/"+member.ID+"/rules-acknowledgements", map[string]any{"version": 1, "method": "verbal"}, http.StatusBadRequest, CodeValidation)
	var old models.StudioRules
	e.ok("GET", "/studio-rules?version=1", nil, &old)
	if old.GroupPolicy != "Arrive early" {
		t.Fatalf("published wording changed: %q", old.GroupPolicy)
	}
}

func TestClassAlertsDraftQueueRequiresOptInForAutomation(t *testing.T) {
	e := newEnv(t)
	member := e.addTrainee("Reminder member", 0)
	e.ok("PUT", "/trainees/"+member.ID, map[string]any{"phone": "03 111 222", "whatsappOptIn": true}, nil)
	var s sessionOut
	e.ok("POST", "/sessions", map[string]any{"title": "Boxing tomorrow", "type": "group", "start": time.Now().Add(48 * time.Hour), "durationMin": 60, "attendees": []map[string]any{{"trainee": member.ID}}}, &s)
	var alerts []classAlert
	e.ok("GET", "/class-alerts", nil, &alerts)
	if len(alerts) != 1 || !alerts[0].OptedIn || alerts[0].Phone != "9613111222" || alerts[0].Status != "unsent" {
		t.Fatalf("draft queue: %+v", alerts)
	}
	e.fail("PUT", "/class-alert-settings", map[string]any{"mode": "automatic", "groupHours": 24, "privateHours": 24, "groupTemplate": "group_reminder", "privateTemplate": "private_reminder", "templateLanguage": "en"}, http.StatusBadRequest, CodeValidation)
	e.ok("POST", "/class-alerts/"+s.ID+"/"+member.ID+"/mark-sent", map[string]any{}, nil)
	e.ok("GET", "/class-alerts", nil, &alerts)
	if alerts[0].Status != "sent" {
		t.Fatalf("marked reminder: %+v", alerts[0])
	}
	e.fail("POST", "/class-alerts/"+s.ID+"/"+member.ID+"/mark-sent", map[string]any{}, http.StatusConflict, CodeConflict)
}

func TestAutomaticClassAlertSendsApprovedTemplateOnceToOptedInMember(t *testing.T) {
	e := newEnv(t)
	member := e.addTrainee("Auto member", 0)
	other := e.addTrainee("No consent", 0)
	e.ok("PUT", "/trainees/"+member.ID, map[string]any{"phone": "03 111 222", "whatsappOptIn": true}, nil)
	e.ok("PUT", "/trainees/"+other.ID, map[string]any{"phone": "03 222 333"}, nil)
	var s sessionOut
	e.ok("POST", "/sessions", map[string]any{"title": "Tomorrow group", "type": "group", "start": time.Now().Add(23 * time.Hour), "durationMin": 60, "attendees": []map[string]any{{"trainee": member.ID}, {"trainee": other.ID}}}, &s)
	_, err := e.store.Coll(models.CollAlertSettings).InsertOne(e.ctx, alertSettings{ID: "studio", Mode: "automatic", GroupHours: 24, PrivateHours: 24, GroupTemplate: "group_reminder", PrivateTemplate: "private_reminder", TemplateLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	h := &alertHandler{store: e.store, cfg: config.Config{WhatsAppToken: "test-token", WhatsAppPhoneID: "phone-id", WhatsAppAPIVersion: "v23.0"}, client: &http.Client{Transport: alertRoundTripper(func(req *http.Request) (*http.Response, error) {
		count++
		if req.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing bearer token")
		}
		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), `"name":"group_reminder"`) || !strings.Contains(string(body), `"to":"9613111222"`) || !strings.Contains(string(body), "Tomorrow group") {
			t.Errorf("wrong template: %s", body)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"messages":[{"id":"wamid.test"}]}`)), Header: http.Header{}}, nil
	})}}
	if err := h.sendDue(e.ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.sendDue(e.ctx); err != nil {
		t.Fatal(err)
	}
	if count != 1 || e.count(models.CollClassAlerts, bson.M{"status": "sent"}) != 1 {
		t.Fatalf("sent %d requests, deliveries %d", count, e.count(models.CollClassAlerts, bson.M{}))
	}
}
