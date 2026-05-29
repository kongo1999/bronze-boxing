package server

import (
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

func registerSearch(r fiber.Router, store *db.Store) {
	r.Get("/search", func(c *fiber.Ctx) error {
		ctx, cancel := reqCtx()
		defer cancel()

		q := strings.TrimSpace(c.Query("q"))
		if q == "" {
			return c.JSON(fiber.Map{
				"trainees": []any{}, "sessions": []any{}, "payments": []any{}, "inventory": []any{},
			})
		}
		rx := bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}
		lim := options.Find().SetLimit(10)

		trainees := []models.Trainee{}
		if cur, err := store.Coll(models.CollTrainees).Find(ctx, bson.M{"name": rx}, lim); err == nil {
			_ = cur.All(ctx, &trainees)
		}
		sessions := []models.Session{}
		if cur, err := store.Coll(models.CollSessions).Find(ctx, bson.M{"title": rx},
			options.Find().SetLimit(10).SetSort(bson.D{{Key: "start", Value: -1}})); err == nil {
			_ = cur.All(ctx, &sessions)
		}
		payments := []models.Payment{}
		if cur, err := store.Coll(models.CollPayments).Find(ctx,
			bson.M{"$or": []bson.M{{"traineeName": rx}, {"note": rx}}},
			options.Find().SetLimit(10).SetSort(bson.D{{Key: "date", Value: -1}})); err == nil {
			_ = cur.All(ctx, &payments)
		}
		inventory := []models.InventoryItem{}
		if cur, err := store.Coll(models.CollInventory).Find(ctx, bson.M{"name": rx}, lim); err == nil {
			_ = cur.All(ctx, &inventory)
		}

		return c.JSON(fiber.Map{
			"trainees":  trainees,
			"sessions":  sessions,
			"payments":  payments,
			"inventory": inventory,
		})
	})
}
