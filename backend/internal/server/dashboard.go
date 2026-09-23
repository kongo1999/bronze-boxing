package server

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

func registerDashboard(r fiber.Router, store *db.Store) {
	r.Get("/dashboard", func(c *fiber.Ctx) error {
		ctx, cancel := reqCtx()
		defer cancel()

		now := time.Now()
		month := models.MonthKey(now)
		startDay := models.StartOfDay(now)
		endDay := startDay.AddDate(0, 0, 1)
		weekEnd := startDay.AddDate(0, 0, 7)

		// Today's sessions.
		scur, err := store.Coll(models.CollSessions).Find(ctx,
			bson.M{"start": bson.M{"$gte": startDay, "$lt": endDay}},
			options.Find().SetSort(bson.D{{Key: "start", Value: 1}}))
		if err != nil {
			return err
		}
		todaySessions := []models.Session{}
		if err := scur.All(ctx, &todaySessions); err != nil {
			return err
		}

		// This week's reminders.
		rcur, err := store.Coll(models.CollReminders).Find(ctx,
			bson.M{"dueDate": bson.M{"$gte": startDay, "$lt": weekEnd}},
			options.Find().SetSort(bson.D{{Key: "dueDate", Value: 1}}))
		if err != nil {
			return err
		}
		weekReminders := []models.Reminder{}
		if err := rcur.All(ctx, &weekReminders); err != nil {
			return err
		}

		monthRevenue, err := computeMonthRevenue(ctx, store, month)
		if err != nil {
			return err
		}
		activeTrainees, err := store.Coll(models.CollTrainees).CountDocuments(ctx, bson.M{"status": models.StatusActive})
		if err != nil {
			return err
		}
		dues, err := listDues(ctx, store, month)
		if err != nil {
			return err
		}
		// Partial and unpaid are counted separately — a partial account is
		// neither paid nor untouched. Unverified rows stay out of both.
		overdue := []subRow{}
		var partial, unpaid int
		var outstanding float64
		for _, s := range dues {
			switch s.State {
			case models.ChargePartial:
				partial++
			case models.ChargeUnpaid:
				unpaid++
			default:
				continue
			}
			outstanding += s.Remaining
			overdue = append(overdue, s)
		}

		return c.JSON(fiber.Map{
			"today":                now,
			"month":                month,
			"monthRevenue":         monthRevenue,
			"activeTrainees":       activeTrainees,
			"overdueCount":         len(overdue),
			"partialCount":         partial,
			"unpaidCount":          unpaid,
			"outstanding":          round2(outstanding),
			"todaySessions":        todaySessions,
			"weekReminders":        weekReminders,
			"overdueSubscriptions": overdue,
		})
	})
}
