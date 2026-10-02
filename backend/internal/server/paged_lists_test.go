package server

import (
	"net/url"
	"testing"
	"time"

	"bronzeboxing/internal/models"
)

func TestPaymentsSummaryAndPagedSearch(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(time.Now())
	trainee := e.addTrainee("Karim Haddad", 100)
	var first, second paymentOut
	e.ok("POST", "/payments", map[string]any{"trainee": trainee.ID, "type": "subscription", "periodMonth": month, "amount": 30}, &first)
	e.ok("POST", "/payments", map[string]any{"trainee": trainee.ID, "type": "subscription", "periodMonth": month, "amount": 20}, &second)
	var summary struct {
		Count     int     `json:"count"`
		Collected float64 `json:"collected"`
	}
	e.ok("GET", "/payments/summary?m="+month, nil, &summary)
	if summary.Count != 2 || summary.Collected != 50 {
		t.Fatalf("summary = %+v", summary)
	}
	e.ok("POST", "/payments/"+second.ID+"/void", map[string]any{"reason": "Duplicate entry"}, nil)
	e.ok("GET", "/payments/summary?m="+month, nil, &summary)
	if summary.Count != 2 || summary.Collected != 30 {
		t.Fatalf("after void summary = %+v", summary)
	}
	var page struct {
		Items []paymentOut `json:"items"`
		Total int          `json:"total"`
	}
	e.ok("GET", "/payments?m="+month+"&q="+url.QueryEscape("Karim")+"&limit=1", nil, &page)
	if page.Total != 2 || len(page.Items) != 1 {
		t.Fatalf("payment page = %+v", page)
	}
}

func TestRosterInventoryAndReminderPagedSearch(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(time.Now())
	partial := e.addTrainee("Rami Khoury", 100)
	e.addTrainee("Karim Haddad", 100)
	e.ok("POST", "/payments", map[string]any{"trainee": partial.ID, "type": "subscription", "periodMonth": month, "amount": 40}, nil)
	var roster struct {
		Items []traineeOut `json:"items"`
		Total int          `json:"total"`
	}
	e.ok("GET", "/trainees?dues=partial&m="+month+"&limit=1", nil, &roster)
	if roster.Total != 1 || len(roster.Items) != 1 || roster.Items[0].ID != partial.ID {
		t.Fatalf("partial roster = %+v", roster)
	}
	e.ok("GET", "/trainees?dues=partial&m="+month+"&q=Rmai&limit=1", nil, &roster)
	if roster.Total != 1 || roster.Items[0].ID != partial.ID {
		t.Fatalf("fuzzy partial roster = %+v", roster)
	}

	e.ok("POST", "/inventory", map[string]any{"name": "Boxing Gloves", "stock": 2, "price": 40, "costPrice": 20, "lowStockThreshold": 3}, nil)
	var stock struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
		Total int `json:"total"`
	}
	e.ok("GET", "/inventory?active=true&stock=short&q=Glvoes&limit=10", nil, &stock)
	if stock.Total != 1 || len(stock.Items) != 1 {
		t.Fatalf("stock search = %+v", stock)
	}

	e.ok("POST", "/reminders", map[string]any{"title": "Order more gloves", "dueDay": models.DateKey(time.Now()), "priority": "high"}, nil)
	var reminders struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
		Total int `json:"total"`
	}
	e.ok("GET", "/reminders?status=open&priority=high&q=Glvoes&limit=10", nil, &reminders)
	if reminders.Total != 1 || len(reminders.Items) != 1 {
		t.Fatalf("reminder search = %+v", reminders)
	}
}

func TestMonthlyReportSeriesOpensAnOccurrence(t *testing.T) {
	e := newEnv(t)
	month := models.MonthKey(time.Now())
	first := month + "-01"
	day, err := models.ParseDay(first)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		SeriesID string `json:"seriesId"`
	}
	e.ok("POST", "/sessions/recurring", map[string]any{
		"title": "Reportable series", "type": "group", "weekdays": []int{int(day.Weekday())},
		"time": "06:00", "durationMin": 45, "fromDay": first, "toDay": first,
	}, &created)
	var report struct {
		Sessions struct {
			Series []struct {
				SeriesID     string `json:"seriesId"`
				OccurrenceID string `json:"occurrenceId"`
			} `json:"series"`
		} `json:"sessions"`
	}
	e.ok("GET", "/reports/monthly?m="+month, nil, &report)
	if len(report.Sessions.Series) != 1 || report.Sessions.Series[0].SeriesID != created.SeriesID || report.Sessions.Series[0].OccurrenceID == "" {
		t.Fatalf("report series = %+v", report.Sessions.Series)
	}
	e.ok("GET", "/sessions/"+report.Sessions.Series[0].OccurrenceID, nil, nil)
	var scheduled struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	e.ok("GET", "/sessions?status=scheduled&limit=10", nil, &scheduled)
	if scheduled.Total != 1 || scheduled.Items[0].ID != report.Sessions.Series[0].OccurrenceID {
		t.Fatalf("scheduled sessions = %+v", scheduled)
	}
}

func TestReminderPagingUsesSnoozedDayAcrossPages(t *testing.T) {
	e := newEnv(t)
	var snoozed struct {
		ID string `json:"id"`
	}
	e.ok("POST", "/reminders", map[string]any{"title": "Snoozed old task", "dueDay": models.DateKey(time.Now().Add(-24 * time.Hour)), "priority": "high"}, &snoozed)
	e.ok("POST", "/reminders/"+snoozed.ID+"/snooze", map[string]any{"days": 7}, nil)
	var today struct {
		ID string `json:"id"`
	}
	e.ok("POST", "/reminders", map[string]any{"title": "Do today", "dueDay": models.DateKey(time.Now()), "priority": "normal"}, &today)
	var first struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	e.ok("GET", "/reminders?status=open&limit=1", nil, &first)
	if first.Total != 2 || len(first.Items) != 1 || first.Items[0].ID != today.ID {
		t.Fatalf("first reminder page = %+v", first)
	}
}
