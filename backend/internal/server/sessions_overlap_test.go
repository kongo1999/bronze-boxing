package server

import (
	"testing"
	"time"

	"bronzeboxing/internal/models"
)

func at(hh, mm int) time.Time {
	return time.Date(2026, 7, 6, hh, mm, 0, 0, time.Local)
}

func sess(typ string, hh, mm, dur int) models.Session {
	return models.Session{Type: typ, Start: at(hh, mm), DurationMin: dur, Status: models.SessScheduled}
}

func TestOverlapForbidden(t *testing.T) {
	cases := []struct {
		name    string
		newType string
		start   time.Time
		dur     int
		ex      models.Session
		want    bool
	}{
		// Private vs private: always allowed, even when identical.
		{"private over private, same slot", models.SessionPrivate, at(18, 0), 60, sess(models.SessionPrivate, 18, 0, 60), false},
		{"private over private, crossing", models.SessionPrivate, at(18, 30), 60, sess(models.SessionPrivate, 18, 0, 60), false},

		// Group vs group: any overlap forbidden.
		{"group over group, overlapping", models.SessionGroup, at(18, 30), 60, sess(models.SessionGroup, 18, 0, 60), true},
		{"group over group, back-to-back ok", models.SessionGroup, at(19, 0), 60, sess(models.SessionGroup, 18, 0, 60), false},

		// Group vs private (either direction): forbidden when overlapping.
		{"group over private", models.SessionGroup, at(18, 30), 60, sess(models.SessionPrivate, 18, 0, 60), true},
		{"private over group", models.SessionPrivate, at(18, 30), 60, sess(models.SessionGroup, 18, 0, 60), true},

		// No time overlap: never forbidden regardless of type.
		{"group, disjoint before", models.SessionGroup, at(16, 0), 60, sess(models.SessionGroup, 18, 0, 60), false},
		{"group touching edge (half-open)", models.SessionGroup, at(17, 0), 60, sess(models.SessionGroup, 18, 0, 60), false},

		// Cancelled existing never blocks, even a group.
		{"group over cancelled group", models.SessionGroup, at(18, 0), 60, models.Session{Type: models.SessionGroup, Start: at(18, 0), DurationMin: 60, Status: models.SessCancelled}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := overlapForbidden(tc.newType, tc.start, tc.dur, tc.ex); got != tc.want {
				t.Fatalf("overlapForbidden = %v, want %v", got, tc.want)
			}
		})
	}
}
