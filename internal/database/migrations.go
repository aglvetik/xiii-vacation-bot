package database

import (
	"context"
	"database/sql"
	"fmt"
)

func RunMigrations(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS bot_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS vacation_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			guild_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			days INTEGER NOT NULL,
			reason TEXT NOT NULL,
			status TEXT NOT NULL,
			officer_message_id TEXT,
			officer_channel_id TEXT,
			created_at TEXT NOT NULL,
			decided_by TEXT,
			decided_at TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS vacations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_id INTEGER NOT NULL,
			guild_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			role_id TEXT NOT NULL,
			days INTEGER NOT NULL,
			reason TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			expected_end_at TEXT NOT NULL,
			ended_at TEXT,
			ended_by TEXT,
			end_type TEXT,
			dm_message_id TEXT,
			FOREIGN KEY(request_id) REFERENCES vacation_requests(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_requests_user_status ON vacation_requests(user_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_vacations_user_status ON vacations(user_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_vacations_expected_end ON vacations(status, expected_end_at);`,
	}

	for _, statement := range statements {
		if err := Retry(ctx, func() error {
			_, err := db.ExecContext(ctx, statement)
			return err
		}); err != nil {
			return fmt.Errorf("run migration: %w", err)
		}
	}
	return nil
}
