// Apply data migrations explicitly — never inside a request handler.
//
//	go run ./cmd/migrate -list      # what is applied / pending
//	go run ./cmd/migrate -dry-run   # report what would change, write nothing
//	go run ./cmd/migrate            # apply pending migrations in order
//	go run ./cmd/migrate -verify    # cross-check dues and stock projections
//
// Back up first (DEPLOY.md). Every migration is idempotent: re-running one
// (-only ID -rerun) against migrated data changes nothing.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"

	"bronzeboxing/internal/config"
	"bronzeboxing/internal/db"
	"bronzeboxing/internal/migrate"
	"bronzeboxing/internal/server"
)

func main() {
	dry := flag.Bool("dry-run", false, "report what would change without writing anything")
	list := flag.Bool("list", false, "list applied and pending migrations")
	only := flag.String("only", "", "run just this migration ID")
	rerun := flag.Bool("rerun", false, "run even if already applied (idempotency check)")
	verify := flag.Bool("verify", false, "cross-check charge balances and stock ledgers (read-only)")
	flag.Parse()

	_ = godotenv.Load()
	cfg := config.Load()
	if loc, err := time.LoadLocation(cfg.Timezone); err == nil {
		time.Local = loc
	} else {
		log.Fatalf("invalid STUDIO_TZ %q: %v", cfg.Timezone, err)
	}
	store, err := db.Connect(cfg.MongoURI, cfg.DBName)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	defer store.Disconnect(ctx)
	fmt.Printf("database %s (studio time %s, transactions %v)\n", cfg.DBName, cfg.Timezone, store.SupportsTx)

	all := server.Migrations()
	switch {
	case *list:
		done, err := migrate.Applied(ctx, store)
		if err != nil {
			log.Fatal(err)
		}
		for _, m := range all {
			if at, ok := done[m.ID]; ok {
				fmt.Printf("  applied %s  %s  %s\n", at.Format("2006-01-02 15:04"), m.ID, m.Description)
			} else {
				fmt.Printf("  PENDING                   %s  %s\n", m.ID, m.Description)
			}
		}
		return
	case *verify:
		findings, err := server.VerifyIntegrity(ctx, store)
		if err != nil {
			log.Fatal(err)
		}
		if len(findings) == 0 {
			fmt.Println("verify: OK — every charge matches its payments and every stock count matches its ledger")
			return
		}
		for _, f := range findings {
			fmt.Printf("  %-22s %s\n", f.Kind, f.Detail)
		}
		fmt.Printf("verify: %d finding(s)\n", len(findings))
		os.Exit(1)
	}

	r := &migrate.Runner{Store: store, DryRun: *dry, Logf: func(f string, a ...any) { fmt.Printf(f+"\n", a...) }}
	if err := r.Run(ctx, all, migrate.Options{Only: *only, Rerun: *rerun}); err != nil {
		log.Fatal(err)
	}
	if len(r.Notes) > 0 {
		fmt.Println("\nneeds review:")
		for _, n := range r.Notes {
			fmt.Println("  - " + n)
		}
	}
	if *dry {
		fmt.Println("\ndry run: nothing was written")
	} else {
		fmt.Println("\ndone")
	}
}
