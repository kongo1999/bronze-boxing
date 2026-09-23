package server

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

// reqCtx returns a request-scoped context with a generous timeout for Mongo ops.
func reqCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// objID parses the :id route param as a Mongo ObjectID.
func objID(c *fiber.Ctx) (primitive.ObjectID, error) {
	return parseOID(c.Params("id"), "id")
}

func parseOID(hex, field string) (primitive.ObjectID, error) {
	id, err := primitive.ObjectIDFromHex(hex)
	if err != nil {
		return primitive.NilObjectID, &APIError{Status: http.StatusBadRequest, Code: CodeInvalidID, Message: "invalid " + field, Field: field}
	}
	return id, nil
}

// parseRange reads ?from=&to= as a half-open [from, to) window. Each bound is
// RFC3339 (an exact instant) or YYYY-MM-DD (the start of that studio-local
// day, so to=2026-09-28 excludes the 28th). Missing bounds are open. A
// malformed bound is rejected rather than silently widened.
func parseRange(c *fiber.Ctx) (time.Time, time.Time, error) {
	from, err := parseBound(c.Query("from"), time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC), "from")
	if err != nil {
		return from, from, err
	}
	to, err := parseBound(c.Query("to"), time.Date(2200, 1, 1, 0, 0, 0, 0, time.UTC), "to")
	if err != nil {
		return from, to, err
	}
	if to.Before(from) {
		return from, to, badField("to", "to must not be before from")
	}
	return from, to, nil
}

func parseBound(s string, def time.Time, field string) (time.Time, error) {
	if s == "" {
		return def, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := models.ParseDay(s); err == nil {
		return t, nil
	}
	return def, badField(field, field+" must be YYYY-MM-DD or an RFC3339 time")
}

// monthOrRange resolves ?m=YYYY-MM or ?from=&to= into a [start,end) window.
func monthOrRange(c *fiber.Ctx) (time.Time, time.Time, error) {
	if m := c.Query("m"); m != "" {
		start, end, err := models.MonthRange(m)
		if err != nil {
			return start, end, badField("m", "m must be YYYY-MM")
		}
		return start, end, nil
	}
	return parseRange(c)
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// round2 normalizes money to 2 decimal places at every write boundary, so
// float drift can never accumulate into the books.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// notVoided narrows a money query to live (non-voided) records. In Mongo,
// {field: nil} matches both missing and null, so legacy documents written
// before voiding existed are included.
func notVoided(filter bson.M) bson.M {
	filter["voidedAt"] = nil
	return filter
}

// deref helpers for optional (pointer) JSON fields — nil means "not sent".
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefF64(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// actorOf names who is making a request: the signed-in username, or "local"
// when the API runs in open (no-login) mode.
func actorOf(c *fiber.Ctx) string {
	if s, ok := c.Locals("session").(models.AuthSession); ok && s.Username != "" {
		return s.Username
	}
	return "local"
}

// oneOf validates an enum-ish string field.
func oneOf(field, v string, allowed ...string) error {
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return badField(field, field+" must be one of: "+strings.Join(allowed, ", "))
}

// requireReason returns the trimmed reason, or a REASON_REQUIRED error.
// Voids and material corrections of money or stock always carry one.
func requireReason(reason, what string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", &APIError{Status: http.StatusBadRequest, Code: CodeReasonRequired,
			Message: "a reason is required to " + what, Field: "reason"}
	}
	if len(reason) > 500 {
		return "", badField("reason", "reason must be at most 500 characters")
	}
	return reason, nil
}

// validMoney rejects NaN/Inf and absurd magnitudes before rounding.
func validMoney(field string, v float64, allowZero bool) (float64, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1e9 {
		return 0, badField(field, field+" must be a non-negative amount")
	}
	v = round2(v)
	if !allowZero && v <= 0 {
		return 0, badField(field, field+" must be positive")
	}
	return v, nil
}

// sanityTime rejects instants far outside the app's plausible range (a
// mistyped year would otherwise land a payment in 1926 or 2226).
func sanityTime(field string, t time.Time) error {
	if t.Year() < 2000 || t.Year() > 2100 {
		return badField(field, field+" is out of range")
	}
	return nil
}

// loadTrainee resolves a trainee id, failing with TRAINEE_NOT_FOUND when the
// id is malformed or no longer exists, so no record links to a ghost.
func loadTrainee(ctx context.Context, store *db.Store, hex, field string) (models.Trainee, error) {
	var t models.Trainee
	id, err := primitive.ObjectIDFromHex(hex)
	if err != nil {
		return t, &APIError{Status: http.StatusBadRequest, Code: CodeTraineeNotFound, Message: "unknown trainee", Field: field}
	}
	if err := store.Coll(models.CollTrainees).FindOne(ctx, bson.M{"_id": id}).Decode(&t); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return t, &APIError{Status: http.StatusBadRequest, Code: CodeTraineeNotFound, Message: "that trainee no longer exists", Field: field}
		}
		return t, err
	}
	return t, nil
}

// traineeNames batch-resolves trainee display names for a set of ids.
func traineeNames(ctx context.Context, store *db.Store, ids []primitive.ObjectID) map[primitive.ObjectID]string {
	out := map[primitive.ObjectID]string{}
	if len(ids) == 0 {
		return out
	}
	cur, err := store.Coll(models.CollTrainees).Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return out
	}
	var ts []models.Trainee
	_ = cur.All(ctx, &ts)
	for _, t := range ts {
		out[t.ID] = t.Name
	}
	return out
}

// isDup reports a unique-index violation.
func isDup(err error) bool {
	return mongo.IsDuplicateKeyError(err)
}
