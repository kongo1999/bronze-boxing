package server

import (
	"context"
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/migrate"
	"bronzeboxing/internal/models"
)

// Migrations is the ordered list cmd/migrate applies and the API checks for
// at boot. Append only; never edit or reorder a migration that has shipped.
func Migrations() []migrate.Migration {
	return []migrate.Migration{
		{ID: "2026-09-001-core-indexes", Description: "Indexes for date, trainee, period, status and ledger queries", Up: migrateCoreIndexes},
		{ID: "2026-09-002-stock-ledger-opening", Description: "Open each item's stock ledger with its current stock", Up: migrateStockOpening},
		{ID: "2026-09-003-subscription-terms-charges", Description: "Membership terms from current trainee records; monthly charges from existing dues", Up: migrateTermsAndCharges},
		{ID: "2026-09-004-reminder-due-day", Description: "Store each reminder's studio-local due day", Up: migrateReminderDueDay},
		{ID: "2026-09-005-closings-returns-indexes", Description: "Indexes for daily cash counts and sale returns", Up: migrateClosingsReturnsIndexes},
		{ID: "2026-09-006-series-and-plans", Description: "Series records for existing recurring sessions; indexes for series and session plans", Up: migrateSeriesAndPlans},
	}
}

func idx(keys bson.D, unique bool) mongo.IndexModel {
	m := mongo.IndexModel{Keys: keys}
	if unique {
		m.Options = options.Index().SetUnique(true)
	}
	return m
}

func migrateCoreIndexes(ctx context.Context, r *migrate.Runner) (migrate.Result, error) {
	res := migrate.Result{}
	specs := map[string][]mongo.IndexModel{
		models.CollPayments: {
			idx(bson.D{{Key: "date", Value: 1}}, false),
			idx(bson.D{{Key: "trainee", Value: 1}, {Key: "date", Value: -1}}, false),
			idx(bson.D{{Key: "type", Value: 1}, {Key: "periodMonth", Value: 1}, {Key: "trainee", Value: 1}}, false),
		},
		models.CollExpenses: {idx(bson.D{{Key: "date", Value: 1}}, false)},
		models.CollSales: {
			idx(bson.D{{Key: "date", Value: 1}}, false),
			idx(bson.D{{Key: "item", Value: 1}, {Key: "date", Value: -1}}, false),
			idx(bson.D{{Key: "trainee", Value: 1}, {Key: "date", Value: -1}}, false),
		},
		models.CollSessions: {
			idx(bson.D{{Key: "start", Value: 1}}, false),
			idx(bson.D{{Key: "seriesId", Value: 1}, {Key: "start", Value: 1}}, false),
			idx(bson.D{{Key: "attendees.trainee", Value: 1}, {Key: "start", Value: -1}}, false),
		},
		models.CollReminders: {idx(bson.D{{Key: "dueDate", Value: 1}}, false), idx(bson.D{{Key: "done", Value: 1}, {Key: "dueDate", Value: 1}}, false)},
		models.CollAudit:     {idx(bson.D{{Key: "entity", Value: 1}, {Key: "ref", Value: 1}, {Key: "at", Value: -1}}, false)},
		models.CollMovements: {idx(bson.D{{Key: "item", Value: 1}, {Key: "at", Value: -1}}, false), idx(bson.D{{Key: "at", Value: 1}}, false)},
		models.CollTerms: {
			idx(bson.D{{Key: "trainee", Value: 1}, {Key: "effectiveDate", Value: 1}}, false),
			idx(bson.D{{Key: "billingFromMonth", Value: 1}}, false),
		},
		models.CollCharges: {
			idx(bson.D{{Key: "trainee", Value: 1}, {Key: "periodMonth", Value: 1}}, true),
			idx(bson.D{{Key: "periodMonth", Value: 1}, {Key: "state", Value: 1}}, false),
		},
	}
	names := make([]string, 0, len(specs))
	for k := range specs {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, coll := range names {
		n, err := r.EnsureIndexes(ctx, coll, specs[coll])
		if err != nil {
			return res, err
		}
		res.Add("indexes", n)
	}
	return res, nil
}

func migrateClosingsReturnsIndexes(ctx context.Context, r *migrate.Runner) (migrate.Result, error) {
	res := migrate.Result{}
	for coll, models_ := range map[string][]mongo.IndexModel{
		models.CollClosings: {idx(bson.D{{Key: "day", Value: 1}}, true)},
		models.CollReturns:  {idx(bson.D{{Key: "sale", Value: 1}}, false), idx(bson.D{{Key: "date", Value: 1}}, false)},
	} {
		n, err := r.EnsureIndexes(ctx, coll, models_)
		if err != nil {
			return res, err
		}
		res.Add("indexes", n)
	}
	return res, nil
}

// migrateSeriesAndPlans gives every existing recurring series a record. The
// original request (how many weeks were asked for) was never stored, so the
// planned count is the number of occurrences still present — marked
// inferred, and shown that way.
func migrateSeriesAndPlans(ctx context.Context, r *migrate.Runner) (migrate.Result, error) {
	res := migrate.Result{}
	for coll, specs := range map[string][]mongo.IndexModel{
		models.CollSeries:   {idx(bson.D{{Key: "seriesId", Value: 1}}, true)},
		models.CollPlans:    {idx(bson.D{{Key: "trainee", Value: 1}, {Key: "status", Value: 1}}, false)},
		models.CollSessions: {idx(bson.D{{Key: "attendees.planId", Value: 1}}, false)},
	} {
		n, err := r.EnsureIndexes(ctx, coll, specs)
		if err != nil {
			return res, err
		}
		res.Add("indexes", n)
	}
	ids, err := r.Store.Coll(models.CollSessions).Distinct(ctx, "seriesId", bson.M{"seriesId": bson.M{"$nin": bson.A{nil, ""}}})
	if err != nil {
		return res, err
	}
	for _, v := range ids {
		sid, ok := v.(string)
		if !ok {
			continue
		}
		n, err := r.Store.Coll(models.CollSeries).CountDocuments(ctx, bson.M{"seriesId": sid})
		if err != nil {
			return res, err
		}
		if n > 0 {
			res.Inc("series_existing")
			continue
		}
		cur, err := r.Store.Coll(models.CollSessions).Find(ctx, bson.M{"seriesId": sid}, options.Find().SetSort(bson.D{{Key: "start", Value: 1}}))
		if err != nil {
			return res, err
		}
		var occ []models.Session
		if err := cur.All(ctx, &occ); err != nil {
			return res, err
		}
		if len(occ) == 0 {
			continue
		}
		res.Inc("series_inferred")
		if r.DryRun {
			continue
		}
		if _, err := r.Store.Coll(models.CollSeries).InsertOne(ctx, inferSeries(sid, occ, r.Now)); err != nil && !isDup(err) {
			return res, err
		}
	}
	return res, nil
}

// migrateStockOpening gives every item without a ledger an "opening" line
// equal to its current stock, so from here on the movements always add up to
// what is on the shelf. History before this line is not reconstructed.
func migrateStockOpening(ctx context.Context, r *migrate.Runner) (migrate.Result, error) {
	res := migrate.Result{}
	cur, err := r.Store.Coll(models.CollInventory).Find(ctx, bson.M{})
	if err != nil {
		return res, err
	}
	var items []models.InventoryItem
	if err := cur.All(ctx, &items); err != nil {
		return res, err
	}
	for _, it := range items {
		n, err := r.Store.Coll(models.CollMovements).CountDocuments(ctx, bson.M{"item": it.ID})
		if err != nil {
			return res, err
		}
		if n > 0 {
			res.Inc("items_already_opened")
			continue
		}
		res.Inc("opening_lines")
		if r.DryRun {
			continue
		}
		if _, err := r.Store.Coll(models.CollMovements).InsertOne(ctx, models.StockMovement{
			Item: it.ID, ItemName: it.Name, Delta: it.Stock, Kind: models.MoveOpening,
			Reason: "Stock on hand when the stock ledger began", StockAfter: it.Stock, At: r.Now, Actor: "migration",
		}); err != nil {
			return res, err
		}
	}
	return res, nil
}

type payGroup struct {
	trainee primitive.ObjectID
	month   string
	cents   int64
	count   int
	name    string
}

// migrateTermsAndCharges starts the dues history:
//
//   - every trainee without terms gets one "migrated" term with today's fee
//     and status, billing from the current month; its effective date is the
//     trainee's creation date — an inference, flagged by Source;
//   - every (trainee, month) that already has live subscription payments gets
//     a charge: months before now become "imported_unverified" (paid is known,
//     what was due is not), the current month is priced from the terms;
//   - the current month gets a charge for every billable trainee.
//
// No unpaid charge is invented for any earlier month. Conflicts (payments
// above today's fee, payments without a trainee or period) are reported for
// review rather than guessed.
func migrateTermsAndCharges(ctx context.Context, r *migrate.Runner) (migrate.Result, error) {
	res := migrate.Result{}
	store := r.Store
	current := models.MonthKey(r.Now)

	tcur, err := store.Coll(models.CollTrainees).Find(ctx, bson.M{})
	if err != nil {
		return res, err
	}
	var trainees []models.Trainee
	if err := tcur.All(ctx, &trainees); err != nil {
		return res, err
	}
	byID := map[primitive.ObjectID]models.Trainee{}
	for _, t := range trainees {
		byID[t.ID] = t
	}
	terms, err := loadTerms(ctx, store, bson.M{})
	if err != nil {
		return res, err
	}
	for _, t := range trainees {
		if len(terms[t.ID]) > 0 {
			res.Inc("terms_existing")
			continue
		}
		eff := models.DateKey(t.CreatedAt)
		if t.CreatedAt.IsZero() {
			eff = models.DateKey(r.Now)
		}
		term := models.SubscriptionTerm{
			ID: primitive.NewObjectID(), Trainee: t.ID, EffectiveDate: eff, BillingFromMonth: current,
			MonthlyFee: round2(t.MonthlyFee), Status: defaultStr(t.Status, models.StatusActive), Source: "migrated",
			Reason: "Created from the trainee record when dues history began", CreatedAt: r.Now, Actor: "migration",
		}
		terms[t.ID] = []models.SubscriptionTerm{term}
		res.Inc("terms_created")
		if r.DryRun {
			continue
		}
		if _, err := store.Coll(models.CollTerms).InsertOne(ctx, term); err != nil {
			return res, err
		}
		if _, err := store.Coll(models.CollTrainees).UpdateOne(ctx, bson.M{"_id": t.ID, "feeFromMonth": bson.M{"$exists": false}},
			bson.M{"$set": bson.M{"feeFromMonth": current}}); err != nil {
			return res, err
		}
	}

	// Group live subscription payments by (trainee, period).
	pcur, err := store.Coll(models.CollPayments).Find(ctx, notVoided(bson.M{"type": models.PaySubscription}),
		options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return res, err
	}
	var pays []models.Payment
	if err := pcur.All(ctx, &pays); err != nil {
		return res, err
	}
	groups := map[string]*payGroup{}
	var keys []string
	for _, p := range pays {
		if p.Trainee == nil || !models.ValidMonth(p.PeriodMonth) {
			res.Inc("payments_needing_review")
			r.Notef("subscription payment %s (%s, %s, %s) has no trainee or period — it counts toward no month's dues; edit it to set them",
				p.ID.Hex(), models.DateKey(p.Date), fmtAmt(p.Amount), defaultStr(p.TraineeName, "no trainee"))
			continue
		}
		k := p.Trainee.Hex() + "|" + p.PeriodMonth
		g := groups[k]
		if g == nil {
			g = &payGroup{trainee: *p.Trainee, month: p.PeriodMonth, name: p.TraineeName}
			groups[k] = g
			keys = append(keys, k)
		}
		g.cents += models.Cents(p.Amount)
		g.count++
	}
	sort.Strings(keys)

	insertCharge := func(ch models.SubscriptionCharge) error {
		if r.DryRun {
			return nil
		}
		_, err := store.Coll(models.CollCharges).UpdateOne(ctx, chargeKey(ch.Trainee, ch.PeriodMonth),
			bson.M{"$setOnInsert": ch}, options.Update().SetUpsert(true))
		return err
	}
	exists := func(tid primitive.ObjectID, month string) (bool, *models.SubscriptionCharge, error) {
		var ch models.SubscriptionCharge
		err := store.Coll(models.CollCharges).FindOne(ctx, chargeKey(tid, month)).Decode(&ch)
		if err == mongo.ErrNoDocuments {
			return false, nil, nil
		}
		return err == nil, &ch, err
	}

	covered := map[string]bool{}
	for _, k := range keys {
		g := groups[k]
		covered[k] = true
		ok, ch, err := exists(g.trainee, g.month)
		if err != nil {
			return res, err
		}
		if ok {
			res.Inc("charges_existing")
			if ch.PaidCents != g.cents {
				r.Notef("charge %s %s: recorded paid %s but live payments total %s — run `migrate -verify`",
					defaultStr(ch.TraineeName, g.name), g.month, fmtAmt(models.Amount(ch.PaidCents)), fmtAmt(models.Amount(g.cents)))
			}
			continue
		}
		name := g.name
		if t, ok := byID[g.trainee]; ok {
			name = t.Name
		}
		ch2 := models.SubscriptionCharge{
			Trainee: g.trainee, TraineeName: name, PeriodMonth: g.month,
			DueCents: g.cents, PaidCents: g.cents, Source: models.ChargeSourceImported,
			CreatedAt: r.Now, UpdatedAt: r.Now,
		}
		if g.month >= current {
			if term := billingTermFor(terms[g.trainee], g.month); billable(term) {
				due := models.Cents(term.MonthlyFee)
				if g.cents <= due {
					ch2.DueCents, ch2.Source = due, models.ChargeSourceNormal
				} else {
					r.Notef("%s %s: payments total %s, above the %s fee — imported as unverified for review",
						name, g.month, fmtAmt(models.Amount(g.cents)), fmtAmt(term.MonthlyFee))
				}
			} else {
				r.Notef("%s %s: has subscription payments but no active fee — imported as unverified for review", name, g.month)
			}
		}
		ch2.State = chargeState(ch2.DueCents, ch2.PaidCents, ch2.Source)
		if ch2.Source == models.ChargeSourceImported {
			res.Inc("charges_imported_unverified")
		} else {
			res.Inc("charges_from_payments")
		}
		if err := insertCharge(ch2); err != nil {
			return res, err
		}
	}

	// This month's dues for everyone billable who hasn't paid anything yet.
	for _, t := range trainees {
		if covered[t.ID.Hex()+"|"+current] {
			continue
		}
		term := billingTermFor(terms[t.ID], current)
		if !billable(term) {
			continue
		}
		ok, _, err := exists(t.ID, current)
		if err != nil {
			return res, err
		}
		if ok {
			res.Inc("charges_existing")
			continue
		}
		due := models.Cents(term.MonthlyFee)
		res.Inc("charges_current_month")
		if err := insertCharge(models.SubscriptionCharge{
			Trainee: t.ID, TraineeName: t.Name, PeriodMonth: current, DueCents: due,
			State: chargeState(due, 0, models.ChargeSourceNormal), Source: models.ChargeSourceNormal,
			CreatedAt: r.Now, UpdatedAt: r.Now,
		}); err != nil {
			return res, err
		}
	}
	return res, nil
}

func migrateReminderDueDay(ctx context.Context, r *migrate.Runner) (migrate.Result, error) {
	res := migrate.Result{}
	cur, err := r.Store.Coll(models.CollReminders).Find(ctx, bson.M{"dueDay": bson.M{"$in": bson.A{nil, ""}}})
	if err != nil {
		return res, err
	}
	var rs []models.Reminder
	if err := cur.All(ctx, &rs); err != nil {
		return res, err
	}
	for _, rem := range rs {
		res.Inc("reminders_dated")
		if r.DryRun {
			continue
		}
		day := models.DateKey(rem.DueDate)
		start, _ := models.ParseDay(day)
		if _, err := r.Store.Coll(models.CollReminders).UpdateOne(ctx, bson.M{"_id": rem.ID},
			bson.M{"$set": bson.M{"dueDay": day, "dueDate": start}}); err != nil {
			return res, err
		}
	}
	return res, nil
}

// Finding is one integrity problem found by VerifyIntegrity.
type Finding struct {
	Kind   string
	Detail string
}

// VerifyIntegrity cross-checks the projections against their sources: each
// charge's paid balance against its live payments, and each item's stock
// against its ledger. Read-only.
func VerifyIntegrity(ctx context.Context, store *db.Store) ([]Finding, error) {
	var out []Finding
	cur, err := store.Coll(models.CollCharges).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var charges []models.SubscriptionCharge
	if err := cur.All(ctx, &charges); err != nil {
		return nil, err
	}
	for _, ch := range charges {
		paid, err := livePaidCents(ctx, store, ch.Trainee, ch.PeriodMonth)
		if err != nil {
			return nil, err
		}
		if paid != ch.PaidCents {
			out = append(out, Finding{"charge_paid_mismatch", fmt.Sprintf("%s %s: charge says %s paid, payments total %s",
				ch.TraineeName, ch.PeriodMonth, fmtAmt(models.Amount(ch.PaidCents)), fmtAmt(models.Amount(paid)))})
		}
		if ch.PaidCents > ch.DueCents && ch.Source != models.ChargeSourceImported {
			out = append(out, Finding{"charge_overpaid", fmt.Sprintf("%s %s: paid %s above due %s",
				ch.TraineeName, ch.PeriodMonth, fmtAmt(models.Amount(ch.PaidCents)), fmtAmt(models.Amount(ch.DueCents)))})
		}
	}
	icur, err := store.Coll(models.CollInventory).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var items []models.InventoryItem
	if err := icur.All(ctx, &items); err != nil {
		return nil, err
	}
	for _, it := range items {
		agg, err := store.Coll(models.CollMovements).Aggregate(ctx, mongo.Pipeline{
			{{Key: "$match", Value: bson.M{"item": it.ID}}},
			{{Key: "$group", Value: bson.M{"_id": nil, "sum": bson.M{"$sum": "$delta"}, "n": bson.M{"$sum": 1}}}},
		})
		if err != nil {
			return nil, err
		}
		var rows []struct {
			Sum int `bson:"sum"`
			N   int `bson:"n"`
		}
		if err := agg.All(ctx, &rows); err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			out = append(out, Finding{"stock_no_ledger", it.Name + ": no stock ledger yet (run migrations)"})
			continue
		}
		if rows[0].Sum != it.Stock {
			out = append(out, Finding{"stock_mismatch", fmt.Sprintf("%s: stock %d but ledger adds up to %d", it.Name, it.Stock, rows[0].Sum)})
		}
	}
	// Each sale's running return totals match its return records.
	ragg, err := store.Coll(models.CollReturns).Aggregate(ctx, mongo.Pipeline{
		{{Key: "$group", Value: bson.M{"_id": "$sale", "qty": bson.M{"$sum": "$qty"}, "n": bson.M{"$sum": 1}}}},
	})
	if err != nil {
		return nil, err
	}
	var rsums []struct {
		Sale primitive.ObjectID `bson:"_id"`
		Qty  int                `bson:"qty"`
	}
	if err := ragg.All(ctx, &rsums); err != nil {
		return nil, err
	}
	returned := map[primitive.ObjectID]int{}
	for _, r := range rsums {
		returned[r.Sale] = r.Qty
	}
	scur, err := store.Coll(models.CollSales).Find(ctx, bson.M{"$or": bson.A{
		bson.M{"returnedQty": bson.M{"$gt": 0}}, bson.M{"_id": bson.M{"$in": keysOf(returned)}},
	}})
	if err != nil {
		return nil, err
	}
	var rsales []models.Sale
	if err := scur.All(ctx, &rsales); err != nil {
		return nil, err
	}
	for _, s := range rsales {
		if s.ReturnedQty != returned[s.ID] || s.ReturnedQty > s.Qty {
			out = append(out, Finding{"sale_returns_mismatch", fmt.Sprintf("sale %s (%s × %d): returnedQty %d but returns add up to %d",
				s.ID.Hex(), s.ItemName, s.Qty, s.ReturnedQty, returned[s.ID])})
		}
	}
	return out, nil
}

func keysOf[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
