package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"xiii-vacation-bot/internal/domain"
)

const timeLayout = time.RFC3339

type StateRepository struct {
	db *sql.DB
}

type RequestRepository struct {
	db *sql.DB
}

type VacationRepository struct {
	db *sql.DB
}

func NewStateRepository(db *DB) *StateRepository {
	return &StateRepository{db: db.SQL()}
}

func NewRequestRepository(db *DB) *RequestRepository {
	return &RequestRepository{db: db.SQL()}
}

func NewVacationRepository(db *DB) *VacationRepository {
	return &VacationRepository{db: db.SQL()}
}

func FormatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(timeLayout, value)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return FormatTime(*value)
}

func (r *StateRepository) Get(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := Retry(ctx, func() error {
		return r.db.QueryRowContext(ctx, `SELECT value FROM bot_state WHERE key = ?`, key).Scan(&value)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get bot state %q: %w", key, err)
	}
	return value, true, nil
}

func (r *StateRepository) Set(ctx context.Context, key, value string) error {
	err := Retry(ctx, func() error {
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO bot_state(key, value)
			VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value
		`, key, value)
		return err
	})
	if err != nil {
		return fmt.Errorf("set bot state %q: %w", key, err)
	}
	return nil
}

func (r *RequestRepository) queryer(q DBTX) DBTX {
	if q != nil {
		return q
	}
	return r.db
}

func (r *VacationRepository) queryer(q DBTX) DBTX {
	if q != nil {
		return q
	}
	return r.db
}

func (r *RequestRepository) Create(ctx context.Context, q DBTX, request *domain.VacationRequest) (int64, error) {
	exec := r.queryer(q)
	var id int64
	err := Retry(ctx, func() error {
		result, err := exec.ExecContext(ctx, `
			INSERT INTO vacation_requests(
				guild_id, user_id, days, reason, status, officer_message_id,
				officer_channel_id, created_at, decided_by, decided_at
			)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			request.GuildID,
			request.UserID,
			request.Days,
			request.Reason,
			string(request.Status),
			nullableString(request.OfficerMessageID),
			nullableString(request.OfficerChannelID),
			FormatTime(request.CreatedAt),
			nullableString(request.DecidedBy),
			nullableTime(request.DecidedAt),
		)
		if err != nil {
			return err
		}
		id, err = result.LastInsertId()
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("create vacation request: %w", err)
	}
	return id, nil
}

func (r *RequestRepository) GetByID(ctx context.Context, q DBTX, id int64) (*domain.VacationRequest, error) {
	exec := r.queryer(q)
	var request *domain.VacationRequest
	err := Retry(ctx, func() error {
		row := exec.QueryRowContext(ctx, `
			SELECT id, guild_id, user_id, days, reason, status, officer_message_id,
				officer_channel_id, created_at, decided_by, decided_at
			FROM vacation_requests
			WHERE id = ?
		`, id)
		var scanErr error
		request, scanErr = scanRequest(row)
		return scanErr
	})
	if err != nil {
		return nil, fmt.Errorf("get vacation request %d: %w", id, err)
	}
	return request, nil
}

func (r *RequestRepository) HasPendingByUser(ctx context.Context, q DBTX, guildID, userID string) (bool, error) {
	exec := r.queryer(q)
	var count int
	err := Retry(ctx, func() error {
		return exec.QueryRowContext(ctx, `
			SELECT COUNT(1)
			FROM vacation_requests
			WHERE guild_id = ? AND user_id = ? AND status = ?
		`, guildID, userID, string(domain.RequestStatusPending)).Scan(&count)
	})
	if err != nil {
		return false, fmt.Errorf("check pending request for user %s: %w", userID, err)
	}
	return count > 0, nil
}

func (r *RequestRepository) UpdateOfficerMessage(ctx context.Context, q DBTX, id int64, channelID, messageID string) error {
	exec := r.queryer(q)
	err := Retry(ctx, func() error {
		_, err := exec.ExecContext(ctx, `
			UPDATE vacation_requests
			SET officer_channel_id = ?, officer_message_id = ?
			WHERE id = ?
		`, channelID, messageID, id)
		return err
	})
	if err != nil {
		return fmt.Errorf("update officer message for request %d: %w", id, err)
	}
	return nil
}

func (r *RequestRepository) SetApproved(ctx context.Context, q DBTX, id int64, decidedBy string, decidedAt time.Time) error {
	exec := r.queryer(q)
	var rows int64
	err := Retry(ctx, func() error {
		result, err := exec.ExecContext(ctx, `
			UPDATE vacation_requests
			SET status = ?, decided_by = ?, decided_at = ?
			WHERE id = ? AND status = ?
		`, string(domain.RequestStatusApproved), decidedBy, FormatTime(decidedAt), id, string(domain.RequestStatusPending))
		if err != nil {
			return err
		}
		rows, err = result.RowsAffected()
		return err
	})
	if err != nil {
		return fmt.Errorf("approve request %d: %w", id, err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *RequestRepository) SetRejected(ctx context.Context, q DBTX, id int64, decidedBy string, decidedAt time.Time) error {
	exec := r.queryer(q)
	var rows int64
	err := Retry(ctx, func() error {
		result, err := exec.ExecContext(ctx, `
			UPDATE vacation_requests
			SET status = ?, decided_by = ?, decided_at = ?
			WHERE id = ? AND status = ?
		`, string(domain.RequestStatusRejected), decidedBy, FormatTime(decidedAt), id, string(domain.RequestStatusPending))
		if err != nil {
			return err
		}
		rows, err = result.RowsAffected()
		return err
	})
	if err != nil {
		return fmt.Errorf("reject request %d: %w", id, err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *RequestRepository) Delete(ctx context.Context, q DBTX, id int64) error {
	exec := r.queryer(q)
	err := Retry(ctx, func() error {
		_, err := exec.ExecContext(ctx, `DELETE FROM vacation_requests WHERE id = ?`, id)
		return err
	})
	if err != nil {
		return fmt.Errorf("delete vacation request %d: %w", id, err)
	}
	return nil
}

func (r *VacationRepository) Create(ctx context.Context, q DBTX, vacation *domain.Vacation) (int64, error) {
	exec := r.queryer(q)
	var id int64
	err := Retry(ctx, func() error {
		result, err := exec.ExecContext(ctx, `
			INSERT INTO vacations(
				request_id, guild_id, user_id, role_id, days, reason, status,
				started_at, expected_end_at, ended_at, ended_by, end_type, dm_message_id
			)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			vacation.RequestID,
			vacation.GuildID,
			vacation.UserID,
			vacation.RoleID,
			vacation.Days,
			vacation.Reason,
			string(vacation.Status),
			FormatTime(vacation.StartedAt),
			FormatTime(vacation.ExpectedEndAt),
			nullableTime(vacation.EndedAt),
			nullableString(vacation.EndedBy),
			nullableString(string(vacation.EndType)),
			nullableString(vacation.DMMessageID),
		)
		if err != nil {
			return err
		}
		id, err = result.LastInsertId()
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("create vacation: %w", err)
	}
	return id, nil
}

func (r *VacationRepository) GetByID(ctx context.Context, q DBTX, id int64) (*domain.Vacation, error) {
	exec := r.queryer(q)
	var vacation *domain.Vacation
	err := Retry(ctx, func() error {
		row := exec.QueryRowContext(ctx, `
			SELECT id, request_id, guild_id, user_id, role_id, days, reason, status,
				started_at, expected_end_at, ended_at, ended_by, end_type, dm_message_id
			FROM vacations
			WHERE id = ?
		`, id)
		var scanErr error
		vacation, scanErr = scanVacation(row)
		return scanErr
	})
	if err != nil {
		return nil, fmt.Errorf("get vacation %d: %w", id, err)
	}
	return vacation, nil
}

func (r *VacationRepository) HasActiveByUser(ctx context.Context, q DBTX, guildID, userID string) (bool, error) {
	exec := r.queryer(q)
	var count int
	err := Retry(ctx, func() error {
		return exec.QueryRowContext(ctx, `
			SELECT COUNT(1)
			FROM vacations
			WHERE guild_id = ? AND user_id = ? AND status = ?
		`, guildID, userID, string(domain.VacationStatusActive)).Scan(&count)
	})
	if err != nil {
		return false, fmt.Errorf("check active vacation for user %s: %w", userID, err)
	}
	return count > 0, nil
}

func (r *VacationRepository) SetDMMessageID(ctx context.Context, q DBTX, id int64, messageID string) error {
	exec := r.queryer(q)
	err := Retry(ctx, func() error {
		_, err := exec.ExecContext(ctx, `
			UPDATE vacations
			SET dm_message_id = ?
			WHERE id = ?
		`, messageID, id)
		return err
	})
	if err != nil {
		return fmt.Errorf("set vacation DM message %d: %w", id, err)
	}
	return nil
}

func (r *VacationRepository) ListExpiredActive(ctx context.Context, now time.Time, limit int) ([]*domain.Vacation, error) {
	if limit <= 0 {
		limit = 100
	}

	var vacations []*domain.Vacation
	err := Retry(ctx, func() error {
		rows, err := r.db.QueryContext(ctx, `
			SELECT id, request_id, guild_id, user_id, role_id, days, reason, status,
				started_at, expected_end_at, ended_at, ended_by, end_type, dm_message_id
			FROM vacations
			WHERE status = ? AND expected_end_at <= ?
			ORDER BY expected_end_at ASC
			LIMIT ?
		`, string(domain.VacationStatusActive), FormatTime(now), limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		result := make([]*domain.Vacation, 0)
		for rows.Next() {
			vacation, err := scanVacation(rows)
			if err != nil {
				return err
			}
			result = append(result, vacation)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		vacations = result
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list expired vacations: %w", err)
	}
	return vacations, nil
}

func (r *VacationRepository) ListActiveVacations(ctx context.Context, guildID string) ([]domain.ActiveVacationView, error) {
	var vacations []domain.ActiveVacationView
	err := Retry(ctx, func() error {
		rows, err := r.db.QueryContext(ctx, `
			SELECT id, request_id, guild_id, user_id, days, reason, started_at, expected_end_at, status
			FROM vacations
			WHERE guild_id = ? AND status = ?
			ORDER BY expected_end_at ASC
		`, guildID, string(domain.VacationStatusActive))
		if err != nil {
			return err
		}
		defer rows.Close()

		result := make([]domain.ActiveVacationView, 0)
		for rows.Next() {
			vacation, err := scanActiveVacationView(rows)
			if err != nil {
				return err
			}
			result = append(result, vacation)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		vacations = result
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list active vacations: %w", err)
	}
	return vacations, nil
}

func (r *VacationRepository) End(ctx context.Context, q DBTX, id int64, endType domain.VacationEndType, endedBy string, endedAt time.Time) error {
	exec := r.queryer(q)
	var rows int64
	err := Retry(ctx, func() error {
		result, err := exec.ExecContext(ctx, `
			UPDATE vacations
			SET status = ?, ended_at = ?, ended_by = ?, end_type = ?
			WHERE id = ? AND status = ?
		`, string(domain.VacationStatusEnded), FormatTime(endedAt), endedBy, string(endType), id, string(domain.VacationStatusActive))
		if err != nil {
			return err
		}
		rows, err = result.RowsAffected()
		return err
	})
	if err != nil {
		return fmt.Errorf("end vacation %d: %w", id, err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRequest(scanner rowScanner) (*domain.VacationRequest, error) {
	var (
		request          domain.VacationRequest
		status           string
		officerMessageID sql.NullString
		officerChannelID sql.NullString
		createdAt        string
		decidedBy        sql.NullString
		decidedAt        sql.NullString
	)
	if err := scanner.Scan(
		&request.ID,
		&request.GuildID,
		&request.UserID,
		&request.Days,
		&request.Reason,
		&status,
		&officerMessageID,
		&officerChannelID,
		&createdAt,
		&decidedBy,
		&decidedAt,
	); err != nil {
		return nil, err
	}

	parsedCreatedAt, err := parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse request created_at: %w", err)
	}
	request.Status = domain.RequestStatus(status)
	request.CreatedAt = parsedCreatedAt
	if officerMessageID.Valid {
		request.OfficerMessageID = officerMessageID.String
	}
	if officerChannelID.Valid {
		request.OfficerChannelID = officerChannelID.String
	}
	if decidedBy.Valid {
		request.DecidedBy = decidedBy.String
	}
	if decidedAt.Valid {
		parsedDecidedAt, err := parseTime(decidedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse request decided_at: %w", err)
		}
		request.DecidedAt = &parsedDecidedAt
	}

	return &request, nil
}

func scanVacation(scanner rowScanner) (*domain.Vacation, error) {
	var (
		vacation      domain.Vacation
		status        string
		startedAt     string
		expectedEndAt string
		endedAt       sql.NullString
		endedBy       sql.NullString
		endType       sql.NullString
		dmMessageID   sql.NullString
	)
	if err := scanner.Scan(
		&vacation.ID,
		&vacation.RequestID,
		&vacation.GuildID,
		&vacation.UserID,
		&vacation.RoleID,
		&vacation.Days,
		&vacation.Reason,
		&status,
		&startedAt,
		&expectedEndAt,
		&endedAt,
		&endedBy,
		&endType,
		&dmMessageID,
	); err != nil {
		return nil, err
	}

	parsedStartedAt, err := parseTime(startedAt)
	if err != nil {
		return nil, fmt.Errorf("parse vacation started_at: %w", err)
	}
	parsedExpectedEndAt, err := parseTime(expectedEndAt)
	if err != nil {
		return nil, fmt.Errorf("parse vacation expected_end_at: %w", err)
	}
	vacation.Status = domain.VacationStatus(status)
	vacation.StartedAt = parsedStartedAt
	vacation.ExpectedEndAt = parsedExpectedEndAt
	if endedAt.Valid {
		parsedEndedAt, err := parseTime(endedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse vacation ended_at: %w", err)
		}
		vacation.EndedAt = &parsedEndedAt
	}
	if endedBy.Valid {
		vacation.EndedBy = endedBy.String
	}
	if endType.Valid {
		vacation.EndType = domain.VacationEndType(endType.String)
	}
	if dmMessageID.Valid {
		vacation.DMMessageID = dmMessageID.String
	}

	return &vacation, nil
}

func scanActiveVacationView(scanner rowScanner) (domain.ActiveVacationView, error) {
	var (
		vacation      domain.ActiveVacationView
		status        string
		startedAt     string
		expectedEndAt string
	)
	if err := scanner.Scan(
		&vacation.ID,
		&vacation.RequestID,
		&vacation.GuildID,
		&vacation.UserID,
		&vacation.Days,
		&vacation.Reason,
		&startedAt,
		&expectedEndAt,
		&status,
	); err != nil {
		return domain.ActiveVacationView{}, err
	}

	parsedStartedAt, err := parseTime(startedAt)
	if err != nil {
		return domain.ActiveVacationView{}, fmt.Errorf("parse active vacation started_at: %w", err)
	}
	parsedExpectedEndAt, err := parseTime(expectedEndAt)
	if err != nil {
		return domain.ActiveVacationView{}, fmt.Errorf("parse active vacation expected_end_at: %w", err)
	}
	vacation.StartedAt = parsedStartedAt
	vacation.ExpectedEndAt = parsedExpectedEndAt
	vacation.Status = domain.VacationStatus(status)

	return vacation, nil
}
