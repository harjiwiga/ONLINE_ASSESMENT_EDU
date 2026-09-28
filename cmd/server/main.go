package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"online_assesment_edu/internal/config"
	httpx "online_assesment_edu/internal/http"
	"online_assesment_edu/internal/payment"
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

	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for range t.C {
			if err := st.SweepExpired(context.Background()); err != nil {
				log.Printf("sweep: %v", err)
			}
		}
	}()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpx.New(st, payment.Scripted{}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
