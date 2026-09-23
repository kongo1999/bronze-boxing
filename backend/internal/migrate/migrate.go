// Package migrate runs ordered, idempotent data migrations.
//
// Schema and data changes never happen inside a request handler: they live
// here, run explicitly (cmd/migrate), support a dry run that writes nothing,
// report progress counts, and record what was applied in schema_migrations so
// a second run is a no-op. Every migration must also be safe to re-run by
// hand (-rerun), because that is how idempotency is verified on a copy of
// real data before release.
package migrate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
)

// Collection that records applied migrations.
const Coll = "schema_migrations"

// Result is a migration's progress counts ("created": 12, "skipped": 3, …).
type Result map[string]int

func (r Result) Add(key string, n int) { r[key] += n }
func (r Result) Inc(key string)        { r[key]++ }

func (r Result) String() string {
	if len(r) == 0 {
		return "(no changes)"
	}
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, r[k])
	}
	return strings.Join(parts, " ")
}

// Migration is one ordered step. Up must check r.DryRun before every write and
// must be idempotent: running it against already-migrated data changes nothing.
type Migration struct {
	ID          string
	Description string
	Up          func(ctx context.Context, r *Runner) (Result, error)
}

// Runner carries what a migration needs.
type Runner struct {
	Store  *db.Store
	DryRun bool
	Logf   func(format string, args ...any)
	// Now is the migration's notion of "now" (fixed per run so every record a
	// run writes agrees on it). Defaults to time.Now.
	Now time.Time
	// Notes collects human-readable findings (data conflicts, rows needing
	// review) that the operator must see; printed after the run.
	Notes []string
}

func (r *Runner) Notef(format string, args ...any) {
	r.Notes = append(r.Notes, fmt.Sprintf(format, args...))
}

type applied struct {
	ID          string    `bson:"_id"`
	Description string    `bson:"description"`
	AppliedAt   time.Time `bson:"appliedAt"`
	Result      Result    `bson:"result"`
}

// Applied returns the set of migration IDs already recorded.
func Applied(ctx context.Context, store *db.Store) (map[string]time.Time, error) {
	cur, err := store.Coll(Coll).Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var rows []applied
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make(map[string]time.Time, len(rows))
	for _, a := range rows {
		out[a.ID] = a.AppliedAt
	}
	return out, nil
}

// Pending lists the migrations not yet applied, in order.
func Pending(ctx context.Context, store *db.Store, all []Migration) ([]Migration, error) {
	done, err := Applied(ctx, store)
	if err != nil {
		return nil, err
	}
	var out []Migration
	for _, m := range all {
		if _, ok := done[m.ID]; !ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// Options select what Run executes.
type Options struct {
	Only  string // run just this ID
	Rerun bool   // run even if already applied (idempotency check)
}

// Run applies pending migrations in order (or the selected one), stopping at
// the first error. In a dry run nothing is written, including the record.
func (r *Runner) Run(ctx context.Context, all []Migration, o Options) error {
	if r.Logf == nil {
		r.Logf = func(string, ...any) {}
	}
	if r.Now.IsZero() {
		r.Now = time.Now()
	}
	done, err := Applied(ctx, r.Store)
	if err != nil {
		return err
	}
	ran := 0
	for _, m := range all {
		if o.Only != "" && m.ID != o.Only {
			continue
		}
		if _, ok := done[m.ID]; ok && !o.Rerun {
			r.Logf("  = %s already applied", m.ID)
			continue
		}
		mode := "apply"
		if r.DryRun {
			mode = "dry-run"
		}
		r.Logf("  > %s [%s] %s", m.ID, mode, m.Description)
		start := time.Now()
		res, err := m.Up(ctx, r)
		if err != nil {
			return fmt.Errorf("migration %s: %w", m.ID, err)
		}
		if res == nil {
			res = Result{}
		}
		r.Logf("    %s (%s)", res, time.Since(start).Round(time.Millisecond))
		if !r.DryRun {
			if _, err := r.Store.Coll(Coll).UpdateOne(ctx, bson.M{"_id": m.ID}, bson.M{"$set": applied{
				ID: m.ID, Description: m.Description, AppliedAt: r.Now, Result: res,
			}}, options.Update().SetUpsert(true)); err != nil {
				return fmt.Errorf("record %s: %w", m.ID, err)
			}
		}
		ran++
	}
	if o.Only != "" && ran == 0 {
		if _, ok := done[o.Only]; !ok {
			return fmt.Errorf("unknown migration %q", o.Only)
		}
	}
	return nil
}

// EnsureIndexes creates indexes idempotently, honouring dry-run. Returns how
// many index specs were requested.
func (r *Runner) EnsureIndexes(ctx context.Context, coll string, models []mongo.IndexModel) (int, error) {
	if r.DryRun {
		return len(models), nil
	}
	if _, err := r.Store.Coll(coll).Indexes().CreateMany(ctx, models); err != nil {
		return 0, fmt.Errorf("indexes on %s: %w", coll, err)
	}
	return len(models), nil
}
