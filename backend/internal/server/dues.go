package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

// Dues model
//
//   subscription_terms   append-only, effective-dated fee/status per trainee
//   subscription_charges one fixed row per (trainee, month) with its due and a
//                        paid-so-far projection kept in step with payments
//
// A month's charge is created once from the terms in force for that month
// and is never silently re-priced: later fee or status changes add terms,
// which only affect months that have no charge yet (or the current month,
// through an explicit, audited adjustment). Charges are materialized lazily
// and idempotently — when a month up to the current one is listed, or when a
// payment is recorded against any month — never for a month before the
// trainee's first billing term, so no unpaid charge is ever invented for
// history the app never recorded.

// billingTermFor returns the term that prices month: the one with the latest
// BillingFromMonth not after month (the latest recorded wins a tie).
func billingTermFor(terms []models.SubscriptionTerm, month string) *models.SubscriptionTerm {
	var best *models.SubscriptionTerm
	for i := range terms {
		t := &terms[i]
		if t.BillingFromMonth > month {
			continue
		}
		if best == nil || t.BillingFromMonth > best.BillingFromMonth ||
			(t.BillingFromMonth == best.BillingFromMonth && laterTerm(t, best)) {
			best = t
		}
	}
	return best
}

// statusTermOn returns the term whose status holds on day (YYYY-MM-DD): the
// latest EffectiveDate not after day (the latest recorded wins a tie).
func statusTermOn(terms []models.SubscriptionTerm, day string) *models.SubscriptionTerm {
	var best *models.SubscriptionTerm
	for i := range terms {
		t := &terms[i]
		if t.EffectiveDate > day {
			continue
		}
		if best == nil || t.EffectiveDate > best.EffectiveDate ||
			(t.EffectiveDate == best.EffectiveDate && laterTerm(t, best)) {
			best = t
		}
	}
	return best
}

func laterTerm(a, b *models.SubscriptionTerm) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.ID.Hex() > b.ID.Hex()
}

// latestTerm is the most recently recorded term (the trainee's current summary).
func latestTerm(terms []models.SubscriptionTerm) *models.SubscriptionTerm {
	var best *models.SubscriptionTerm
	for i := range terms {
		if best == nil || laterTerm(&terms[i], best) {
			best = &terms[i]
		}
	}
	return best
}

func billable(t *models.SubscriptionTerm) bool {
	return t != nil && t.Status == models.StatusActive && models.Cents(t.MonthlyFee) > 0
}

// chargeState derives a stored charge state. Imported rows stay "unverified"
// — out of paid/unpaid metrics — until an adjustment reconciles their due.
func chargeState(dueCents, paidCents int64, source string) string {
	switch {
	case source == models.ChargeSourceImported:
		return models.ChargeUnverified
	case dueCents <= 0 && paidCents <= 0:
		return models.ChargeWaived
	case paidCents >= dueCents:
		return models.ChargePaid
	case paidCents > 0:
		return models.ChargePartial
	default:
		return models.ChargeUnpaid
	}
}

// displayState adds the read-time "upcoming" state: a future month's charge
// with nothing paid yet is not overdue, it simply hasn't come around.
func displayState(state, month, currentMonth string) string {
	if month > currentMonth && state == models.ChargeUnpaid {
		return models.ChargeUpcoming
	}
	return state
}

func loadTerms(ctx context.Context, store *db.Store, filter bson.M) (map[primitive.ObjectID][]models.SubscriptionTerm, error) {
	cur, err := store.Coll(models.CollTerms).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var terms []models.SubscriptionTerm
	if err := cur.All(ctx, &terms); err != nil {
		return nil, err
	}
	out := map[primitive.ObjectID][]models.SubscriptionTerm{}
	for _, t := range terms {
		out[t.Trainee] = append(out[t.Trainee], t)
	}
	return out, nil
}

// livePaidCents sums live subscription payments for one trainee and month.
func livePaidCents(ctx context.Context, store *db.Store, trainee primitive.ObjectID, month string) (int64, error) {
	cur, err := store.Coll(models.CollPayments).Find(ctx, notVoided(bson.M{
		"type": models.PaySubscription, "trainee": trainee, "periodMonth": month,
	}))
	if err != nil {
		return 0, err
	}
	var ps []models.Payment
	if err := cur.All(ctx, &ps); err != nil {
		return 0, err
	}
	var sum int64
	for _, p := range ps {
		sum += models.Cents(p.Amount)
	}
	return sum, nil
}

var chargeKey = func(trainee primitive.ObjectID, month string) bson.M {
	return bson.M{"trainee": trainee, "periodMonth": month}
}

// ensureCharge returns the charge for (trainee, month), creating it from the
// trainee's terms if it doesn't exist yet. Returns nil when nothing is owed
// for that month (no billing term yet, inactive, or no fee). A new charge
// starts with whatever live payments already point at the month, so the
// projection is right even for payments that predate it.
func ensureCharge(ctx context.Context, store *db.Store, t models.Trainee, month string) (*models.SubscriptionCharge, error) {
	coll := store.Coll(models.CollCharges)
	var ch models.SubscriptionCharge
	err := coll.FindOne(ctx, chargeKey(t.ID, month)).Decode(&ch)
	if err == nil {
		return &ch, nil
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	terms, err := loadTerms(ctx, store, bson.M{"trainee": t.ID})
	if err != nil {
		return nil, err
	}
	term := billingTermFor(terms[t.ID], month)
	if !billable(term) {
		return nil, nil
	}
	paid, err := livePaidCents(ctx, store, t.ID, month)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	due := models.Cents(term.MonthlyFee)
	ch = models.SubscriptionCharge{
		Trainee: t.ID, TraineeName: t.Name, PeriodMonth: month,
		DueCents: due, PaidCents: paid, State: chargeState(due, paid, models.ChargeSourceNormal),
		Source: models.ChargeSourceNormal, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := coll.UpdateOne(ctx, chargeKey(t.ID, month), bson.M{"$setOnInsert": ch},
		options.Update().SetUpsert(true)); err != nil && !isDup(err) {
		return nil, err
	}
	if err := coll.FindOne(ctx, chargeKey(t.ID, month)).Decode(&ch); err != nil {
		return nil, err
	}
	return &ch, nil
}

// ensureChargesForMonth materializes the month's charges for every existing
// trainee whose terms bill them for it. Future months are never materialized
// here (only projected), so a later fee change still prices them.
func ensureChargesForMonth(ctx context.Context, store *db.Store, month string) error {
	if month > models.MonthKey(time.Now()) {
		return nil
	}
	terms, err := loadTerms(ctx, store, bson.M{"billingFromMonth": bson.M{"$lte": month}})
	if err != nil {
		return err
	}
	if len(terms) == 0 {
		return nil
	}
	have := map[primitive.ObjectID]bool{}
	cur, err := store.Coll(models.CollCharges).Find(ctx, bson.M{"periodMonth": month},
		options.Find().SetProjection(bson.M{"trainee": 1}))
	if err != nil {
		return err
	}
	var existing []models.SubscriptionCharge
	if err := cur.All(ctx, &existing); err != nil {
		return err
	}
	for _, c := range existing {
		have[c.Trainee] = true
	}
	var need []primitive.ObjectID
	for id, ts := range terms {
		if !have[id] && billable(billingTermFor(ts, month)) {
			need = append(need, id)
		}
	}
	if len(need) == 0 {
		return nil
	}
	tcur, err := store.Coll(models.CollTrainees).Find(ctx, bson.M{"_id": bson.M{"$in": need}})
	if err != nil {
		return err
	}
	var trainees []models.Trainee
	if err := tcur.All(ctx, &trainees); err != nil {
		return err
	}
	for _, t := range trainees {
		if _, err := ensureCharge(ctx, store, t, month); err != nil {
			return err
		}
	}
	return nil
}

// applyChargeDelta moves a charge's paid projection by deltaCents inside the
// caller's transaction. The update itself carries the guard — paid can never
// exceed due, nor drop below zero — so two concurrent payments cannot both
// pass: they write the same document, one aborts on the conflict, and its
// retry sees the other's payment. Imported (unverified) charges refuse new
// money until reconciled, but still accept reversals.
func applyChargeDelta(ctx context.Context, store *db.Store, trainee primitive.ObjectID, month string, deltaCents int64) error {
	if deltaCents == 0 {
		return nil
	}
	coll := store.Coll(models.CollCharges)
	filter := chargeKey(trainee, month)
	if deltaCents > 0 {
		filter["source"] = bson.M{"$ne": models.ChargeSourceImported}
		filter["$expr"] = bson.M{"$lte": bson.A{bson.M{"$add": bson.A{"$paidCents", deltaCents}}, "$dueCents"}}
	} else {
		filter["paidCents"] = bson.M{"$gte": -deltaCents}
	}
	var ch models.SubscriptionCharge
	err := coll.FindOneAndUpdate(ctx, filter,
		bson.M{"$inc": bson.M{"paidCents": deltaCents}, "$set": bson.M{"updatedAt": time.Now()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&ch)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return explainChargeRefusal(ctx, store, trainee, month, deltaCents)
	}
	if err != nil {
		return err
	}
	if st := chargeState(ch.DueCents, ch.PaidCents, ch.Source); st != ch.State {
		if _, err := coll.UpdateOne(ctx, bson.M{"_id": ch.ID}, bson.M{"$set": bson.M{"state": st}}); err != nil {
			return err
		}
	}
	return nil
}

func explainChargeRefusal(ctx context.Context, store *db.Store, trainee primitive.ObjectID, month string, deltaCents int64) error {
	var cur models.SubscriptionCharge
	if err := store.Coll(models.CollCharges).FindOne(ctx, chargeKey(trainee, month)).Decode(&cur); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return apiErr(http.StatusBadRequest, CodeNoCharge,
				fmt.Sprintf("No dues are recorded for %s — set a monthly fee first, or log this as another payment type.", month)).
				withField("periodMonth")
		}
		return err
	}
	if deltaCents > 0 && cur.Source == models.ChargeSourceImported {
		return apiErr(http.StatusConflict, CodeUnverifiedDue,
			fmt.Sprintf("Dues for %s were imported from older records and need review. Confirm the amount due first, then record the payment.", month)).
			withField("periodMonth")
	}
	remaining := cur.DueCents - cur.PaidCents
	if remaining < 0 {
		remaining = 0
	}
	if deltaCents > 0 {
		return apiErr(http.StatusBadRequest, CodeOverpayment, fmt.Sprintf(
			"%s would overpay %s: due %s, already paid %s, remaining %s",
			fmtAmt(models.Amount(deltaCents)), month, fmtAmt(models.Amount(cur.DueCents)),
			fmtAmt(models.Amount(cur.PaidCents)), fmtAmt(models.Amount(remaining)))).
			withField("amount").
			withDetails(map[string]float64{
				"due": models.Amount(cur.DueCents), "paid": models.Amount(cur.PaidCents), "remaining": models.Amount(remaining),
			})
	}
	return apiErr(http.StatusConflict, CodeConflict, "that reversal would take the paid balance below zero")
}

// chargeView is a charge as the API returns it.
type chargeView struct {
	ID          string    `json:"id"`
	Trainee     string    `json:"trainee"`
	TraineeName string    `json:"traineeName"`
	PeriodMonth string    `json:"periodMonth"`
	Due         float64   `json:"due"`
	PaidAmount  float64   `json:"paidAmount"`
	Remaining   float64   `json:"remaining"`
	State       string    `json:"state"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func viewCharge(c models.SubscriptionCharge) chargeView {
	rem := c.DueCents - c.PaidCents
	if rem < 0 || c.Source == models.ChargeSourceImported {
		rem = max(rem, 0)
	}
	return chargeView{
		ID: c.ID.Hex(), Trainee: c.Trainee.Hex(), TraineeName: c.TraineeName, PeriodMonth: c.PeriodMonth,
		Due: models.Amount(c.DueCents), PaidAmount: models.Amount(c.PaidCents), Remaining: models.Amount(rem),
		State: displayState(c.State, c.PeriodMonth, models.MonthKey(time.Now())), Source: c.Source,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// adjustCharge sets a charge's due amount (inside the caller's transaction)
// and records why. The due can't go below what is already paid — there is no
// credit/refund flow, so that would leave money with nowhere to live. Setting
// the due to exactly the paid amount settles (forgives) the rest; setting it
// to zero with nothing paid waives the month. Adjusting an imported charge is
// how it is reconciled: it becomes a normal, verified record.
func adjustCharge(ctx context.Context, store *db.Store, id primitive.ObjectID, dueCents int64, reason, actor string) (*models.SubscriptionCharge, error) {
	coll := store.Coll(models.CollCharges)
	var before models.SubscriptionCharge
	if err := coll.FindOne(ctx, bson.M{"_id": id}).Decode(&before); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, notFound("charge")
		}
		return nil, err
	}
	if dueCents < before.PaidCents {
		return nil, apiErr(http.StatusBadRequest, CodeDueBelowPaid, fmt.Sprintf(
			"%s has already been paid toward %s, so the amount due can't be set below that.",
			fmtAmt(models.Amount(before.PaidCents)), before.PeriodMonth)).withField("due")
	}
	after := before
	after.DueCents = dueCents
	after.Source = models.ChargeSourceAdjusted
	after.State = chargeState(dueCents, before.PaidCents, after.Source)
	after.UpdatedAt = time.Now()
	// Guard on the paid projection we read, so a payment landing between the
	// read and this write can't be stranded above the new due.
	res, err := coll.UpdateOne(ctx, bson.M{"_id": id, "paidCents": before.PaidCents}, bson.M{"$set": bson.M{
		"dueCents": after.DueCents, "source": after.Source, "state": after.State, "updatedAt": after.UpdatedAt,
	}})
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, apiErr(http.StatusConflict, CodeConflict, "a payment changed this month's balance just now — reload and try again")
	}
	if err := writeAudit(ctx, store, auditLine{
		Entity: "charge", Ref: id, Action: "adjust",
		Before: viewCharge(before), After: viewCharge(after), Actor: actor, Reason: reason,
	}); err != nil {
		return nil, err
	}
	return &after, nil
}

// subRow is one line of the dues roster for a month. It keeps the original
// {trainee, due, amountPaid, state} shape and appends the balance, period,
// provenance and payment evidence.
type subRow struct {
	Trainee         models.Trainee `json:"trainee"`
	ChargeID        string         `json:"chargeId,omitempty"`
	PeriodMonth     string         `json:"periodMonth"`
	Due             float64        `json:"due"`
	AmountPaid      float64        `json:"amountPaid"`
	Remaining       float64        `json:"remaining"`
	State           string         `json:"state"`
	Source          string         `json:"source"`
	Projected       bool           `json:"projected,omitempty"`
	PaymentCount    int            `json:"paymentCount"`
	LastPaymentDate *time.Time     `json:"lastPaymentDate,omitempty"`
}

type paidAgg struct {
	cents int64
	count int
	last  time.Time
}

// listDues builds the month's roster from stored charges (including trainees
// who have since gone inactive or been removed) plus, for future months,
// projected rows from the terms. Paid amounts come from the live payments for
// that exact trainee and period.
func listDues(ctx context.Context, store *db.Store, month string) ([]subRow, error) {
	current := models.MonthKey(time.Now())
	if err := ensureChargesForMonth(ctx, store, month); err != nil {
		return nil, err
	}
	cur, err := store.Coll(models.CollCharges).Find(ctx, bson.M{"periodMonth": month})
	if err != nil {
		return nil, err
	}
	var charges []models.SubscriptionCharge
	if err := cur.All(ctx, &charges); err != nil {
		return nil, err
	}

	pcur, err := store.Coll(models.CollPayments).Find(ctx, notVoided(bson.M{
		"type": models.PaySubscription, "periodMonth": month,
	}))
	if err != nil {
		return nil, err
	}
	var pays []models.Payment
	if err := pcur.All(ctx, &pays); err != nil {
		return nil, err
	}
	paid := map[primitive.ObjectID]*paidAgg{}
	for _, p := range pays {
		if p.Trainee == nil {
			continue
		}
		a := paid[*p.Trainee]
		if a == nil {
			a = &paidAgg{}
			paid[*p.Trainee] = a
		}
		a.cents += models.Cents(p.Amount)
		a.count++
		if p.Date.After(a.last) {
			a.last = p.Date
		}
	}

	// Projected rows for a future month: billable by terms, no charge yet.
	have := map[primitive.ObjectID]bool{}
	for _, c := range charges {
		have[c.Trainee] = true
	}
	var projected []models.SubscriptionTerm
	if month > current {
		terms, err := loadTerms(ctx, store, bson.M{"billingFromMonth": bson.M{"$lte": month}})
		if err != nil {
			return nil, err
		}
		for id, ts := range terms {
			if t := billingTermFor(ts, month); !have[id] && billable(t) {
				projected = append(projected, *t)
			}
		}
	}

	ids := make([]primitive.ObjectID, 0, len(charges)+len(projected))
	for _, c := range charges {
		ids = append(ids, c.Trainee)
	}
	for _, t := range projected {
		ids = append(ids, t.Trainee)
	}
	byID := map[primitive.ObjectID]models.Trainee{}
	if len(ids) > 0 {
		tcur, err := store.Coll(models.CollTrainees).Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
		if err != nil {
			return nil, err
		}
		var ts []models.Trainee
		if err := tcur.All(ctx, &ts); err != nil {
			return nil, err
		}
		for _, t := range ts {
			byID[t.ID] = t
		}
	}

	out := make([]subRow, 0, len(charges)+len(projected))
	for _, c := range charges {
		t, ok := byID[c.Trainee]
		if !ok { // removed trainee: keep the history under the name it was charged to
			t = models.Trainee{ID: c.Trainee, Name: c.TraineeName, Status: "removed"}
		}
		a := paid[c.Trainee]
		if a == nil {
			a = &paidAgg{}
		}
		row := subRow{
			Trainee: t, ChargeID: c.ID.Hex(), PeriodMonth: month,
			Due: models.Amount(c.DueCents), AmountPaid: models.Amount(a.cents),
			Remaining: models.Amount(max(c.DueCents-a.cents, 0)),
			State:     displayState(chargeState(c.DueCents, a.cents, c.Source), month, current),
			Source:    c.Source, PaymentCount: a.count,
		}
		if a.count > 0 {
			last := a.last
			row.LastPaymentDate = &last
		}
		out = append(out, row)
	}
	for _, term := range projected {
		t, ok := byID[term.Trainee]
		if !ok {
			continue // terms of a removed trainee never project new dues
		}
		due := models.Cents(term.MonthlyFee)
		out = append(out, subRow{
			Trainee: t, PeriodMonth: month, Due: models.Amount(due), Remaining: models.Amount(due),
			State: models.ChargeUpcoming, Source: "projected", Projected: true,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Trainee.Name < out[j].Trainee.Name })
	return out, nil
}
