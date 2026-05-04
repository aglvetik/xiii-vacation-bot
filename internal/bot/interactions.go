package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"xiii-vacation-bot/internal/domain"
	"xiii-vacation-bot/internal/service"

	"github.com/bwmarrin/discordgo"
)

func (c *Client) handleMessageComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	switch {
	case customID == customApply:
		c.handleApply(s, i)
	case strings.HasPrefix(customID, customApproveBase+":"):
		c.handleApprove(s, i, customID)
	case strings.HasPrefix(customID, customRejectBase+":"):
		c.handleReject(s, i, customID)
	case strings.HasPrefix(customID, customEndBase+":"):
		c.handleVacationEndPrompt(s, i, customID)
	case strings.HasPrefix(customID, customEndConfirm+":"):
		c.handleVacationEndConfirm(s, i, customID)
	case strings.HasPrefix(customID, customEndCancel+":"):
		c.handleVacationEndCancel(s, i)
	default:
		c.log.Warn("unknown component custom id", slog.String("custom_id", customID))
	}
}

func (c *Client) handleModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.ModalSubmitData().CustomID != customModal {
		return
	}
	c.handleVacationModal(s, i)
}

func (c *Client) handleApplicationCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	commandName := i.ApplicationCommandData().Name
	switch commandName {
	case vacationsCommandName:
		c.handleVacationsCommand(s, i)
	default:
		c.log.Warn("unknown application command", slog.String("command", commandName))
	}
}

func (c *Client) handleVacationsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID := interactionUserID(i)
	allowed, err := c.permissionSvc.CanReviewRequests(userID)
	if err != nil {
		c.log.Error("permission check failed for vacations command", slog.String("user_id", userID), slog.String("error", err.Error()))
		respondEphemeral(s, i, "Не удалось проверить доступ к этой команде.")
		return
	}
	if !allowed {
		respondEphemeral(s, i, "У вас нет доступа к этой команде.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vacations, err := c.vacationSvc.ListActiveVacations(ctx, c.cfg.GuildID)
	if err != nil {
		c.log.Error("failed to list active vacations", slog.String("error", err.Error()))
		respondPublic(s, i, "Не удалось загрузить активные отпуска. Попробуйте позже.")
		return
	}

	respondPublicEmbed(s, i, activeVacationsEmbed(vacations))
}

func (c *Client) handleApply(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: customModal,
			Title:    "Заявка на отпуск",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "days",
							Label:       "На сколько дней?",
							Style:       discordgo.TextInputShort,
							Placeholder: "Например: 3",
							Required:    true,
							MinLength:   1,
							MaxLength:   3,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "reason",
							Label:       "Причина отпуска",
							Style:       discordgo.TextInputParagraph,
							Placeholder: "Коротко объясни причину",
							Required:    true,
							MinLength:   5,
							MaxLength:   1000,
						},
					},
				},
			},
		},
	})
	if err != nil {
		c.log.Warn("failed to open vacation modal", slog.String("error", err.Error()))
	}
}

func (c *Client) handleVacationModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	fields := modalValues(i.ModalSubmitData().Components)
	daysRaw := strings.TrimSpace(fields["days"])
	reason := strings.TrimSpace(fields["reason"])

	days, err := strconv.Atoi(daysRaw)
	if err != nil {
		respondEphemeral(s, i, "Количество дней должно быть целым числом.")
		return
	}
	if days < 1 {
		respondEphemeral(s, i, "Количество дней должно быть не меньше 1.")
		return
	}
	if days > c.cfg.MaxVacationDays {
		respondEphemeral(s, i, fmt.Sprintf("Максимальная длительность отпуска: %d дн.", c.cfg.MaxVacationDays))
		return
	}
	if reason == "" {
		respondEphemeral(s, i, "Причина отпуска не может быть пустой.")
		return
	}

	userID := interactionUserID(i)
	guildID := i.GuildID
	if guildID == "" {
		guildID = c.cfg.GuildID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	request, err := c.vacationSvc.SubmitRequest(ctx, guildID, userID, days, reason)
	if err != nil {
		respondEphemeral(s, i, service.FriendlyServiceError(err))
		if !errors.Is(err, service.ErrPendingRequestExists) && !errors.Is(err, service.ErrActiveVacationExists) {
			c.log.Error("failed to submit vacation request", slog.String("user_id", userID), slog.String("error", err.Error()))
		}
		return
	}

	message, err := c.notification.SendOfficerMessage(
		officerRequestEmbed(request, "Ожидает рассмотрения", ""),
		officerRequestComponents(request.ID, false),
	)
	if err != nil {
		c.log.Error("failed to send officer request message", slog.Int64("request_id", request.ID), slog.String("error", err.Error()))
		if cleanupErr := c.vacationSvc.DeleteRequest(ctx, request.ID); cleanupErr != nil {
			c.log.Error("failed to cleanup request after officer message failure", slog.Int64("request_id", request.ID), slog.String("error", cleanupErr.Error()))
		}
		respondEphemeral(s, i, "Не удалось отправить заявку офицерам. Попробуйте позже.")
		return
	}
	if err := c.vacationSvc.SaveOfficerMessage(ctx, request.ID, message.ChannelID, message.ID); err != nil {
		c.log.Error("failed to save officer message", slog.Int64("request_id", request.ID), slog.String("error", err.Error()))
	}

	respondEphemeral(s, i, "Ваша заявка на отпуск отправлена. Офицеры рассмотрят её в ближайшее время.")
}

func (c *Client) handleApprove(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	requestID, err := parseCustomID(customApproveBase, customID)
	if err != nil {
		respondEphemeral(s, i, "Некорректная кнопка заявки.")
		return
	}
	if !c.deferEphemeral(s, i) {
		return
	}

	officerID := interactionUserID(i)
	allowed, err := c.permissionSvc.CanReviewRequests(officerID)
	if err != nil {
		c.log.Error("permission check failed", slog.String("user_id", officerID), slog.String("error", err.Error()))
		c.followupEphemeral(s, i, "Не удалось проверить доступ к рассмотрению заявок.")
		return
	}
	if !allowed {
		c.followupEphemeral(s, i, "У вас нет доступа к рассмотрению заявок.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	preparedRequest, err := c.vacationSvc.PrepareApproval(ctx, requestID)
	if err != nil {
		c.followupEphemeral(s, i, service.FriendlyServiceError(err))
		return
	}

	if err := s.GuildMemberRoleAdd(preparedRequest.GuildID, preparedRequest.UserID, c.cfg.VacationRoleID); err != nil {
		c.log.Error("failed to add vacation role", slog.Int64("request_id", requestID), slog.String("user_id", preparedRequest.UserID), slog.String("error", err.Error()))
		c.followupEphemeral(s, i, "Не удалось выдать роль отпуска. Заявка не была одобрена.")
		return
	}

	request, vacation, err := c.vacationSvc.ApproveRequest(ctx, requestID, officerID, time.Now().UTC())
	if err != nil {
		c.log.Error("failed to approve request after role add", slog.Int64("request_id", requestID), slog.String("error", err.Error()))
		c.followupEphemeral(s, i, service.FriendlyServiceError(err))
		return
	}

	c.editOfficerRequestMessage(i, request, "Одобрено", officerID)

	dm, err := c.notification.SendDM(request.UserID, approvalDMEmbed(vacation, c.cfg.BrandName), approvalDMComponents(vacation.ID))
	if err != nil {
		c.log.Warn("failed to send approval DM", slog.Int64("vacation_id", vacation.ID), slog.String("user_id", request.UserID), slog.String("error", err.Error()))
		c.notification.SendOfficerWarning("Не удалось отправить DM", fmt.Sprintf("Отпуск для <@%s> одобрен, но личное сообщение не было доставлено.", request.UserID))
	} else if err := c.vacationSvc.SaveDMMessage(ctx, vacation.ID, dm.ID); err != nil {
		c.log.Error("failed to save vacation DM message", slog.Int64("vacation_id", vacation.ID), slog.String("error", err.Error()))
	}

	if err := c.notification.SendOfficerLog(vacationStartedLogEmbed(vacation, officerID)); err != nil {
		c.log.Warn("failed to send vacation started log", slog.Int64("vacation_id", vacation.ID), slog.String("error", err.Error()))
	}

	c.followupEphemeral(s, i, "Заявка одобрена. Роль отпуска выдана.")
}

func (c *Client) handleReject(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	requestID, err := parseCustomID(customRejectBase, customID)
	if err != nil {
		respondEphemeral(s, i, "Некорректная кнопка заявки.")
		return
	}
	if !c.deferEphemeral(s, i) {
		return
	}

	officerID := interactionUserID(i)
	allowed, err := c.permissionSvc.CanReviewRequests(officerID)
	if err != nil {
		c.log.Error("permission check failed", slog.String("user_id", officerID), slog.String("error", err.Error()))
		c.followupEphemeral(s, i, "Не удалось проверить доступ к рассмотрению заявок.")
		return
	}
	if !allowed {
		c.followupEphemeral(s, i, "У вас нет доступа к рассмотрению заявок.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	request, err := c.vacationSvc.RejectRequest(ctx, requestID, officerID, time.Now().UTC())
	if err != nil {
		c.followupEphemeral(s, i, service.FriendlyServiceError(err))
		return
	}

	c.editOfficerRequestMessage(i, request, "Отклонено", officerID)

	if _, err := c.notification.SendDM(request.UserID, rejectionDMEmbed(c.cfg.BrandName), nil); err != nil {
		c.log.Warn("failed to send rejection DM", slog.Int64("request_id", request.ID), slog.String("user_id", request.UserID), slog.String("error", err.Error()))
		c.notification.SendOfficerWarning("Не удалось отправить DM", fmt.Sprintf("Заявка <@%s> отклонена, но личное сообщение не было доставлено.", request.UserID))
	}

	if err := c.notification.SendOfficerLog(requestRejectedLogEmbed(request, officerID)); err != nil {
		c.log.Warn("failed to send request rejected log", slog.Int64("request_id", request.ID), slog.String("error", err.Error()))
	}

	c.followupEphemeral(s, i, "Заявка отклонена.")
}

func (c *Client) handleVacationEndPrompt(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	vacationID, err := parseCustomID(customEndBase, customID)
	if err != nil {
		respondDM(s, i, "Некорректная кнопка отпуска.", nil)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = c.vacationSvc.GetVacationForUser(ctx, vacationID, interactionUserID(i))
	if err != nil {
		respondDM(s, i, service.FriendlyServiceError(err), nil)
		return
	}

	respondDM(s, i, "Вы уверены, что хотите досрочно закончить отпуск?", confirmationComponents(vacationID))
}

func (c *Client) handleVacationEndConfirm(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	vacationID, err := parseCustomID(customEndConfirm, customID)
	if err != nil {
		updateInteractionMessage(s, i, "Некорректная кнопка отпуска.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	userID := interactionUserID(i)
	vacation, err := c.vacationSvc.GetVacationForUser(ctx, vacationID, userID)
	if err != nil {
		updateInteractionMessage(s, i, service.FriendlyServiceError(err))
		return
	}

	removeIssue := ""
	if err := s.GuildMemberRoleRemove(vacation.GuildID, vacation.UserID, vacation.RoleID); err != nil {
		if isBenignRoleRemovalError(err) {
			removeIssue = fmt.Sprintf("Роль уже снята или участник недоступен: %s", err.Error())
			c.log.Warn("benign role removal issue during early end", slog.Int64("vacation_id", vacation.ID), slog.String("error", err.Error()))
		} else {
			c.log.Error("failed to remove vacation role during early end", slog.Int64("vacation_id", vacation.ID), slog.String("error", err.Error()))
			respondEphemeral(s, i, "Не удалось снять роль отпуска. Отпуск не был завершён.")
			return
		}
	}

	endedVacation, err := c.vacationSvc.EndVacationByUser(ctx, vacationID, userID, time.Now().UTC())
	if err != nil {
		updateInteractionMessage(s, i, service.FriendlyServiceError(err))
		return
	}

	if err := c.notification.SendOfficerLog(earlyEndLogEmbed(endedVacation, removeIssue)); err != nil {
		c.log.Warn("failed to send early vacation end log", slog.Int64("vacation_id", vacationID), slog.String("error", err.Error()))
	}

	updateInteractionMessage(s, i, "Вы досрочно закончили отпуск. Роль отпуска снята.")
}

func (c *Client) handleVacationEndCancel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	updateInteractionMessage(s, i, "Завершение отпуска отменено.")
}

func (c *Client) editOfficerRequestMessage(i *discordgo.InteractionCreate, request *domain.VacationRequest, status, decidedBy string) {
	channelID := request.OfficerChannelID
	messageID := request.OfficerMessageID
	if channelID == "" && i.ChannelID != "" {
		channelID = i.ChannelID
	}
	if messageID == "" && i.Message != nil {
		messageID = i.Message.ID
	}

	err := c.notification.EditMessage(
		channelID,
		messageID,
		officerRequestEmbed(request, status, decidedBy),
		officerRequestComponents(request.ID, true),
	)
	if err != nil {
		c.log.Warn("failed to edit officer request message", slog.Int64("request_id", request.ID), slog.String("error", err.Error()))
		c.notification.SendOfficerWarning("Не удалось обновить сообщение заявки", fmt.Sprintf("Заявка `%d` обработана, но исходное сообщение заявки не удалось обновить.", request.ID))
	}
}

func modalValues(components []discordgo.MessageComponent) map[string]string {
	values := make(map[string]string)
	for _, component := range components {
		row, ok := component.(*discordgo.ActionsRow)
		if !ok {
			if rowValue, ok := component.(discordgo.ActionsRow); ok {
				row = &rowValue
			} else {
				continue
			}
		}
		for _, nested := range row.Components {
			input, ok := nested.(*discordgo.TextInput)
			if !ok {
				if inputValue, ok := nested.(discordgo.TextInput); ok {
					input = &inputValue
				} else {
					continue
				}
			}
			values[input.CustomID] = input.Value
		}
	}
	return values
}

func parseCustomID(prefix, customID string) (int64, error) {
	raw := strings.TrimPrefix(customID, prefix+":")
	if raw == customID || raw == "" {
		return 0, fmt.Errorf("custom id %q does not match prefix %q", customID, prefix)
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid id in custom id %q", customID)
	}
	return id, nil
}

func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		})
	}
}

func respondPublic(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	}); err != nil {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: content,
		})
	}
}

func respondPublicEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	}); err != nil {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Embeds: []*discordgo.MessageEmbed{embed},
		})
	}
}

func (c *Client) deferEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		c.log.Warn("failed to defer interaction", slog.String("error", err.Error()))
		return false
	}
	return true
}

func (c *Client) followupEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		c.log.Warn("failed to send interaction followup", slog.String("error", err.Error()))
	}
}

func respondDM(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    content,
			Components: components,
		},
	}); err != nil {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content:    content,
			Components: components,
		})
	}
}

func updateInteractionMessage(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	components := []discordgo.MessageComponent{}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    content,
			Components: components,
		},
	}); err != nil {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: content,
		})
	}
}

func isBenignRoleRemovalError(err error) bool {
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
		strings.Contains(msg, "unknown role") ||
		strings.Contains(msg, "member not found") ||
		strings.Contains(msg, "role not found")
}
