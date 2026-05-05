package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"xiii-vacation-bot/internal/domain"

	"github.com/bwmarrin/discordgo"
)

const activeVacationsPanelStateKey = "active_vacations_message_id"

func (c *Client) EnsureActiveVacationsPanel(ctx context.Context) error {
	c.log.Info("ensuring active vacations panel", slog.String("channel_id", c.cfg.ActiveVacationsChannelID))
	if err := c.RefreshActiveVacationsPanel(ctx); err != nil {
		c.log.Error("failed to ensure active vacations panel", slog.String("error", err.Error()))
		return err
	}
	return nil
}

func (c *Client) RefreshActiveVacationsPanel(ctx context.Context) error {
	c.activeVacationsPanelMu.Lock()
	defer c.activeVacationsPanelMu.Unlock()

	vacations, err := c.vacationSvc.ListActiveVacations(ctx, c.cfg.GuildID)
	if err != nil {
		return fmt.Errorf("list active vacations for panel: %w", err)
	}

	visibleVacations := c.visibleActiveVacations(c.session, vacations)
	embed := activeVacationsEmbed(visibleVacations)
	messageID, ok, err := c.state.Get(ctx, activeVacationsPanelStateKey)
	if err != nil {
		return err
	}

	if ok && messageID != "" {
		content := ""
		embeds := []*discordgo.MessageEmbed{embed}
		_, err := c.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel:         c.cfg.ActiveVacationsChannelID,
			ID:              messageID,
			Content:         &content,
			Embeds:          &embeds,
			AllowedMentions: noAllowedMentions(),
		})
		if err == nil {
			c.log.Info("active vacations panel refreshed", slog.String("message_id", messageID), slog.Int("visible_count", len(visibleVacations)))
			return nil
		}
		if !isUnknownMessageError(err) {
			return fmt.Errorf("edit active vacations panel %s: %w", messageID, err)
		}
		c.log.Warn("failed to edit existing active vacations panel, creating a new one", slog.String("message_id", messageID), slog.String("error", err.Error()))
	}

	message, err := c.session.ChannelMessageSendComplex(c.cfg.ActiveVacationsChannelID, &discordgo.MessageSend{
		Embeds:          []*discordgo.MessageEmbed{embed},
		AllowedMentions: noAllowedMentions(),
	})
	if err != nil {
		return fmt.Errorf("send active vacations panel: %w", err)
	}
	if err := c.state.Set(ctx, activeVacationsPanelStateKey, message.ID); err != nil {
		return err
	}
	c.log.Info("active vacations panel created", slog.String("message_id", message.ID), slog.Int("visible_count", len(visibleVacations)))
	return nil
}

func (c *Client) StartActiveVacationsPanelRefresher(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	c.activeVacationsPanelCancel = cancel
	c.activeVacationsPanelWG.Add(1)

	go func() {
		defer c.activeVacationsPanelWG.Done()

		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.refreshActiveVacationsPanelWithTimeout()
			}
		}
	}()
}

func (c *Client) StopActiveVacationsPanelRefresher() {
	if c.activeVacationsPanelCancel != nil {
		c.activeVacationsPanelCancel()
	}
	c.activeVacationsPanelWG.Wait()
}

func (c *Client) refreshActiveVacationsPanelAsync() {
	go c.refreshActiveVacationsPanelWithTimeout()
}

func (c *Client) refreshActiveVacationsPanelWithTimeout() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := c.RefreshActiveVacationsPanel(ctx); err != nil {
		c.log.Warn("failed to refresh active vacations panel", slog.String("error", err.Error()))
	}
}

func (c *Client) visibleActiveVacations(s *discordgo.Session, vacations []domain.ActiveVacationView) []domain.ActiveVacationView {
	visible := make([]domain.ActiveVacationView, 0, len(vacations))
	for _, vacation := range vacations {
		member, err := s.GuildMember(vacation.GuildID, vacation.UserID)
		if err != nil {
			continue
		}
		if memberHasRole(member, c.cfg.VacationRoleID) {
			visible = append(visible, vacation)
		}
	}
	return visible
}

func isUnknownMessageError(err error) bool {
	if err == nil {
		return false
	}
	var restErr *discordgo.RESTError
	if errors.As(err, &restErr) && restErr.Message != nil && restErr.Message.Code == 10008 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "unknown message")
}
