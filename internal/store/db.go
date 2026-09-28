package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Store struct {
	DB      *sql.DB
	HoldTTL time.Duration
}

func Open(dsn string, holdTTL time.Duration) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if holdTTL <= 0 {
		holdTTL = 8 * time.Minute
	}
	return &Store{DB: db, HoldTTL: holdTTL}, nil
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) Migrate(ctx context.Context, sqlPath string) error {
	body, err := os.ReadFile(sqlPath)
	if err != nil {
		return err
	}
	return s.execScript(ctx, string(body))
}

func (s *Store) ResetSchema(ctx context.Context, dir string) error {
	reset := `
SET FOREIGN_KEY_CHECKS = 0;
DROP TABLE IF EXISTS payment_attempts;
DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS seats;
DROP TABLE IF EXISTS trial_classes;
DROP TABLE IF EXISTS students;
DROP TABLE IF EXISTS parents;
SET FOREIGN_KEY_CHECKS = 1;
`
	if err := s.execScript(ctx, reset); err != nil {
		return err
	}
	return s.MigrateFromDir(ctx, dir)
}

func isIgnorableMigrateErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "already exists")
}

func (s *Store) MigrateFromDir(ctx context.Context, dir string) error {
	path := filepath.Join(dir, "001_init.sql")
	return s.Migrate(ctx, path)
}

func (s *Store) execScript(ctx context.Context, script string) error {
	// Split on semicolons that end statements. Keep it simple: the migration
	// has no procedures or triggers.
	parts := strings.Split(script, ";")
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			if isIgnorableMigrateErr(err) {
				continue
			}
			return fmt.Errorf("migrate statement failed: %w\n%s", err, stmt)
		}
	}
	return nil
}
