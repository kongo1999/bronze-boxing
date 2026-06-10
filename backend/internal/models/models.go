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
	MonthlyFee float64            `bson:"monthlyFee" json:"monthlyFee"`
	Status     string             `bson:"status" json:"status"`
	Notes      string             `bson:"notes,omitempty" json:"notes,omitempty"`
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time          `bson:"updatedAt" json:"updatedAt"`
}

type Attendee struct {
	Trainee     primitive.ObjectID `bson:"trainee" json:"trainee"`
	TraineeName string             `bson:"traineeName,omitempty" json:"traineeName,omitempty"`
	Status      string             `bson:"status" json:"status"`
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
	SaleID      *primitive.ObjectID `bson:"saleId,omitempty" json:"saleId,omitempty"`
	CreatedAt   time.Time           `bson:"createdAt" json:"createdAt"`
}

type Reminder struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title     string             `bson:"title" json:"title"`
	DueDate   time.Time          `bson:"dueDate" json:"dueDate"`
	Priority  string             `bson:"priority" json:"priority"`
	Done      bool               `bson:"done" json:"done"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

type Expense struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Amount    float64            `bson:"amount" json:"amount"`
	Category  string             `bson:"category" json:"category"`
	Note      string             `bson:"note,omitempty" json:"note,omitempty"`
	Date      time.Time          `bson:"date" json:"date"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
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

type Sale struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Item        primitive.ObjectID  `bson:"item" json:"item"`
	ItemName    string              `bson:"itemName" json:"itemName"`
	Trainee     *primitive.ObjectID `bson:"trainee,omitempty" json:"trainee,omitempty"`
	TraineeName string              `bson:"traineeName,omitempty" json:"traineeName,omitempty"`
	Qty         int                 `bson:"qty" json:"qty"`
	UnitPrice   float64             `bson:"unitPrice" json:"unitPrice"`
	Total       float64             `bson:"total" json:"total"`
	Date        time.Time           `bson:"date" json:"date"`
	PaymentID   *primitive.ObjectID `bson:"paymentId,omitempty" json:"paymentId,omitempty"`
	CreatedAt   time.Time           `bson:"createdAt" json:"createdAt"`
}
