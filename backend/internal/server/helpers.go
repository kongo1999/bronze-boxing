package server

import (
	"context"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

// reqCtx returns a request-scoped context with a generous timeout for Mongo ops.
func reqCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// objID parses the :id route param as a Mongo ObjectID.
func objID(c *fiber.Ctx) (primitive.ObjectID, error) {
	id, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return primitive.NilObjectID, fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	return id, nil
}

// parseRange reads ?from=&to= (RFC3339 or YYYY-MM-DD); defaults to an open range.
func parseRange(c *fiber.Ctx) (time.Time, time.Time) {
	from := parseTime(c.Query("from"), time.Now().AddDate(-100, 0, 0))
	to := parseTime(c.Query("to"), time.Now().AddDate(100, 0, 0))
	return from, to
}

func parseTime(s string, def time.Time) time.Time {
	if s == "" {
		return def
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return def
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
