package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	sqlDB *sql.DB
}

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" {
		return nil, errors.New("database path is empty")
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	dsn := path
	if !strings.Contains(dsn, "?") {
		dsn += "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=ON"
	}

	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	db := &DB{sqlDB: sqlDB}
	if err := db.Ping(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := RunMigrations(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return db, nil
}

func (db *DB) Ping(ctx context.Context) error {
	return retrySQLite(ctx, func() error {
		if err := db.sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("ping sqlite: %w", err)
		}
		return nil
	})
}

func (db *DB) Close() error {
	return db.sqlDB.Close()
}

func (db *DB) SQL() *sql.DB {
	return db.sqlDB
}

func (db *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	return retrySQLite(ctx, func() error {
		tx, err := db.sqlDB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() {
			_ = tx.Rollback()
		}()

		if err := fn(tx); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		return nil
	})
}

func retrySQLite(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if !IsBusyError(err) {
				return err
			}
			delay := time.Duration(50*(attempt+1)) * time.Millisecond
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
				continue
			}
		}
		return nil
	}
	return lastErr
}

func IsBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "sqlite_locked")
}

func Retry(ctx context.Context, fn func() error) error {
	return retrySQLite(ctx, fn)
}
