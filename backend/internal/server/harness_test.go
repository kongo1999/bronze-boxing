package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"

	"bronzeboxing/internal/config"
	"bronzeboxing/internal/db"
	"bronzeboxing/internal/migrate"
)

// Integration tests run the real HTTP handlers against a real MongoDB
// replica set (transactions included). Point TEST_MONGODB_URI at the
// disposable one from docker-compose.test.yml:
//
//	docker compose -f docker-compose.test.yml up -d --wait
//	TEST_MONGODB_URI="mongodb://localhost:27027/?replicaSet=rs0" go test ./...
//
// Each test gets its own freshly named database (prefix bbtest_) and drops
// only that database when it finishes. Without the variable they skip, so
// `go test ./...` stays green on a machine with no database.

const testDBPrefix = "bbtest_"

func TestMain(m *testing.M) {
	loc, err := time.LoadLocation("Asia/Beirut")
	if err != nil {
		panic(err)
	}
	time.Local = loc
	os.Exit(m.Run())
}

type testEnv struct {
	t     *testing.T
	store *db.Store
	app   *fiber.App
	ctx   context.Context
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	uri := os.Getenv("TEST_MONGODB_URI")
	if uri == "" {
		t.Skip("integration test: set TEST_MONGODB_URI (docker compose -f docker-compose.test.yml up -d --wait)")
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	name := fmt.Sprintf("%s%d_%s", testDBPrefix, time.Now().UnixNano(), hex.EncodeToString(b))
	store, err := db.Connect(uri, name)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if !store.SupportsTx {
		t.Fatalf("TEST_MONGODB_URI must point at a replica set (transactions are under test)")
	}
	ctx := context.Background()
	t.Cleanup(func() {
		failpoint = func(string) error { return nil }
		if strings.HasPrefix(store.DB.Name(), testDBPrefix) { // only ever drop our own
			_ = store.DB.Drop(context.Background())
		}
		_ = store.Disconnect(context.Background())
	})
	r := &migrate.Runner{Store: store}
	if err := r.Run(ctx, Migrations(), migrate.Options{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	app := New(config.Config{CORSOrigins: "*", Quiet: true}, store)
	return &testEnv{t: t, store: store, app: app, ctx: ctx}
}

// req performs an HTTP request against the app and returns status + body.
func (e *testEnv) req(method, path string, body any) (int, []byte) {
	e.t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, "/api"+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	res, err := e.app.Test(req, 30_000)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, out
}

// ok performs a request that must succeed (2xx) and decodes the body.
func (e *testEnv) ok(method, path string, body any, out any) {
	e.t.Helper()
	st, raw := e.req(method, path, body)
	if st < 200 || st > 299 {
		e.t.Fatalf("%s %s: status %d: %s", method, path, st, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("%s %s: decode: %v (%s)", method, path, err, raw)
		}
	}
}

type apiErrBody struct {
	Error   string         `json:"error"`
	Code    string         `json:"code"`
	Field   string         `json:"field"`
	Details map[string]any `json:"details"`
}

// fail performs a request that must fail with the given status and code.
func (e *testEnv) fail(method, path string, body any, status int, code string) apiErrBody {
	e.t.Helper()
	st, raw := e.req(method, path, body)
	var eb apiErrBody
	_ = json.Unmarshal(raw, &eb)
	if st != status || (code != "" && eb.Code != code) {
		e.t.Fatalf("%s %s: got %d %s (%s), want %d %s", method, path, st, eb.Code, raw, status, code)
	}
	return eb
}

func (e *testEnv) count(coll string, filter bson.M) int64 {
	e.t.Helper()
	n, err := e.store.Coll(coll).CountDocuments(e.ctx, filter)
	if err != nil {
		e.t.Fatalf("count %s: %v", coll, err)
	}
	return n
}

// fixture writes a JSON sample of a response to docs/fixtures when
// UPDATE_FIXTURES=1, so the documented contract is what the API really
// returns rather than something typed by hand.
func (e *testEnv) fixture(name string, v any) {
	if os.Getenv("UPDATE_FIXTURES") != "1" {
		return
	}
	dir := filepath.Join("..", "..", "..", "docs", "fixtures")
	_ = os.MkdirAll(dir, 0o755)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	if err := os.WriteFile(filepath.Join(dir, name+".json"), buf.Bytes(), 0o644); err != nil {
		e.t.Fatalf("fixture %s: %v", name, err)
	}
}

// Small typed shapes for decoding responses.
type traineeOut struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	MonthlyFee   float64 `json:"monthlyFee"`
	Status       string  `json:"status"`
	FeeFromMonth string  `json:"feeFromMonth"`
}

type subRowOut struct {
	Trainee    traineeOut `json:"trainee"`
	ChargeID   string     `json:"chargeId"`
	Due        float64    `json:"due"`
	AmountPaid float64    `json:"amountPaid"`
	Remaining  float64    `json:"remaining"`
	State      string     `json:"state"`
	Source     string     `json:"source"`
	Projected  bool       `json:"projected"`
}

type paymentOut struct {
	ID          string  `json:"id"`
	Amount      float64 `json:"amount"`
	Type        string  `json:"type"`
	PeriodMonth string  `json:"periodMonth"`
	Date        string  `json:"date"`
	VoidedAt    *string `json:"voidedAt"`
	VoidReason  string  `json:"voidReason"`
	CreatedBy   string  `json:"createdBy"`
}

func (e *testEnv) addTrainee(name string, fee float64) traineeOut {
	e.t.Helper()
	var t traineeOut
	e.ok("POST", "/trainees", map[string]any{"name": name, "monthlyFee": fee}, &t)
	return t
}

func (e *testEnv) dues(month string) map[string]subRowOut {
	e.t.Helper()
	var rows []subRowOut
	e.ok("GET", "/subscriptions?m="+month, nil, &rows)
	out := map[string]subRowOut{}
	for _, r := range rows {
		out[r.Trainee.ID] = r
	}
	return out
}

func (e *testEnv) pay(trainee string, amount float64, month string) paymentOut {
	e.t.Helper()
	var p paymentOut
	e.ok("POST", "/payments", map[string]any{"trainee": trainee, "amount": amount, "type": "subscription", "periodMonth": month}, &p)
	return p
}
