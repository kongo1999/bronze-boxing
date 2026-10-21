package server

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"bronzeboxing/internal/models"
)

// attendanceStats gives the schedule a stable all-time studio total and
// attended counts for only the trainees visible in the requested week.
// A student credit needs both a completed class and marked attendance.
func (h *sessionHandler) attendanceStats(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	ids := []primitive.ObjectID{}
	seen := map[primitive.ObjectID]bool{}
	for _, raw := range strings.Split(c.Query("trainees"), ",") {
		if raw == "" {
			continue
		}
		id, err := parseOID(raw, "trainees")
		if err != nil {
			return err
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > 100 {
		return apiErr(http.StatusBadRequest, CodeValidation, "at most 100 trainees at a time").withField("trainees")
	}
	total, err := h.store.Coll(models.CollSessions).CountDocuments(ctx, bson.M{"status": models.SessCompleted})
	if err != nil {
		return err
	}
	counts := map[string]int{}
	if len(ids) > 0 {
		cur, err := h.store.Coll(models.CollSessions).Aggregate(ctx, mongo.Pipeline{
			{{Key: "$match", Value: bson.M{"status": models.SessCompleted, "attendees.trainee": bson.M{"$in": ids}}}},
			{{Key: "$unwind", Value: "$attendees"}},
			{{Key: "$match", Value: bson.M{"attendees.trainee": bson.M{"$in": ids}, "attendees.status": models.AttendAttended}}},
			{{Key: "$group", Value: bson.M{"_id": "$attendees.trainee", "count": bson.M{"$sum": 1}}}},
		})
		if err != nil {
			return err
		}
		var rows []struct {
			ID    primitive.ObjectID `bson:"_id"`
			Count int                `bson:"count"`
		}
		if err := cur.All(ctx, &rows); err != nil {
			return err
		}
		for _, row := range rows {
			counts[row.ID.Hex()] = row.Count
		}
	}
	return c.JSON(fiber.Map{"studioCompleted": total, "traineeCompleted": counts})
}
