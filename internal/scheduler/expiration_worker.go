package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"xiii-vacation-bot/internal/config"
	"xiii-vacation-bot/internal/domain"
	"xiii-vacation-bot/internal/service"

	"github.com/bwmarrin/discordgo"
)

type ExpirationWorker struct {
	cfg          config.Config
	session      *discordgo.Session
	log          *slog.Logger
	vacations    *service.VacationService
	notification *service.NotificationService
	afterChange  func()
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

func NewExpirationWorker(
	cfg config.Config,
	session *discordgo.Session,
	log *slog.Logger,
	vacations *service.VacationService,
	notification *service.NotificationService,
	afterChange func(),
) *ExpirationWorker {
	return &ExpirationWorker{
		cfg:          cfg,
		session:      session,
		log:          log,
		vacations:    vacations,
		notification: notification,
		afterChange:  afterChange,
	}
}

func (w *ExpirationWorker) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.loop(ctx)
	}()
}

func (w *ExpirationWorker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

func (w *ExpirationWorker) loop(ctx context.Context) {
	w.processOnce(ctx)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processOnce(ctx)
		}
	}
}

func (w *ExpirationWorker) processOnce(ctx context.Context) {
	vacations, err := w.vacations.ListExpiredActive(ctx, time.Now().UTC(), 100)
	if err != nil {
		w.log.Error("failed to list expired vacations", slog.String("error", err.Error()))
		return
	}
	for _, vacation := range vacations {
		w.processVacation(ctx, vacation)
	}
}

func (w *ExpirationWorker) processVacation(ctx context.Context, vacation *domain.Vacation) {
	removeIssue := ""
	if err := w.session.GuildMemberRoleRemove(vacation.GuildID, vacation.UserID, vacation.RoleID); err != nil {
		removeIssue = fmt.Sprintf("Не удалось снять роль: %s", err.Error())
		w.log.Warn("failed to remove vacation role during expiration", slog.Int64("vacation_id", vacation.ID), slog.String("user_id", vacation.UserID), slog.String("error", err.Error()))
	}

	endedVacation, err := w.vacations.EndVacationExpired(ctx, vacation.ID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, service.ErrVacationNotActive) {
			w.log.Info("vacation already ended before expiration worker processed it", slog.Int64("vacation_id", vacation.ID))
			return
		}
		w.log.Error("failed to mark expired vacation ended", slog.Int64("vacation_id", vacation.ID), slog.String("error", err.Error()))
		return
	}

	if err := w.notification.SendVacationExpiredDM(endedVacation); err != nil {
		w.log.Warn("failed to send vacation expiration DM", slog.Int64("vacation_id", vacation.ID), slog.String("user_id", vacation.UserID), slog.String("error", err.Error()))
		w.notification.SendOfficerWarning("Не удалось отправить DM", fmt.Sprintf("Отпуск <@%s> завершён автоматически, но личное сообщение не было доставлено.", vacation.UserID))
	}
	if err := w.notification.SendAutoExpiredLog(endedVacation, removeIssue); err != nil {
		w.log.Warn("failed to send auto expiration log", slog.Int64("vacation_id", vacation.ID), slog.String("error", err.Error()))
	}
	if w.afterChange != nil {
		w.afterChange()
	}
}

func IsMissingMemberOrRole(err error) bool {
	if err == nil {
		return false
	}
	var restErr *discordgo.RESTError
	if errors.As(err, &restErr) && restErr.Message != nil {
		switch restErr.Message.Code {
		case 10007, 10011:
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown member") ||
		strings.Contains(msg, "unknown role")
}
