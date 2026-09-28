package main

import (
	"context"
	"log"
	"os"

	"online_assesment_edu/internal/config"
	"online_assesment_edu/internal/store"
)

func main() {
	cfg := config.FromEnv()
	ctx := context.Background()
	st, err := store.Open(cfg.MySQLDSN, cfg.HoldTTL)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer st.Close()

	mig := os.Getenv("MIGRATIONS_PATH")
	if mig == "" {
		mig = "migrations"
	}
	if err := st.MigrateFromDir(ctx, mig); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := st.Seed(ctx); err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Printf("seed complete")
}
