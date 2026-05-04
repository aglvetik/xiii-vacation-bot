package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"xiii-vacation-bot/internal/config"
	"xiii-vacation-bot/internal/database"
	"xiii-vacation-bot/internal/domain"
)

var (
	ErrPendingRequestExists    = errors.New("pending request already exists")
	ErrActiveVacationExists    = errors.New("active vacation already exists")
	ErrRequestNotFound         = errors.New("request not found")
	ErrRequestAlreadyProcessed = errors.New("request already processed")
	ErrVacationNotFound        = errors.New("vacation not found")
	ErrVacationNotActive       = errors.New("vacation is not active")
	ErrVacationNotOwned        = errors.New("vacation does not belong to user")
)

type VacationService struct {
	cfg       config.Config
	db        *database.DB
	requests  *database.RequestRepository
	vacations *database.VacationRepository
}

func NewVacationService(
	cfg config.Config,
	db *database.DB,
	requests *database.RequestRepository,
	vacations *database.VacationRepository,
) *VacationService {
	return &VacationService{
		cfg:       cfg,
		db:        db,
		requests:  requests,
		vacations: vacations,
	}
}

func (s *VacationService) SubmitRequest(ctx context.Context, guildID, userID string, days int, reason string) (*domain.VacationRequest, error) {
	reason = strings.TrimSpace(reason)

	hasPending, err := s.requests.HasPendingByUser(ctx, nil, guildID, userID)
	if err != nil {
		return nil, err
	}
	if hasPending {
		return nil, ErrPendingRequestExists
	}

	hasActive, err := s.vacations.HasActiveByUser(ctx, nil, guildID, userID)
	if err != nil {
		return nil, err
	}
	if hasActive {
		return nil, ErrActiveVacationExists
	}

	request := &domain.VacationRequest{
		GuildID:   guildID,
		UserID:    userID,
		Days:      days,
		Reason:    reason,
		Status:    domain.RequestStatusPending,
		CreatedAt: time.Now().UTC(),
	}

	id, err := s.requests.Create(ctx, nil, request)
	if err != nil {
		return nil, err
	}
	request.ID = id

	return request, nil
}

func (s *VacationService) DeleteRequest(ctx context.Context, requestID int64) error {
	return s.requests.Delete(ctx, nil, requestID)
}

func (s *VacationService) SaveOfficerMessage(ctx context.Context, requestID int64, channelID, messageID string) error {
	return s.requests.UpdateOfficerMessage(ctx, nil, requestID, channelID, messageID)
}

func (s *VacationService) PrepareApproval(ctx context.Context, requestID int64) (*domain.VacationRequest, error) {
	request, err := s.requests.GetByID(ctx, nil, requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	if request.Status != domain.RequestStatusPending {
		return nil, ErrRequestAlreadyProcessed
	}

	hasActive, err := s.vacations.HasActiveByUser(ctx, nil, request.GuildID, request.UserID)
	if err != nil {
		return nil, err
	}
	if hasActive {
		return nil, ErrActiveVacationExists
	}

	return request, nil
}

func (s *VacationService) ApproveRequest(ctx context.Context, requestID int64, approvedBy string, now time.Time) (*domain.VacationRequest, *domain.Vacation, error) {
	now = now.UTC()
	var request *domain.VacationRequest
	var vacation *domain.Vacation

	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		request, err = s.requests.GetByID(ctx, tx, requestID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRequestNotFound
		}
		if err != nil {
			return err
		}
		if request.Status != domain.RequestStatusPending {
			return ErrRequestAlreadyProcessed
		}

		hasActive, err := s.vacations.HasActiveByUser(ctx, tx, request.GuildID, request.UserID)
		if err != nil {
			return err
		}
		if hasActive {
			return ErrActiveVacationExists
		}

		expectedEnd := now.Add(time.Duration(request.Days) * 24 * time.Hour)
		vacation = &domain.Vacation{
			RequestID:     request.ID,
			GuildID:       request.GuildID,
			UserID:        request.UserID,
			RoleID:        s.cfg.VacationRoleID,
			Days:          request.Days,
			Reason:        request.Reason,
			Status:        domain.VacationStatusActive,
			StartedAt:     now,
			ExpectedEndAt: expectedEnd,
		}
		vacation.ID, err = s.vacations.Create(ctx, tx, vacation)
		if err != nil {
			return err
		}

		if err := s.requests.SetApproved(ctx, tx, request.ID, approvedBy, now); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRequestAlreadyProcessed
			}
			return err
		}

		request.Status = domain.RequestStatusApproved
		request.DecidedBy = approvedBy
		request.DecidedAt = &now
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return request, vacation, nil
}

func (s *VacationService) RejectRequest(ctx context.Context, requestID int64, rejectedBy string, now time.Time) (*domain.VacationRequest, error) {
	now = now.UTC()
	var request *domain.VacationRequest

	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		request, err = s.requests.GetByID(ctx, tx, requestID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRequestNotFound
		}
		if err != nil {
			return err
		}
		if request.Status != domain.RequestStatusPending {
			return ErrRequestAlreadyProcessed
		}
		if err := s.requests.SetRejected(ctx, tx, request.ID, rejectedBy, now); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRequestAlreadyProcessed
			}
			return err
		}
		request.Status = domain.RequestStatusRejected
		request.DecidedBy = rejectedBy
		request.DecidedAt = &now
		return nil
	})
	if err != nil {
		return nil, err
	}

	return request, nil
}

func (s *VacationService) GetVacationForUser(ctx context.Context, vacationID int64, userID string) (*domain.Vacation, error) {
	vacation, err := s.vacations.GetByID(ctx, nil, vacationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrVacationNotFound
	}
	if err != nil {
		return nil, err
	}
	if vacation.UserID != userID {
		return nil, ErrVacationNotOwned
	}
	if vacation.Status != domain.VacationStatusActive {
		return nil, ErrVacationNotActive
	}
	return vacation, nil
}

func (s *VacationService) EndVacationByUser(ctx context.Context, vacationID int64, userID string, now time.Time) (*domain.Vacation, error) {
	return s.endVacation(ctx, vacationID, userID, domain.VacationEndTypeEarlyUser, userID, now)
}

func (s *VacationService) EndVacationExpired(ctx context.Context, vacationID int64, now time.Time) (*domain.Vacation, error) {
	return s.endVacation(ctx, vacationID, "", domain.VacationEndTypeAutoExpired, "bot", now)
}

func (s *VacationService) endVacation(
	ctx context.Context,
	vacationID int64,
	requiredUserID string,
	endType domain.VacationEndType,
	endedBy string,
	now time.Time,
) (*domain.Vacation, error) {
	now = now.UTC()
	var vacation *domain.Vacation

	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		vacation, err = s.vacations.GetByID(ctx, tx, vacationID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrVacationNotFound
		}
		if err != nil {
			return err
		}
		if requiredUserID != "" && vacation.UserID != requiredUserID {
			return ErrVacationNotOwned
		}
		if vacation.Status != domain.VacationStatusActive {
			return ErrVacationNotActive
		}
		if err := s.vacations.End(ctx, tx, vacationID, endType, endedBy, now); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrVacationNotActive
			}
			return err
		}
		vacation.Status = domain.VacationStatusEnded
		vacation.EndType = endType
		vacation.EndedBy = endedBy
		vacation.EndedAt = &now
		return nil
	})
	if err != nil {
		return nil, err
	}

	return vacation, nil
}

func (s *VacationService) SaveDMMessage(ctx context.Context, vacationID int64, messageID string) error {
	if strings.TrimSpace(messageID) == "" {
		return nil
	}
	return s.vacations.SetDMMessageID(ctx, nil, vacationID, messageID)
}

func (s *VacationService) ListExpiredActive(ctx context.Context, now time.Time, limit int) ([]*domain.Vacation, error) {
	return s.vacations.ListExpiredActive(ctx, now.UTC(), limit)
}

func (s *VacationService) ListActiveVacations(ctx context.Context, guildID string) ([]domain.ActiveVacationView, error) {
	return s.vacations.ListActiveVacations(ctx, guildID)
}

func FriendlyServiceError(err error) string {
	switch {
	case errors.Is(err, ErrPendingRequestExists):
		return "У вас уже есть заявка на отпуск, которая ожидает рассмотрения."
	case errors.Is(err, ErrActiveVacationExists):
		return "У вас уже есть активный отпуск."
	case errors.Is(err, ErrRequestNotFound):
		return "Заявка не найдена."
	case errors.Is(err, ErrRequestAlreadyProcessed):
		return "Эта заявка уже была рассмотрена."
	case errors.Is(err, ErrVacationNotFound):
		return "Отпуск не найден."
	case errors.Is(err, ErrVacationNotOwned):
		return "Эта кнопка не относится к вашему отпуску."
	case errors.Is(err, ErrVacationNotActive):
		return "Этот отпуск уже завершён."
	default:
		return fmt.Sprintf("Произошла ошибка: %s", err.Error())
	}
}
