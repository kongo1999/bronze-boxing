// Seed the database with realistic sample data for development/demo.
// Run with: go run ./cmd/seed
//
// WARNING: clears trainees, sessions, payments, reminders, expenses,
// inventory and sales collections first.
package main

import (
	"context"
	"log"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bronzeboxing/internal/config"
	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

func at(day time.Time, h, m int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, time.Local)
}

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	store, err := db.Connect(cfg.MongoURI, cfg.DBName)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	ctx := context.Background()

	log.Println("clearing collections...")
	for _, c := range []string{
		models.CollTrainees, models.CollSessions, models.CollPayments,
		models.CollReminders, models.CollExpenses, models.CollInventory, models.CollSales,
	} {
		if _, err := store.Coll(c).DeleteMany(ctx, bson.M{}); err != nil {
			log.Fatalf("clear %s: %v", c, err)
		}
	}

	now := time.Now()
	month := models.MonthKey(now)

	// Monday of the current week.
	weekday := int(now.Weekday()) // Sun=0
	offset := (weekday + 6) % 7    // days since Monday
	weekStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -offset)

	log.Println("creating trainees...")
	trainees := []any{
		models.Trainee{Name: "Karim Haddad", Phone: "03 111 222", SkillLevel: "intermediate", MonthlyFee: 120, Status: "active", Notes: "Southpaw. Working on the jab.", CreatedAt: now, UpdatedAt: now},
		models.Trainee{Name: "Rami Khoury", Phone: "03 333 444", SkillLevel: "beginner", MonthlyFee: 100, Status: "active", CreatedAt: now, UpdatedAt: now},
		models.Trainee{Name: "Jad Saliba", Phone: "70 555 666", SkillLevel: "advanced", MonthlyFee: 120, Status: "active", Notes: "Prepping for amateur bout.", CreatedAt: now, UpdatedAt: now},
		models.Trainee{Name: "Nour Aoun", Phone: "76 777 888", SkillLevel: "beginner", MonthlyFee: 80, Status: "active", CreatedAt: now, UpdatedAt: now},
		models.Trainee{Name: "Tarek Mansour", Phone: "71 999 000", SkillLevel: "intermediate", MonthlyFee: 100, Status: "active", CreatedAt: now, UpdatedAt: now},
		models.Trainee{Name: "Sami Dabboussi", Phone: "03 121 212", SkillLevel: "beginner", MonthlyFee: 0, Status: "active", Notes: "Drop-in only.", CreatedAt: now, UpdatedAt: now},
		models.Trainee{Name: "Lara Fares", Phone: "78 343 434", SkillLevel: "intermediate", MonthlyFee: 90, Status: "active", CreatedAt: now, UpdatedAt: now},
		models.Trainee{Name: "Omar Zein", Phone: "70 565 656", SkillLevel: "beginner", MonthlyFee: 0, Status: "inactive", Notes: "On a break.", CreatedAt: now, UpdatedAt: now},
	}
	tres, err := store.Coll(models.CollTrainees).InsertMany(ctx, trainees)
	if err != nil {
		log.Fatalf("trainees: %v", err)
	}
	id := func(i int) primitive.ObjectID { return tres.InsertedIDs[i].(primitive.ObjectID) }
	karim, rami, jad, nour, tarek, sami, lara := id(0), id(1), id(2), id(3), id(4), id(5), id(6)
	name := []string{"Karim Haddad", "Rami Khoury", "Jad Saliba", "Nour Aoun", "Tarek Mansour", "Sami Dabboussi", "Lara Fares"}

	log.Println("creating sessions...")
	seriesID := primitive.NewObjectID().Hex()
	var sessions []any
	for _, off := range []int{0, 2, 4} { // Mon, Wed, Fri
		day := weekStart.AddDate(0, 0, off)
		start := at(day, 18, 0)
		status := models.SessScheduled
		past := start.Before(now)
		if past {
			status = models.SessCompleted
		}
		att := []models.Attendee{}
		for i, tid := range []primitive.ObjectID{karim, rami, jad, nour, lara} {
			st := models.AttendBooked
			if past {
				st = models.AttendAttended
				if i == 1 {
					st = models.AttendNoShow
				}
			}
			nm := []string{"Karim Haddad", "Rami Khoury", "Jad Saliba", "Nour Aoun", "Lara Fares"}[i]
			att = append(att, models.Attendee{Trainee: tid, TraineeName: nm, Status: st})
		}
		sessions = append(sessions, models.Session{
			Title: "Evening Group Class", Type: "group", Start: start, DurationMin: 60,
			Location: "Main floor", Capacity: 12, SeriesID: seriesID, Status: status,
			Attendees: att, CreatedAt: now, UpdatedAt: now,
		})
	}
	morningStatus := models.SessScheduled
	if now.After(at(now, 8, 20)) {
		morningStatus = models.SessCompleted
	}
	sessions = append(sessions,
		models.Session{Title: "Morning Conditioning", Type: "group", Start: at(now, 7, 30), DurationMin: 50,
			Location: "Main floor", Capacity: 10, Status: morningStatus, Attendees: []models.Attendee{
				{Trainee: nour, TraineeName: name[3], Status: "booked"},
				{Trainee: tarek, TraineeName: name[4], Status: "booked"},
				{Trainee: sami, TraineeName: name[5], Status: "booked"},
			}, CreatedAt: now, UpdatedAt: now},
		models.Session{Title: "Private — Jad", Type: "private", Start: at(now, 16, 30), DurationMin: 45,
			Location: "Ring 1", Fee: 35, Status: "scheduled", Attendees: []models.Attendee{
				{Trainee: jad, TraineeName: name[2], Status: "booked"},
			}, CreatedAt: now, UpdatedAt: now},
		models.Session{Title: "Private — Lara", Type: "private", Start: at(now.AddDate(0, 0, 1), 17, 0), DurationMin: 45,
			Location: "Ring 1", Fee: 35, Status: "scheduled", Attendees: []models.Attendee{
				{Trainee: lara, TraineeName: name[6], Status: "booked"},
			}, CreatedAt: now, UpdatedAt: now},
	)
	if _, err := store.Coll(models.CollSessions).InsertMany(ctx, sessions); err != nil {
		log.Fatalf("sessions: %v", err)
	}

	log.Println("creating payments...")
	pay := func(t primitive.ObjectID, nm string, amt float64, typ, period, note string) models.Payment {
		tid := t
		return models.Payment{Trainee: &tid, TraineeName: nm, Amount: amt, Type: typ, PeriodMonth: period, Date: now, Note: note, CreatedAt: now}
	}
	payments := []any{
		pay(karim, name[0], 120, "subscription", month, ""),
		pay(nour, name[3], 80, "subscription", month, ""),
		pay(lara, name[6], 90, "subscription", month, ""),
		pay(rami, name[1], 50, "subscription", month, "Half now, half later"),
		pay(jad, name[2], 35, "private", "", ""),
		pay(sami, name[5], 20, "dropin", "", ""),
	}
	if _, err := store.Coll(models.CollPayments).InsertMany(ctx, payments); err != nil {
		log.Fatalf("payments: %v", err)
	}

	log.Println("creating reminders...")
	reminders := []any{
		models.Reminder{Title: "Fix the speed-bag bracket", DueDate: now.AddDate(0, 0, -1), Priority: "high", CreatedAt: now},
		models.Reminder{Title: "Call Rami about his schedule", DueDate: now, Priority: "normal", CreatedAt: now},
		models.Reminder{Title: "Order new gloves (size M)", DueDate: now.AddDate(0, 0, 2), Priority: "normal", CreatedAt: now},
		models.Reminder{Title: "Renew gym insurance", DueDate: now.AddDate(0, 0, 4), Priority: "high", CreatedAt: now},
		models.Reminder{Title: "Wipe down the ring canvas", DueDate: now.AddDate(0, 0, -2), Priority: "low", Done: true, CreatedAt: now},
	}
	if _, err := store.Coll(models.CollReminders).InsertMany(ctx, reminders); err != nil {
		log.Fatalf("reminders: %v", err)
	}

	log.Println("creating expenses...")
	expenses := []any{
		models.Expense{Amount: 800, Category: "rent", Note: "Gym space — monthly", Date: at(now.AddDate(0, 0, -now.Day()+3), 10, 0), CreatedAt: now},
		models.Expense{Amount: 240, Category: "equipment", Note: "Two pairs of focus mitts", Date: now.AddDate(0, 0, -5), CreatedAt: now},
		models.Expense{Amount: 95, Category: "utilities", Note: "Electricity", Date: now.AddDate(0, 0, -3), CreatedAt: now},
	}
	if _, err := store.Coll(models.CollExpenses).InsertMany(ctx, expenses); err != nil {
		log.Fatalf("expenses: %v", err)
	}

	log.Println("creating inventory...")
	items := []any{
		models.InventoryItem{Name: "Hand Wraps (4.5m)", SKU: "WRAP-45", Stock: 24, Price: 8, CostPrice: 4, LowStockThreshold: 6, Active: true, CreatedAt: now, UpdatedAt: now},
		models.InventoryItem{Name: "Boxing Gloves 12oz", SKU: "GLV-12", Stock: 8, Price: 45, CostPrice: 28, LowStockThreshold: 3, Active: true, CreatedAt: now, UpdatedAt: now},
		models.InventoryItem{Name: "Water Bottle 750ml", SKU: "H2O-750", Stock: 30, Price: 5, CostPrice: 2, LowStockThreshold: 10, Active: true, CreatedAt: now, UpdatedAt: now},
		models.InventoryItem{Name: "Bronze Boxing Tee", SKU: "TEE-BB", Stock: 15, Price: 20, CostPrice: 9, LowStockThreshold: 5, Active: true, CreatedAt: now, UpdatedAt: now},
	}
	ires, err := store.Coll(models.CollInventory).InsertMany(ctx, items)
	if err != nil {
		log.Fatalf("inventory: %v", err)
	}

	log.Println("creating a sample sale...")
	wrapsID := ires.InsertedIDs[0].(primitive.ObjectID)
	karimID := karim
	sale := models.Sale{Item: wrapsID, ItemName: "Hand Wraps (4.5m)", Trainee: &karimID, TraineeName: name[0],
		Qty: 1, UnitPrice: 8, Total: 8, Date: now, CreatedAt: now}
	if _, err := store.Coll(models.CollSales).InsertOne(ctx, sale); err != nil {
		log.Fatalf("sale: %v", err)
	}

	counts := map[string]int64{}
	for _, c := range []string{models.CollTrainees, models.CollSessions, models.CollPayments, models.CollReminders, models.CollExpenses, models.CollInventory, models.CollSales} {
		n, _ := store.Coll(c).CountDocuments(ctx, bson.M{})
		counts[c] = n
	}
	log.Printf("seed complete: %+v", counts)
	_ = store.Disconnect(ctx)
}
