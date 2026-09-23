package server

import (
	"testing"
	"time"

	"bronzeboxing/internal/models"
)

// Home's "needs attention": partial and unpaid counted apart, overdue
// reminders, items to restock, plans close to done, and the next class.
func TestDashboardNeedsAttention(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(nowFn())
	a := e.addTrainee("Adam", 50)
	e.addTrainee("Bilal", 50)
	e.pay(a.ID, 20, month)

	e.ok("POST", "/reminders", map[string]any{"title": "Fix the bag", "dueDay": models.DateKey(time.Now().AddDate(0, 0, -2)), "priority": "high"}, nil)
	e.ok("POST", "/inventory", map[string]any{"name": "Tape", "stock": 1, "price": 3, "lowStockThreshold": 2}, nil)
	e.ok("POST", "/inventory", map[string]any{"name": "Wraps", "stock": 0, "price": 8}, nil)
	e.ok("POST", "/inventory", map[string]any{"name": "Water", "stock": 40, "price": 1, "lowStockThreshold": 5}, nil)

	var plan planOut
	e.ok("POST", "/trainees/"+a.ID+"/session-plans", map[string]any{"title": "2 sessions", "targetCount": 2, "startDate": models.DateKey(time.Now())}, &plan)
	var s sessionOut
	e.ok("POST", "/sessions", map[string]any{"title": "Tomorrow", "type": "group", "start": time.Now().Add(24 * time.Hour), "durationMin": 60}, &s)

	var d struct {
		PartialCount     int `json:"partialCount"`
		UnpaidCount      int `json:"unpaidCount"`
		RemindersOverdue int `json:"remindersOverdue"`
		LowStock         int `json:"lowStock"`
		OutOfStock       int `json:"outOfStock"`
		PlansNearing     []struct {
			Title     string `json:"title"`
			Remaining int    `json:"remaining"`
		} `json:"plansNearing"`
		NextSession *struct {
			ID string `json:"id"`
		} `json:"nextSession"`
	}
	e.ok("GET", "/dashboard", nil, &d)
	if d.PartialCount != 1 || d.UnpaidCount != 1 || d.RemindersOverdue != 1 || d.LowStock != 1 || d.OutOfStock != 1 {
		t.Fatalf("dashboard = %+v", d)
	}
	if len(d.PlansNearing) != 1 || d.PlansNearing[0].Remaining != 2 {
		t.Fatalf("plans nearing = %+v", d.PlansNearing)
	}
	if d.NextSession == nil || d.NextSession.ID != s.ID {
		t.Fatalf("next session = %+v, want %s", d.NextSession, s.ID)
	}
}
