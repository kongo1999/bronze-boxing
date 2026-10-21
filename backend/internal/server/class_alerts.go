package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/config"
	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

type alertSettings struct {
	ID               string    `bson:"_id" json:"-"`
	Mode             string    `bson:"mode" json:"mode"`
	GroupHours       int       `bson:"groupHours" json:"groupHours"`
	PrivateHours     int       `bson:"privateHours" json:"privateHours"`
	GroupTemplate    string    `bson:"groupTemplate" json:"groupTemplate"`
	PrivateTemplate  string    `bson:"privateTemplate" json:"privateTemplate"`
	TemplateLanguage string    `bson:"templateLanguage" json:"templateLanguage"`
	UpdatedAt        time.Time `bson:"updatedAt,omitempty" json:"updatedAt,omitempty"`
	UpdatedBy        string    `bson:"updatedBy,omitempty" json:"updatedBy,omitempty"`
}

type classAlert struct {
	SessionID string     `json:"sessionId"`
	TraineeID string     `json:"traineeId"`
	Name      string     `json:"name"`
	Phone     string     `json:"phone"`
	OptedIn   bool       `json:"optedIn"`
	Title     string     `json:"title"`
	Type      string     `json:"type"`
	Start     time.Time  `json:"start"`
	DueAt     time.Time  `json:"dueAt"`
	Status    string     `json:"status"`
	SentAt    *time.Time `json:"sentAt,omitempty"`
	Error     string     `json:"error,omitempty"`
	Text      string     `json:"text"`
}

type alertDelivery struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Session   primitive.ObjectID `bson:"session"`
	Trainee   primitive.ObjectID `bson:"trainee"`
	Status    string             `bson:"status"`
	Channel   string             `bson:"channel"`
	SentAt    *time.Time         `bson:"sentAt,omitempty"`
	Error     string             `bson:"error,omitempty"`
	CreatedAt time.Time          `bson:"createdAt"`
}

type alertHandler struct {
	store  *db.Store
	cfg    config.Config
	client *http.Client
}

func registerClassAlerts(r fiber.Router, store *db.Store, cfg config.Config) {
	h := &alertHandler{store: store, cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}}
	r.Get("/class-alert-settings", h.getSettings)
	r.Put("/class-alert-settings", h.putSettings)
	r.Get("/class-alerts", h.list)
	r.Post("/class-alerts/:session/:trainee/mark-sent", h.markSent)
}

func defaultAlertSettings() alertSettings {
	return alertSettings{ID: "studio", Mode: "drafts", GroupHours: 24, PrivateHours: 24, TemplateLanguage: "en"}
}

func (h *alertHandler) settings(ctx context.Context) (alertSettings, error) {
	s := defaultAlertSettings()
	err := h.store.Coll(models.CollAlertSettings).FindOne(ctx, bson.M{"_id": "studio"}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return defaultAlertSettings(), nil
	}
	return s, err
}

func (h *alertHandler) providerReady() bool {
	return h.cfg.WhatsAppToken != "" && h.cfg.WhatsAppPhoneID != ""
}

func (h *alertHandler) getSettings(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	s, err := h.settings(ctx)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"settings": s, "providerReady": h.providerReady()})
}

var templateNameRE = regexp.MustCompile(`^[a-z0-9_]{0,512}$`)
var languageRE = regexp.MustCompile(`^[a-z]{2,3}(?:_[A-Z]{2})?$`)

func (h *alertHandler) putSettings(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	s := defaultAlertSettings()
	if err := c.BodyParser(&s); err != nil {
		return badField("body", "invalid settings")
	}
	s.ID = "studio"
	s.GroupTemplate = strings.TrimSpace(s.GroupTemplate)
	s.PrivateTemplate = strings.TrimSpace(s.PrivateTemplate)
	if s.Mode != "drafts" && s.Mode != "automatic" {
		return badField("mode", "choose drafts or automatic")
	}
	if s.GroupHours < 1 || s.GroupHours > 168 || s.PrivateHours < 1 || s.PrivateHours > 168 {
		return badField("hours", "choose 1 to 168 hours")
	}
	if !templateNameRE.MatchString(s.GroupTemplate) || !templateNameRE.MatchString(s.PrivateTemplate) {
		return badField("template", "use a lowercase approved template name")
	}
	if !languageRE.MatchString(s.TemplateLanguage) {
		return badField("templateLanguage", "enter a language code such as en or ar")
	}
	if s.Mode == "automatic" && (!h.providerReady() || s.GroupTemplate == "" || s.PrivateTemplate == "") {
		return badField("mode", "automatic reminders need server credentials and both approved template names")
	}
	s.UpdatedAt, s.UpdatedBy = time.Now(), actorOf(c)
	_, err := h.store.Coll(models.CollAlertSettings).ReplaceOne(ctx, bson.M{"_id": "studio"}, s, options.Replace().SetUpsert(true))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"settings": s, "providerReady": h.providerReady()})
}

func alertPhone(raw string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, raw)
	if strings.HasPrefix(strings.TrimSpace(raw), "+") {
		if len(digits) >= 8 && len(digits) <= 15 {
			return digits
		}
		return ""
	}
	if strings.HasPrefix(digits, "00") {
		digits = strings.TrimPrefix(digits, "00")
	} else if strings.HasPrefix(digits, "0") {
		digits = "961" + strings.TrimLeft(digits, "0")
	} else if len(digits) == 8 {
		digits = "961" + digits
	} else if !strings.HasPrefix(digits, "961") {
		return "" // never guess another country's calling code
	}
	if len(digits) < 8 || len(digits) > 15 {
		return ""
	}
	return digits
}

func (h *alertHandler) candidates(ctx context.Context, from, until time.Time, s alertSettings) ([]classAlert, error) {
	cur, err := h.store.Coll(models.CollSessions).Find(ctx, bson.M{"status": models.SessScheduled, "start": bson.M{"$gte": from, "$lt": until}}, options.Find().SetSort(bson.D{{Key: "start", Value: 1}}).SetLimit(1000))
	if err != nil {
		return nil, err
	}
	var sessions []models.Session
	if err := cur.All(ctx, &sessions); err != nil {
		return nil, err
	}
	ids := []primitive.ObjectID{}
	seen := map[primitive.ObjectID]bool{}
	for _, se := range sessions {
		for _, a := range se.Attendees {
			if a.Status == models.AttendBooked && !seen[a.Trainee] {
				ids = append(ids, a.Trainee)
				seen[a.Trainee] = true
			}
		}
	}
	trainees := map[primitive.ObjectID]models.Trainee{}
	if len(ids) > 0 {
		rows, err := h.store.Coll(models.CollTrainees).Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
		if err != nil {
			return nil, err
		}
		var all []models.Trainee
		if err := rows.All(ctx, &all); err != nil {
			return nil, err
		}
		for _, t := range all {
			trainees[t.ID] = t
		}
	}
	deliveries := map[string]alertDelivery{}
	if len(sessions) > 0 {
		sessionIDs := make([]primitive.ObjectID, len(sessions))
		for i, se := range sessions {
			sessionIDs[i] = se.ID
		}
		rows, err := h.store.Coll(models.CollClassAlerts).Find(ctx, bson.M{"session": bson.M{"$in": sessionIDs}})
		if err != nil {
			return nil, err
		}
		var all []alertDelivery
		if err := rows.All(ctx, &all); err != nil {
			return nil, err
		}
		for _, d := range all {
			deliveries[d.Session.Hex()+":"+d.Trainee.Hex()] = d
		}
	}
	alerts := []classAlert{}
	for _, se := range sessions {
		hours := s.GroupHours
		if se.Type == models.SessionPrivate {
			hours = s.PrivateHours
		}
		for _, a := range se.Attendees {
			if a.Status != models.AttendBooked {
				continue
			}
			t, exists := trainees[a.Trainee]
			if !exists {
				continue
			}
			phone := alertPhone(t.Phone)
			local := se.Start.In(time.Local)
			d := deliveries[se.ID.Hex()+":"+t.ID.Hex()]
			status := d.Status
			if status == "" {
				status = "unsent"
			}
			if phone == "" && status == "unsent" {
				status = "needs_phone"
			}
			alerts = append(alerts, classAlert{
				SessionID: se.ID.Hex(), TraineeID: t.ID.Hex(), Name: t.Name, Phone: phone, OptedIn: t.WhatsAppOptInAt != nil,
				Title: se.Title, Type: se.Type, Start: se.Start, DueAt: se.Start.Add(-time.Duration(hours) * time.Hour),
				Status: status, SentAt: d.SentAt, Error: d.Error,
				Text: fmt.Sprintf("Hi %s, reminder: %s is on %s at %s. See you at Bronze Boxing!", t.Name, se.Title, local.Format("Monday 2 January 2006"), local.Format("3:04 PM")),
			})
		}
	}
	return alerts, nil
}

func (h *alertHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	days := 14
	if raw := c.Query("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 31 {
			return badField("days", "choose 1 to 31 days")
		}
		days = n
	}
	s, err := h.settings(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	alerts, err := h.candidates(ctx, now, now.AddDate(0, 0, days), s)
	if err != nil {
		return err
	}
	return c.JSON(alerts)
}

func (h *alertHandler) markSent(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	seID, err := parseOID(c.Params("session"), "session")
	if err != nil {
		return err
	}
	tID, err := parseOID(c.Params("trainee"), "trainee")
	if err != nil {
		return err
	}
	var se models.Session
	if err := h.store.Coll(models.CollSessions).FindOne(ctx, bson.M{"_id": seID, "status": models.SessScheduled, "start": bson.M{"$gt": time.Now()}}).Decode(&se); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return notFound("upcoming class")
		}
		return err
	}
	booked := false
	for _, a := range se.Attendees {
		if a.Trainee == tID && a.Status == models.AttendBooked {
			booked = true
			break
		}
	}
	if !booked {
		return badField("trainee", "member is not booked in this class")
	}
	if _, err := loadTrainee(ctx, h.store, tID.Hex(), "trainee"); err != nil {
		return err
	}
	now := time.Now()
	d := alertDelivery{ID: primitive.NewObjectID(), Session: seID, Trainee: tID, Status: "sent", Channel: "manual", SentAt: &now, CreatedAt: now}
	if _, err := h.store.Coll(models.CollClassAlerts).InsertOne(ctx, d); err != nil {
		if isDup(err) {
			result, updateErr := h.store.Coll(models.CollClassAlerts).UpdateOne(ctx, bson.M{"session": seID, "trainee": tID, "status": bson.M{"$in": []string{"failed", "sending"}}}, bson.M{"$set": bson.M{"status": "sent", "channel": "manual", "sentAt": now, "error": ""}})
			if updateErr != nil {
				return updateErr
			}
			if result.ModifiedCount == 1 {
				return c.JSON(fiber.Map{"status": "sent", "sentAt": now})
			}
			return apiErr(http.StatusConflict, CodeConflict, "a reminder was already recorded for this member and class")
		}
		return err
	}
	return c.JSON(fiber.Map{"status": "sent", "sentAt": now})
}

func (h *alertHandler) sendTemplate(ctx context.Context, a classAlert, s alertSettings) error {
	template := s.GroupTemplate
	if a.Type == models.SessionPrivate {
		template = s.PrivateTemplate
	}
	local := a.Start.In(time.Local)
	vars := []string{a.Name, a.Title, local.Format("Monday 2 January 2006"), local.Format("3:04 PM")}
	params := make([]map[string]string, len(vars))
	for i, v := range vars {
		params[i] = map[string]string{"type": "text", "text": v}
	}
	body, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp", "to": a.Phone, "type": "template",
		"template": map[string]any{"name": template, "language": map[string]string{"code": s.TemplateLanguage}, "components": []any{map[string]any{"type": "body", "parameters": params}}},
	})
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/messages", h.cfg.WhatsAppAPIVersion, h.cfg.WhatsAppPhoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.cfg.WhatsAppToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("WhatsApp API returned HTTP %d", resp.StatusCode)
	}
	var accepted struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		return fmt.Errorf("WhatsApp API response could not be confirmed: %w", err)
	}
	if len(accepted.Messages) == 0 || accepted.Messages[0].ID == "" {
		return errors.New("WhatsApp API did not confirm a message ID")
	}
	return nil
}

func (h *alertHandler) sendDue(ctx context.Context) error {
	s, err := h.settings(ctx)
	if err != nil {
		return err
	}
	if s.Mode != "automatic" || !h.providerReady() {
		return nil
	}
	now := time.Now()
	hours := s.GroupHours
	if s.PrivateHours > hours {
		hours = s.PrivateHours
	}
	alerts, err := h.candidates(ctx, now, now.Add(time.Duration(hours)*time.Hour+time.Minute), s)
	if err != nil {
		return err
	}
	for _, a := range alerts {
		if !a.OptedIn || a.Status != "unsent" || a.DueAt.After(now) || !a.Start.After(now) {
			continue
		}
		seID, _ := primitive.ObjectIDFromHex(a.SessionID)
		tID, _ := primitive.ObjectIDFromHex(a.TraineeID)
		d := alertDelivery{ID: primitive.NewObjectID(), Session: seID, Trainee: tID, Status: "sending", Channel: "automatic", CreatedAt: time.Now()}
		if _, err := h.store.Coll(models.CollClassAlerts).InsertOne(ctx, d); err != nil {
			if isDup(err) {
				continue
			}
			return err
		}
		status, msg := "sent", ""
		if err := h.sendTemplate(ctx, a, s); err != nil {
			status, msg = "failed", err.Error()
			log.Printf("class reminder %s/%s failed: %s", a.SessionID, a.TraineeID, msg)
		}
		set := bson.M{"status": status, "error": msg}
		if status == "sent" {
			set["sentAt"] = time.Now()
		}
		if _, err := h.store.Coll(models.CollClassAlerts).UpdateOne(ctx, bson.M{"_id": d.ID, "status": "sending"}, bson.M{"$set": set}); err != nil {
			return err
		}
	}
	return nil
}

// StartClassAlertWorker checks once on startup and then every minute. Unique
// deliveries keep restarts from sending the same class reminder twice.
func StartClassAlertWorker(ctx context.Context, store *db.Store, cfg config.Config) {
	h := &alertHandler{store: store, cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}}
	run := func() {
		work, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		if err := h.sendDue(work); err != nil && ctx.Err() == nil {
			log.Printf("class reminder scan failed: %v", err)
		}
	}
	run()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
