package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

// auditLine is one change to record in the trail.
type auditLine struct {
	Entity string
	Ref    primitive.ObjectID
	Action string
	Before any
	After  any
	Actor  string
	Reason string
}

// writeAudit appends one line to the audit trail. Call it with the same ctx
// as the change it describes: inside WithTx that makes the line part of the
// transaction, so a change can never commit without its line, and a failed
// audit write rolls the change back instead of leaving a silent gap.
func writeAudit(ctx context.Context, store *db.Store, l auditLine) error {
	if _, err := store.Coll(models.CollAudit).InsertOne(ctx, models.AuditEntry{
		Entity: l.Entity,
		Ref:    l.Ref,
		Action: l.Action,
		Before: l.Before,
		After:  l.After,
		Actor:  l.Actor,
		Reason: l.Reason,
		At:     time.Now(),
	}); err != nil {
		return fmt.Errorf("audit %s %s: %w", l.Entity, l.Action, err)
	}
	return nil
}

// auditEntities are the record kinds the trail can be read for.
var auditEntities = map[string]bool{
	"payment": true, "expense": true, "sale": true, "charge": true,
	"item": true, "trainee": true, "series": true, "session": true, "plan": true,
}

// jsonable turns the driver's generic BSON values into shapes that marshal to
// ordinary JSON. Before/After are stored as `any`, so Mongo hands them back as
// primitive.D — which encoding/json would render as [{"Key":…,"Value":…}],
// useless to the UI. Documents become objects, ObjectIDs and dates become
// strings, and everything nested is converted the same way.
func jsonable(v any) any {
	switch t := v.(type) {
	case primitive.D:
		m := make(map[string]any, len(t))
		for _, e := range t {
			m[e.Key] = jsonable(e.Value)
		}
		return m
	case primitive.M:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[k] = jsonable(val)
		}
		return m
	case primitive.A:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = jsonable(e)
		}
		return out
	case primitive.ObjectID:
		return t.Hex()
	case primitive.DateTime:
		return t.Time().UTC()
	case primitive.Null:
		return nil
	default:
		return v
	}
}

// auditOut is one trail line as the UI sees it: before/after flattened to
// plain JSON objects.
type auditOut struct {
	ID     string    `json:"id"`
	Entity string    `json:"entity"`
	Ref    string    `json:"ref"`
	Action string    `json:"action"`
	Before any       `json:"before,omitempty"`
	After  any       `json:"after,omitempty"`
	Actor  string    `json:"actor,omitempty"`
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at"`
}

// registerAudit exposes the trail read-only: GET /api/audit/:entity/:id lists
// every recorded change for one record, newest first.
func registerAudit(r fiber.Router, store *db.Store) {
	r.Get("/audit/:entity/:id", func(c *fiber.Ctx) error {
		ctx, cancel := reqCtx()
		defer cancel()
		entity := c.Params("entity")
		if !auditEntities[entity] {
			return badField("entity", "unknown audit entity")
		}
		id, err := objID(c)
		if err != nil {
			return err
		}
		cur, err := store.Coll(models.CollAudit).Find(ctx,
			bson.M{"entity": entity, "ref": id},
			options.Find().SetSort(bson.D{{Key: "at", Value: -1}}))
		if err != nil {
			return err
		}
		entries := []models.AuditEntry{}
		if err := cur.All(ctx, &entries); err != nil {
			return err
		}
		out := make([]auditOut, len(entries))
		for i, e := range entries {
			out[i] = auditOut{
				ID:     e.ID.Hex(),
				Entity: e.Entity,
				Ref:    e.Ref.Hex(),
				Action: e.Action,
				Before: jsonable(e.Before),
				After:  jsonable(e.After),
				Actor:  e.Actor,
				Reason: e.Reason,
				At:     e.At,
			}
		}
		return c.JSON(out)
	})
}
