package service

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"xiii-vacation-bot/internal/config"
	"xiii-vacation-bot/internal/domain"

	"github.com/bwmarrin/discordgo"
)

type NotificationService struct {
	cfg     config.Config
	session *discordgo.Session
	log     *slog.Logger
}

func NewNotificationService(cfg config.Config, session *discordgo.Session, log *slog.Logger) *NotificationService {
	return &NotificationService{cfg: cfg, session: session, log: log}
}

func (s *NotificationService) SendOfficerMessage(embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) (*discordgo.Message, error) {
	message, err := s.session.ChannelMessageSendComplex(s.cfg.OfficerChannelID, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	})
	if err != nil {
		return nil, fmt.Errorf("send officer message: %w", err)
	}
	return message, nil
}

func (s *NotificationService) EditMessage(channelID, messageID string, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	if strings.TrimSpace(channelID) == "" || strings.TrimSpace(messageID) == "" {
		return fmt.Errorf("message location is empty")
	}

	_, err := s.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    channelID,
		ID:         messageID,
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
	if err != nil {
		return fmt.Errorf("edit message %s/%s: %w", channelID, messageID, err)
	}
	return nil
}

func (s *NotificationService) SendDM(userID string, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) (*discordgo.Message, error) {
	channel, err := s.session.UserChannelCreate(userID)
	if err != nil {
		return nil, fmt.Errorf("create DM channel: %w", err)
	}

	message, err := s.session.ChannelMessageSendComplex(channel.ID, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	})
	if err != nil {
		return nil, fmt.Errorf("send DM: %w", err)
	}
	return message, nil
}

func (s *NotificationService) SendOfficerLog(embed *discordgo.MessageEmbed) error {
	if _, err := s.session.ChannelMessageSendEmbed(s.cfg.OfficerChannelID, embed); err != nil {
		return fmt.Errorf("send officer log: %w", err)
	}
	return nil
}

func (s *NotificationService) SendOfficerWarning(title, description string) {
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       0xFEE75C,
		Footer: &discordgo.MessageEmbedFooter{
			Text: s.cfg.BrandName,
		},
	}
	if err := s.SendOfficerLog(embed); err != nil {
		s.log.Warn("failed to send officer warning", slog.String("error", err.Error()))
	}
}

func (s *NotificationService) SendVacationExpiredDM(vacation *domain.Vacation) error {
	embed := &discordgo.MessageEmbed{
		Title:       "Отпуск завершён",
		Description: "Ваш отпуск завершён. Роль отпуска снята.",
		Color:       0x57F287,
		Footer: &discordgo.MessageEmbedFooter{
			Text: s.cfg.BrandName,
		},
	}
	_, err := s.SendDM(vacation.UserID, embed, nil)
	return err
}

func (s *NotificationService) SendAutoExpiredLog(vacation *domain.Vacation, issue string) error {
	fields := []*discordgo.MessageEmbedField{
		{Name: "User", Value: fmt.Sprintf("<@%s>\n`%s`", vacation.UserID, vacation.UserID), Inline: false},
		{Name: "Started at", Value: DiscordTimestamp(vacation.StartedAt), Inline: true},
		{Name: "Ended at", Value: DiscordTimestamp(timeOrNow(vacation.EndedAt)), Inline: true},
	}
	if strings.TrimSpace(issue) != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Проблема", Value: issue, Inline: false})
	}
	return s.SendOfficerLog(&discordgo.MessageEmbed{
		Title:  "Отпуск завершён автоматически",
		Color:  0x57F287,
		Fields: fields,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "XIII Vacation System",
		},
	})
}

func timeOrNow(value *time.Time) time.Time {
	if value == nil {
		return time.Now().UTC()
	}
	return *value
}

func DiscordTimestamp(t time.Time) string {
	return fmt.Sprintf("<t:%d:F>", t.UTC().Unix())
}
