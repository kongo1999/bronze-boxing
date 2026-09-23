package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Enum-ish string values (kept as strings for Mongo friendliness).
const (
	SkillBeginner     = "beginner"
	SkillIntermediate = "intermediate"
	SkillAdvanced     = "advanced"

	StatusActive   = "active"
	StatusInactive = "inactive"

	SessionGroup   = "group"
	SessionPrivate = "private"

	SessScheduled = "scheduled"
	SessCompleted = "completed"
	SessCancelled = "cancelled"

	AttendBooked   = "booked"
	AttendAttended = "attended"
	AttendNoShow   = "no_show"

	PaySubscription = "subscription"
	PayPrivate      = "private"
	PayDropin       = "dropin"
	PaySale         = "sale"
	PayOther        = "other"

	// How money was received. Payments recorded before methods existed have
	// none and read as "unspecified".
	MethodCash     = "cash"
	MethodCard     = "card"
	MethodTransfer = "bank_transfer"
	MethodOther    = "other"

	ExpRent      = "rent"
	ExpEquipment = "equipment"
	ExpUtilities = "utilities"
	ExpSupplies  = "supplies"
	ExpWages     = "wages"
	ExpOther     = "other"

	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"

	RoleAdmin = "admin"

	// Stock movement kinds: every change to an item's stock is one of these.
	MoveOpening    = "opening"    // ledger start: the stock on hand when tracking began
	MoveSale       = "sale"       // sold (negative)
	MoveReturn     = "return"     // customer returned units (positive)
	MoveRestock    = "restock"    // new stock delivered (positive)
	MoveDamage     = "damage"     // written off: damaged / lost (negative)
	MoveCorrection = "correction" // counted stock differs from the ledger (either sign)
	MoveSaleEdit   = "sale_edit"  // a sale's quantity was corrected (either sign)
	MoveSaleVoid   = "sale_void"  // an erroneous sale was voided (positive)

	// Subscription charge states. "upcoming" is derived at read time for a
	// future month with nothing paid; it is never stored.
	ChargeUnpaid     = "unpaid"
	ChargePartial    = "partial"
	ChargePaid       = "paid"
	ChargeWaived     = "waived"
	ChargeUnverified = "unverified"
	ChargeUpcoming   = "upcoming"

	// Where a charge's due amount came from.
	ChargeSourceNormal   = "normal"              // materialized from the trainee's terms
	ChargeSourceAdjusted = "adjusted"            // changed by an explicit, audited adjustment
	ChargeSourceImported = "imported_unverified" // migrated from pre-charge data: due unknown until reconciled
)

// Collection names.
const (
	CollTrainees     = "trainees"
	CollSessions     = "sessions"
	CollPayments     = "payments"
	CollReminders    = "reminders"
	CollExpenses     = "expenses"
	CollInventory    = "inventory"
	CollSales        = "sales"
	CollUsers        = "users"
	CollAuthSessions = "auth_sessions" // login sessions, not training sessions
	CollAudit        = "audit_log"     // append-only money audit trail
	CollMovements    = "stock_movements"
	CollTerms        = "subscription_terms"
	CollCharges      = "subscription_charges"
	CollClosings     = "cash_closings"
	CollReturns      = "sale_returns"
	CollSeries       = "session_series"
	CollPlans        = "trainee_session_plans"
)

// User is a staff login account. The admin account is bootstrapped from
// ADMIN_USERNAME / ADMIN_PASSWORD env config on server start.
type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Username     string             `bson:"username" json:"username"`
	PasswordHash string             `bson:"passwordHash" json:"-"`
	Role         string             `bson:"role" json:"role"`
	CreatedAt    time.Time          `bson:"createdAt" json:"createdAt"`
}

// AuthSession is a server-side login session. The client holds the raw token;
// we store only its SHA-256, and Mongo's TTL monitor purges expired rows.
type AuthSession struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	TokenHash string             `bson:"tokenHash"`
	User      primitive.ObjectID `bson:"user"`
	Username  string             `bson:"username"`
	ExpiresAt time.Time          `bson:"expiresAt"`
	CreatedAt time.Time          `bson:"createdAt"`
}

type Trainee struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name       string             `bson:"name" json:"name"`
	Phone      string             `bson:"phone,omitempty" json:"phone,omitempty"`
	SkillLevel string             `bson:"skillLevel,omitempty" json:"skillLevel,omitempty"`
	// MonthlyFee and Status summarize the most recently recorded terms — the
	// fee going forward. Historical dues come from subscription_terms and
	// subscription_charges, never from these fields.
	MonthlyFee float64 `bson:"monthlyFee" json:"monthlyFee"`
	Status     string  `bson:"status" json:"status"`
	// FeeFromMonth is the first month billed at MonthlyFee (so the UI can say
	// "$120/mo from October" when a change is scheduled).
	FeeFromMonth string `bson:"feeFromMonth,omitempty" json:"feeFromMonth,omitempty"`
	Notes        string `bson:"notes,omitempty" json:"notes,omitempty"`
	// ArchivedAt hides a former trainee from the everyday roster while every
	// payment, session and sale that names them stays linked and visible.
	ArchivedAt *time.Time `bson:"archivedAt,omitempty" json:"archivedAt,omitempty"`
	CreatedAt  time.Time  `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time  `bson:"updatedAt" json:"updatedAt"`
}

type Attendee struct {
	Trainee     primitive.ObjectID `bson:"trainee" json:"trainee"`
	TraineeName string             `bson:"traineeName,omitempty" json:"traineeName,omitempty"`
	Status      string             `bson:"status" json:"status"`
	// PlanID credits this booking to one of the trainee's session plans. A
	// booking credits at most one plan; it counts toward it only once the
	// session is completed AND this attendee is marked attended.
	PlanID *primitive.ObjectID `bson:"planId,omitempty" json:"planId,omitempty"`
}

type Session struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title       string             `bson:"title" json:"title"`
	Type        string             `bson:"type" json:"type"`
	Start       time.Time          `bson:"start" json:"start"`
	DurationMin int                `bson:"durationMin" json:"durationMin"`
	Location    string             `bson:"location,omitempty" json:"location,omitempty"`
	Capacity    int                `bson:"capacity,omitempty" json:"capacity,omitempty"`
	Fee         float64            `bson:"fee,omitempty" json:"fee,omitempty"`
	SeriesID    string             `bson:"seriesId,omitempty" json:"seriesId,omitempty"`
	Status      string             `bson:"status" json:"status"`
	Attendees   []Attendee         `bson:"attendees" json:"attendees"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time          `bson:"updatedAt" json:"updatedAt"`
}

type Payment struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Trainee     *primitive.ObjectID `bson:"trainee,omitempty" json:"trainee,omitempty"`
	TraineeName string              `bson:"traineeName,omitempty" json:"traineeName,omitempty"`
	Amount      float64             `bson:"amount" json:"amount"`
	Type        string              `bson:"type" json:"type"`
	PeriodMonth string              `bson:"periodMonth,omitempty" json:"periodMonth,omitempty"`
	Date        time.Time           `bson:"date" json:"date"`
	Note        string              `bson:"note,omitempty" json:"note,omitempty"`
	Method      string              `bson:"method,omitempty" json:"method,omitempty"`       // cash | card | bank_transfer | other
	Reference   string              `bson:"reference,omitempty" json:"reference,omitempty"` // card slip / transfer ref
	SaleID      *primitive.ObjectID `bson:"saleId,omitempty" json:"saleId,omitempty"`
	CreatedAt   time.Time           `bson:"createdAt" json:"createdAt"`
	CreatedBy   string              `bson:"createdBy,omitempty" json:"createdBy,omitempty"`
	VoidedAt    *time.Time          `bson:"voidedAt,omitempty" json:"voidedAt,omitempty"`
	VoidReason  string              `bson:"voidReason,omitempty" json:"voidReason,omitempty"`
	VoidedBy    string              `bson:"voidedBy,omitempty" json:"voidedBy,omitempty"`
}

type Reminder struct {
	ID    primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title string             `bson:"title" json:"title"`
	// DueDay is the studio-local calendar day the reminder is for. DueDate is
	// the instant that day starts, kept for range queries and old clients.
	// Reminders written before DueDay existed get it derived on read.
	DueDay   string     `bson:"dueDay,omitempty" json:"dueDay"`
	DueDate  time.Time  `bson:"dueDate" json:"dueDate"`
	Priority string     `bson:"priority" json:"priority"`
	Done     bool       `bson:"done" json:"done"`
	DoneAt   *time.Time `bson:"doneAt,omitempty" json:"doneAt,omitempty"`
	// SnoozedUntil hides the reminder until that studio-local day without
	// rewriting when it was originally due.
	SnoozedUntil string `bson:"snoozedUntil,omitempty" json:"snoozedUntil,omitempty"`
	// Optional link to the record it is about.
	RelatedType  string              `bson:"relatedType,omitempty" json:"relatedType,omitempty"` // trainee | session | item
	RelatedID    *primitive.ObjectID `bson:"relatedId,omitempty" json:"relatedId,omitempty"`
	RelatedLabel string              `bson:"relatedLabel,omitempty" json:"relatedLabel,omitempty"`
	// Recurrence repeats the reminder: completing one instance creates the
	// next. Instances of one chain share SeriesID; NextID points forward.
	Recurrence string              `bson:"recurrence,omitempty" json:"recurrence,omitempty"` // daily | weekly | monthly
	SeriesID   string              `bson:"seriesId,omitempty" json:"seriesId,omitempty"`
	NextID     *primitive.ObjectID `bson:"nextId,omitempty" json:"nextId,omitempty"`
	CreatedAt  time.Time           `bson:"createdAt" json:"createdAt"`
}

type Expense struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Amount     float64            `bson:"amount" json:"amount"`
	Category   string             `bson:"category" json:"category"`
	Note       string             `bson:"note,omitempty" json:"note,omitempty"`
	Date       time.Time          `bson:"date" json:"date"`
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
	CreatedBy  string             `bson:"createdBy,omitempty" json:"createdBy,omitempty"`
	VoidedAt   *time.Time         `bson:"voidedAt,omitempty" json:"voidedAt,omitempty"`
	VoidReason string             `bson:"voidReason,omitempty" json:"voidReason,omitempty"`
	VoidedBy   string             `bson:"voidedBy,omitempty" json:"voidedBy,omitempty"`
}

type InventoryItem struct {
	ID                primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name              string             `bson:"name" json:"name"`
	SKU               string             `bson:"sku,omitempty" json:"sku,omitempty"`
	Stock             int                `bson:"stock" json:"stock"`
	Price             float64            `bson:"price" json:"price"`
	CostPrice         float64            `bson:"costPrice,omitempty" json:"costPrice,omitempty"`
	LowStockThreshold int                `bson:"lowStockThreshold,omitempty" json:"lowStockThreshold,omitempty"`
	Active            bool               `bson:"active" json:"active"`
	CreatedAt         time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// Shortage says whether an item needs restocking: "out" when none is left,
// "low" at or under its threshold (when one is set), otherwise "".
func (i InventoryItem) Shortage() string {
	switch {
	case i.Stock <= 0:
		return "out"
	case i.LowStockThreshold > 0 && i.Stock <= i.LowStockThreshold:
		return "low"
	}
	return ""
}

type Sale struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Item        primitive.ObjectID  `bson:"item" json:"item"`
	ItemName    string              `bson:"itemName" json:"itemName"`
	Trainee     *primitive.ObjectID `bson:"trainee,omitempty" json:"trainee,omitempty"`
	TraineeName string              `bson:"traineeName,omitempty" json:"traineeName,omitempty"`
	Qty         int                 `bson:"qty" json:"qty"`
	UnitPrice   float64             `bson:"unitPrice" json:"unitPrice"`
	// UnitCost snapshots the item's cost price at the moment of sale, so
	// margins on old sales never use today's cost.
	UnitCost float64 `bson:"unitCost,omitempty" json:"unitCost,omitempty"`
	// ListPrice is the item's price at the time of sale; when UnitPrice
	// differs (a discount or override), PriceReason says why.
	ListPrice   float64 `bson:"listPrice,omitempty" json:"listPrice,omitempty"`
	PriceReason string  `bson:"priceReason,omitempty" json:"priceReason,omitempty"`
	Method      string  `bson:"method,omitempty" json:"method,omitempty"` // how the buyer paid; cash when unset
	Total       float64 `bson:"total" json:"total"`
	// Running totals of live partial returns against this sale. The sale
	// itself is never rewritten by a return; returns are their own records.
	ReturnedQty   int                 `bson:"returnedQty,omitempty" json:"returnedQty,omitempty"`
	ReturnedTotal float64             `bson:"returnedTotal,omitempty" json:"returnedTotal,omitempty"`
	Date          time.Time           `bson:"date" json:"date"`
	PaymentID     *primitive.ObjectID `bson:"paymentId,omitempty" json:"paymentId,omitempty"`
	CreatedAt     time.Time           `bson:"createdAt" json:"createdAt"`
	CreatedBy     string              `bson:"createdBy,omitempty" json:"createdBy,omitempty"`
	VoidedAt      *time.Time          `bson:"voidedAt,omitempty" json:"voidedAt,omitempty"`
	VoidReason    string              `bson:"voidReason,omitempty" json:"voidReason,omitempty"`
	VoidedBy      string              `bson:"voidedBy,omitempty" json:"voidedBy,omitempty"`
}

// AuditEntry is one immutable line in the money audit trail: every creation,
// edit, void or adjustment of a payment, expense, sale or charge records what
// it looked like before and after, who did it, why, and when. Entries are
// append-only — nothing updates or deletes them — and are written in the same
// transaction as the change they describe, so a change cannot land without
// its line. Lines written before actor/reason existed simply lack them.
type AuditEntry struct {
	ID     primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Entity string             `bson:"entity" json:"entity"` // payment | expense | sale | charge | item | trainee | series
	Ref    primitive.ObjectID `bson:"ref" json:"ref"`
	Action string             `bson:"action" json:"action"` // create | update | void | adjust | return | …
	Before any                `bson:"before,omitempty" json:"before,omitempty"`
	After  any                `bson:"after,omitempty" json:"after,omitempty"`
	Actor  string             `bson:"actor,omitempty" json:"actor,omitempty"`
	Reason string             `bson:"reason,omitempty" json:"reason,omitempty"`
	At     time.Time          `bson:"at" json:"at"`
}

// StockMovement is one line of an item's stock ledger. The sum of an item's
// movements equals its stock, so every unit that arrives or leaves is
// explained. Append-only.
type StockMovement struct {
	ID       primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Item     primitive.ObjectID  `bson:"item" json:"item"`
	ItemName string              `bson:"itemName" json:"itemName"`
	Delta    int                 `bson:"delta" json:"delta"`
	Kind     string              `bson:"kind" json:"kind"`
	Reason   string              `bson:"reason,omitempty" json:"reason,omitempty"`
	RefType  string              `bson:"refType,omitempty" json:"refType,omitempty"` // sale | return
	Ref      *primitive.ObjectID `bson:"ref,omitempty" json:"ref,omitempty"`
	// StockAfter is the item's stock once this movement applied.
	StockAfter int       `bson:"stockAfter" json:"stockAfter"`
	At         time.Time `bson:"at" json:"at"`
	Actor      string    `bson:"actor,omitempty" json:"actor,omitempty"`
}

// SubscriptionTerm is one effective-dated membership record: from
// EffectiveDate the trainee has this status, and from BillingFromMonth their
// monthly charge is MonthlyFee. Terms are append-only — a fee or status change
// adds a term, it never edits an earlier one — so "what was owed in March"
// and "who was active at the end of March" stay answerable.
type SubscriptionTerm struct {
	ID               primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Trainee          primitive.ObjectID `bson:"trainee" json:"trainee"`
	EffectiveDate    string             `bson:"effectiveDate" json:"effectiveDate"`       // studio-local YYYY-MM-DD
	BillingFromMonth string             `bson:"billingFromMonth" json:"billingFromMonth"` // YYYY-MM
	MonthlyFee       float64            `bson:"monthlyFee" json:"monthlyFee"`
	Status           string             `bson:"status" json:"status"`
	// Source is "migrated" for the term created from the pre-terms trainee
	// record: its effective date is the trainee's creation date, which is an
	// inference, not a recorded fact.
	Source    string    `bson:"source,omitempty" json:"source,omitempty"`
	Reason    string    `bson:"reason,omitempty" json:"reason,omitempty"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	Actor     string    `bson:"actor,omitempty" json:"actor,omitempty"`
}

// SubscriptionCharge is what one trainee owes for one month: a fixed record,
// created once from the terms in force for that month and never silently
// re-priced. PaidCents is a balance projection maintained in the same
// transaction as every subscription payment, void and correction; it is the
// guard that makes overpayment impossible even under concurrent requests.
// Amounts are integer cents so the guard compares exactly.
type SubscriptionCharge struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Trainee     primitive.ObjectID `bson:"trainee" json:"trainee"`
	TraineeName string             `bson:"traineeName" json:"traineeName"`
	PeriodMonth string             `bson:"periodMonth" json:"periodMonth"`
	DueCents    int64              `bson:"dueCents" json:"-"`
	PaidCents   int64              `bson:"paidCents" json:"-"`
	State       string             `bson:"state" json:"state"`
	Source      string             `bson:"source" json:"source"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// CashClosing is the cash the owner counted in the till at the end of a
// studio day, compared against the cash the books say came in.
type CashClosing struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Day          string             `bson:"day" json:"day"` // studio-local YYYY-MM-DD, unique
	CountedCents int64              `bson:"countedCents" json:"-"`
	Note         string             `bson:"note,omitempty" json:"note,omitempty"`
	Actor        string             `bson:"actor,omitempty" json:"actor,omitempty"`
	CreatedAt    time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// SaleReturn is units of a sale coming back, with the money refunded. The
// sale itself is not rewritten: a return is its own record, dated when the
// refund happened, so it lands in that month's cash. A return only counts
// while its sale is live — voiding the sale voids the whole transaction.
type SaleReturn struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Sale      primitive.ObjectID `bson:"sale" json:"sale"`
	Item      primitive.ObjectID `bson:"item" json:"item"`
	ItemName  string             `bson:"itemName" json:"itemName"`
	Qty       int                `bson:"qty" json:"qty"`
	Amount    float64            `bson:"amount" json:"amount"` // refunded
	Date      time.Time          `bson:"date" json:"date"`
	Reason    string             `bson:"reason" json:"reason"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	Actor     string             `bson:"actor,omitempty" json:"actor,omitempty"`
}

// SessionSeries is the record of a recurring series: what was asked for and,
// crucially, how many occurrences were planned — the denominator of
// "9/12 completed" — which must not shrink when an occurrence is cancelled.
// It changes only through an explicit, audited extension or reduction.
type SessionSeries struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	SeriesID     string             `bson:"seriesId" json:"seriesId"`
	Title        string             `bson:"title" json:"title"`
	Type         string             `bson:"type" json:"type"`
	Weekdays     []int              `bson:"weekdays,omitempty" json:"weekdays,omitempty"`
	Time         string             `bson:"time,omitempty" json:"time,omitempty"`
	DurationMin  int                `bson:"durationMin,omitempty" json:"durationMin,omitempty"`
	FromDay      string             `bson:"fromDay" json:"fromDay"`
	ToDay        string             `bson:"toDay" json:"toDay"`
	PlannedCount int                `bson:"plannedCount" json:"plannedCount"`
	Status       string             `bson:"status" json:"status"` // active | ended
	// Inferred marks a series rebuilt from existing occurrences by a
	// migration: its planned count is what was still stored, not what was
	// originally asked for.
	Inferred  bool      `bson:"inferred,omitempty" json:"inferred,omitempty"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// SessionPlan is a trainee's session allowance, e.g. "12 private sessions".
// Progress is never stored: it is computed from the sessions whose attendee
// entry for this trainee points at the plan, so repeated updates, moves and
// reopened sessions can't double count.
type SessionPlan struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Trainee     primitive.ObjectID `bson:"trainee" json:"trainee"`
	TraineeName string             `bson:"traineeName" json:"traineeName"`
	Title       string             `bson:"title" json:"title"`
	TargetCount int                `bson:"targetCount" json:"targetCount"`
	StartDate   string             `bson:"startDate" json:"startDate"`                 // studio day
	EndDate     string             `bson:"endDate,omitempty" json:"endDate,omitempty"` // optional, inclusive
	SessionType string             `bson:"sessionType,omitempty" json:"sessionType,omitempty"`
	Status      string             `bson:"status" json:"status"` // active | completed | cancelled
	Notes       string             `bson:"notes,omitempty" json:"notes,omitempty"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time          `bson:"updatedAt" json:"updatedAt"`
}
