package server

import (
	"context"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

// planNearing is an active session plan worth a look: two or fewer sessions
// left, or ending within a week.
type planNearing struct {
	ID          string `json:"id"`
	Trainee     string `json:"trainee"`
	TraineeName string `json:"traineeName"`
	Title       string `json:"title"`
	Remaining   int    `json:"remaining"`
	EndDate     string `json:"endDate,omitempty"`
}

const (
	nearingSessions = 2
	nearingDays     = 7
)

func plansNearing(ctx context.Context, store *db.Store, now time.Time) ([]planNearing, error) {
	var plans []models.SessionPlan
	if cur, err := store.Coll(models.CollPlans).Find(ctx, bson.M{"status": "active"}); err != nil {
		return nil, err
	} else if err := cur.All(ctx, &plans); err != nil {
		return nil, err
	}
	out := []planNearing{}
	if len(plans) == 0 {
		return out, nil
	}
	prog, err := planProgress(ctx, store, plans, nil)
	if err != nil {
		return nil, err
	}
	soon := models.DateKey(now.AddDate(0, 0, nearingDays))
	for _, p := range plans {
		left := prog[p.ID].Remaining
		ending := p.EndDate != "" && p.EndDate <= soon
		if left > nearingSessions && !ending {
			continue
		}
		out = append(out, planNearing{ID: p.ID.Hex(), Trainee: p.Trainee.Hex(), TraineeName: p.TraineeName,
			Title: p.Title, Remaining: left, EndDate: p.EndDate})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Remaining != out[j].Remaining {
			return out[i].Remaining < out[j].Remaining
		}
		return out[i].TraineeName < out[j].TraineeName
	})
	return out, nil
}
