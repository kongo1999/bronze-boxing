package server

import (
	"encoding/json"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// The audit trail stores before/after as `any`, so the driver hands them back
// as primitive.D — which encoding/json renders as [{"Key":…,"Value":…}], a
// shape the UI can't diff. jsonable must flatten that to a plain object.
func TestJsonableFlattensBSONDocuments(t *testing.T) {
	oid := primitive.NewObjectID()
	when := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	in := primitive.D{
		{Key: "_id", Value: oid},
		{Key: "amount", Value: 120.0},
		{Key: "note", Value: "half now"},
		{Key: "date", Value: primitive.NewDateTimeFromTime(when)},
		{Key: "attendees", Value: primitive.A{
			primitive.D{{Key: "traineeName", Value: "Karim"}, {Key: "status", Value: "booked"}},
		}},
		{Key: "meta", Value: primitive.M{"voided": true}},
	}

	raw, err := json.Marshal(jsonable(in))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the result is not a JSON object: %v (%s)", err, raw)
	}

	if got["_id"] != oid.Hex() {
		t.Errorf("_id = %v, want the hex string %s", got["_id"], oid.Hex())
	}
	if got["amount"] != 120.0 {
		t.Errorf("amount = %v, want 120", got["amount"])
	}
	if got["note"] != "half now" {
		t.Errorf("note = %v, want %q", got["note"], "half now")
	}
	if got["date"] != "2026-05-01T12:00:00Z" {
		t.Errorf("date = %v, want an RFC3339 string", got["date"])
	}

	list, ok := got["attendees"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("attendees = %#v, want a one-element array", got["attendees"])
	}
	first, ok := list[0].(map[string]any)
	if !ok || first["traineeName"] != "Karim" {
		t.Errorf("attendees[0] = %#v, want a flattened object", list[0])
	}
	meta, ok := got["meta"].(map[string]any)
	if !ok || meta["voided"] != true {
		t.Errorf("meta = %#v, want {voided:true}", got["meta"])
	}
}

// A nil snapshot (an audit line that only recorded one side) must stay absent
// rather than becoming a confusing empty object.
func TestJsonablePassesThroughNilAndScalars(t *testing.T) {
	if got := jsonable(nil); got != nil {
		t.Errorf("jsonable(nil) = %#v, want nil", got)
	}
	if got := jsonable("plain"); got != "plain" {
		t.Errorf("jsonable(string) = %#v, want it untouched", got)
	}
	if got := jsonable(primitive.Null{}); got != nil {
		t.Errorf("jsonable(Null) = %#v, want nil", got)
	}
}
