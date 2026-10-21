package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/models"
)

const maxBulkClasses = 200

func bulkIDs(raw []string) ([]primitive.ObjectID, error) {
	if len(raw) == 0 || len(raw) > maxBulkClasses {
		return nil, badField("ids", fmt.Sprintf("select between 1 and %d classes", maxBulkClasses))
	}
	ids := make([]primitive.ObjectID, 0, len(raw))
	seen := make(map[primitive.ObjectID]bool, len(raw))
	for _, s := range raw {
		id, err := parseOID(s, "ids")
		if err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, badField("ids", "the same class was selected twice")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func (h *sessionHandler) bulkTargets(ctx context.Context, ids []primitive.ObjectID) ([]models.Session, error) {
	cur, err := h.store.Coll(models.CollSessions).Find(ctx, bson.M{"_id": bson.M{"$in": ids}},
		options.Find().SetSort(bson.D{{Key: "start", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var rows []models.Session
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	if len(rows) != len(ids) {
		return nil, apiErr(http.StatusConflict, CodeConflict, "one or more selected classes no longer exist; reload and select again")
	}
	for _, s := range rows {
		if s.Status != models.SessScheduled {
			return nil, apiErr(http.StatusConflict, CodeConflict, fmt.Sprintf("%s is already %s; bulk actions only change scheduled classes", s.Title, s.Status))
		}
	}
	return rows, nil
}

type bulkEditInput struct {
	IDs     []string      `json:"ids"`
	Changes seriesChanges `json:"changes"`
	Reason  string        `json:"reason"`
}

// bulkEdit changes arbitrary scheduled classes, including classes from
// different series. Checks every destination before writing any of them.
func (h *sessionHandler) bulkEdit(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in bulkEditInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	ids, err := bulkIDs(in.IDs)
	if err != nil {
		return err
	}
	reason, err := requireReason(in.Reason, "change selected classes")
	if err != nil {
		return err
	}
	ch := in.Changes
	if ch.Type != nil || ch.Status != nil {
		return badField("changes", "bulk edit can change title, time, duration, location, capacity or fee")
	}
	if ch.Title == nil && ch.Time == nil && ch.DurationMin == nil && ch.Location == nil && ch.Capacity == nil && ch.Fee == nil {
		return badField("changes", "choose at least one field to change")
	}
	hh, mm, err := validateSeriesChanges(ch)
	if err != nil {
		return err
	}
	changed := 0
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		changed = 0
		before, err := h.bulkTargets(tx, ids)
		if err != nil {
			return err
		}
		after := make([]models.Session, len(before))
		bySeries := map[string]int{}
		for i, s := range before {
			if s.SeriesID != "" {
				bySeries[s.SeriesID]++
			}
			if ch.Title != nil {
				s.Title = strings.TrimSpace(*ch.Title)
			}
			if ch.Time != nil {
				d := s.Start.In(time.Local)
				s.Start = time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, time.Local)
			}
			if ch.DurationMin != nil {
				s.DurationMin = *ch.DurationMin
			}
			if ch.Location != nil {
				s.Location = strings.TrimSpace(*ch.Location)
			}
			if ch.Capacity != nil {
				s.Capacity = *ch.Capacity
			}
			if ch.Fee != nil {
				s.Fee = round2(*ch.Fee)
				if s.Fee != before[i].Fee {
					n, err := linkedClassPayments(tx, h.store, s.ID, nil)
					if err != nil {
						return err
					}
					if n > 0 {
						return apiErr(http.StatusConflict, CodeConflict, fmt.Sprintf("%s has a linked payment; change its fee individually after reviewing that payment", s.Title))
					}
				}
			}
			if s.Capacity > 0 && len(s.Attendees) > s.Capacity {
				return apiErr(http.StatusConflict, CodeCapacity, fmt.Sprintf("%s already has %d booked; capacity cannot be %d", s.Title, len(s.Attendees), s.Capacity))
			}
			if ch.Time != nil {
				for _, a := range s.Attendees {
					if err := notStartedErr(s.Start, a.Status); err != nil {
						return err
					}
				}
			}
			after[i] = s
		}
		if ch.Time != nil || ch.DurationMin != nil {
			for i, s := range after {
				clash, err := h.findOverlapExcluding(tx, s.Type, s.Start, s.DurationMin, ids)
				if err != nil {
					return err
				}
				if clash != nil {
					return apiErr(http.StatusConflict, CodeScheduleClash, fmt.Sprintf("%s on %s: %s", s.Title, models.DateKey(s.Start), overlapMsg(s.Type, clash)))
				}
				for j := 0; j < i; j++ {
					if overlapForbidden(s.Type, s.Start, s.DurationMin, after[j]) {
						return apiErr(http.StatusConflict, CodeScheduleClash, fmt.Sprintf("selected classes %q and %q would overlap", s.Title, after[j].Title))
					}
				}
			}
		}
		for i, s := range after {
			set := bson.M{"updatedAt": time.Now()}
			if ch.Title != nil {
				set["title"] = s.Title
			}
			if ch.Time != nil {
				set["start"] = s.Start
			}
			if ch.DurationMin != nil {
				set["durationMin"] = s.DurationMin
			}
			if ch.Location != nil {
				set["location"] = s.Location
			}
			if ch.Capacity != nil {
				set["capacity"] = s.Capacity
			}
			if ch.Fee != nil {
				set["fee"] = s.Fee
			}
			if _, err := h.store.Coll(models.CollSessions).UpdateOne(tx, bson.M{"_id": s.ID, "status": models.SessScheduled}, bson.M{"$set": set}); err != nil {
				return err
			}
			if err := writeAudit(tx, h.store, auditLine{Entity: "session", Ref: s.ID, Action: "bulk_edit", Before: before[i], After: s, Actor: actorOf(c), Reason: reason}); err != nil {
				return err
			}
			changed++
		}
		// If every unfinished occurrence was selected, update the template used
		// when the coach later extends that series. A partial selection leaves it.
		for sid, selectedCount := range bySeries {
			total, err := h.store.Coll(models.CollSessions).CountDocuments(tx, bson.M{"seriesId": sid, "status": models.SessScheduled})
			if err != nil {
				return err
			}
			if int64(selectedCount) != total {
				continue
			}
			record, err := h.seriesRecord(tx, sid)
			if err != nil {
				return err
			}
			set := bson.M{"updatedAt": time.Now()}
			if ch.Title != nil {
				set["title"] = strings.TrimSpace(*ch.Title)
			}
			if ch.Time != nil {
				set["time"] = *ch.Time
			}
			if ch.DurationMin != nil {
				set["durationMin"] = *ch.DurationMin
			}
			if _, err := h.store.Coll(models.CollSeries).UpdateOne(tx, bson.M{"_id": record.ID}, bson.M{"$set": set}); err != nil {
				return err
			}
			if err := writeAudit(tx, h.store, auditLine{Entity: "series", Ref: record.ID, Action: "bulk_edit_unfinished", Before: record, After: set, Actor: actorOf(c), Reason: reason}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true, "changed": changed})
}

type bulkDeleteInput struct {
	IDs    []string `json:"ids"`
	Reason string   `json:"reason"`
}

func (h *sessionHandler) bulkDelete(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in bulkDeleteInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	ids, err := bulkIDs(in.IDs)
	if err != nil {
		return err
	}
	reason, err := requireReason(in.Reason, "delete selected classes")
	if err != nil {
		return err
	}
	deleted := 0
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		deleted = 0
		rows, err := h.bulkTargets(tx, ids)
		if err != nil {
			return err
		}
		seriesCounts := map[string]int{}
		for _, s := range rows {
			for _, a := range s.Attendees {
				if a.Status != models.AttendBooked {
					return apiErr(http.StatusConflict, CodeConflict, fmt.Sprintf("%s has attendance recorded; keep it in history", s.Title))
				}
			}
			n, err := linkedClassPayments(tx, h.store, s.ID, nil)
			if err != nil {
				return err
			}
			if n > 0 {
				return apiErr(http.StatusConflict, CodeConflict, fmt.Sprintf("%s has linked payments; correct or void them first", s.Title))
			}
			for _, collection := range []string{models.CollWaitlist, models.CollCancellations, models.CollProgress} {
				n, err := h.store.Coll(collection).CountDocuments(tx, bson.M{"session": s.ID})
				if err != nil {
					return err
				}
				if n > 0 {
					return apiErr(http.StatusConflict, CodeConflict, fmt.Sprintf("%s has recorded activity; cancel it instead", s.Title))
				}
			}
			if s.SeriesID != "" {
				seriesCounts[s.SeriesID]++
			}
		}
		series := map[string]models.SessionSeries{}
		for sid := range seriesCounts {
			record, err := h.seriesRecord(tx, sid)
			if err != nil {
				return err
			}
			series[sid] = record
		}
		res, err := h.store.Coll(models.CollSessions).DeleteMany(tx, bson.M{"_id": bson.M{"$in": ids}, "status": models.SessScheduled})
		if err != nil {
			return err
		}
		if res.DeletedCount != int64(len(ids)) {
			return apiErr(http.StatusConflict, CodeConflict, "classes changed while deleting; reload and try again")
		}
		for sid, count := range seriesCounts {
			record := series[sid]
			planned := max(record.PlannedCount-count, 0)
			set := bson.M{"plannedCount": planned, "updatedAt": time.Now()}
			if planned == 0 {
				set["status"] = "ended"
			}
			if _, err := h.store.Coll(models.CollSeries).UpdateOne(tx, bson.M{"_id": record.ID}, bson.M{"$set": set}); err != nil {
				return err
			}
			if err := writeAudit(tx, h.store, auditLine{Entity: "series", Ref: record.ID, Action: "bulk_delete_occurrences", Before: fiber.Map{"plannedCount": record.PlannedCount}, After: fiber.Map{"plannedCount": planned, "deleted": count}, Actor: actorOf(c), Reason: reason}); err != nil {
				return err
			}
		}
		for _, s := range rows {
			if err := writeAudit(tx, h.store, auditLine{Entity: "session", Ref: s.ID, Action: "bulk_delete", Before: s, Actor: actorOf(c), Reason: reason}); err != nil {
				return err
			}
		}
		deleted = len(rows)
		return nil
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true, "deleted": deleted})
}
