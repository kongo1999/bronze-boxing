package server

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

// writeAudit appends one line to the money audit trail. Best-effort by design:
// an audit hiccup must never fail the user's actual operation, but it is
// logged loudly so it can't go quietly missing.
func writeAudit(ctx context.Context, store *db.Store, entity string, ref primitive.ObjectID, action string, before, after any) {
	_, err := store.Coll(models.CollAudit).InsertOne(ctx, models.AuditEntry{
		Entity: entity,
		Ref:    ref,
		Action: action,
		Before: before,
		After:  after,
		At:     time.Now(),
	})
	if err != nil {
		log.Printf("AUDIT WRITE FAILED (%s %s %s): %v", entity, action, ref.Hex(), err)
	}
}

// registerAudit exposes the trail read-only: GET /api/audit/:entity/:id lists
// every recorded change for one payment, expense, or sale, newest first.
func registerAudit(r fiber.Router, store *db.Store) {
	r.Get("/audit/:entity/:id", func(c *fiber.Ctx) error {
		ctx, cancel := reqCtx()
		defer cancel()
		entity := c.Params("entity")
		switch entity {
		case "payment", "expense", "sale":
		default:
			return fiber.NewError(fiber.StatusBadRequest, "entity must be payment, expense or sale")
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
		out := []models.AuditEntry{}
		if err := cur.All(ctx, &out); err != nil {
			return err
		}
		return c.JSON(out)
	})
}
