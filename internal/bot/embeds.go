package bot

import (
	"fmt"
	"strings"
	"time"

	"xiii-vacation-bot/internal/domain"
	"xiii-vacation-bot/internal/service"

	"github.com/bwmarrin/discordgo"
)

const (
	vacationsCommandName = "vacations"

	customApply       = "vacation:apply"
	customModal       = "vacation:modal"
	customApproveBase = "vacation:approve"
	customRejectBase  = "vacation:reject"
	customEndBase     = "vacation:end"
	customEndConfirm  = "vacation:end_confirm"
	customEndCancel   = "vacation:end_cancel"
)

const (
	activeVacationsDisplayLimit = 20
	activeVacationReasonLimit   = 180
	activeVacationsEmbedBudget  = 5400
)

func panelEmbed(brand string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "Отпуск XIII",
		Description: "Нужно временно отойти от активности?\n" +
			"Подай заявку на отпуск через кнопку ниже.\n\n" +
			"Укажи количество дней и причину.\n" +
			"Офицеры рассмотрят заявку и примут решение.",
		Color: 0x5865F2,
		Footer: &discordgo.MessageEmbedFooter{
			Text: brand,
		},
	}
}

func panelComponents() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Подать заявку на отпуск",
					Style:    discordgo.PrimaryButton,
					CustomID: customApply,
				},
			},
		},
	}
}

func officerRequestEmbed(request *domain.VacationRequest, status string, decidedBy string) *discordgo.MessageEmbed {
	fields := []*discordgo.MessageEmbedField{
		{Name: "Участник", Value: userValue(request.UserID), Inline: false},
		{Name: "Количество дней", Value: fmt.Sprintf("%d", request.Days), Inline: true},
		{Name: "Причина", Value: request.Reason, Inline: false},
		{Name: "Статус", Value: status, Inline: true},
	}
	if decidedBy != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Рассмотрел", Value: userValue(decidedBy), Inline: true})
	}

	return &discordgo.MessageEmbed{
		Title:  "Новая заявка на отпуск",
		Color:  statusColor(status),
		Fields: fields,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "XIII Vacation System",
		},
		Timestamp: request.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func officerRequestComponents(requestID int64, disabled bool) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Принять",
					Style:    discordgo.SuccessButton,
					CustomID: fmt.Sprintf("%s:%d", customApproveBase, requestID),
					Disabled: disabled,
				},
				discordgo.Button{
					Label:    "Отклонить",
					Style:    discordgo.DangerButton,
					CustomID: fmt.Sprintf("%s:%d", customRejectBase, requestID),
					Disabled: disabled,
				},
			},
		},
	}
}

func approvalDMEmbed(vacation *domain.Vacation, brand string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "Отпуск одобрен",
		Description: "Ваша заявка на отпуск была одобрена.\n" +
			"Вам выдана роль отпуска.\n\n" +
			"Вы можете досрочно закончить отпуск в любой момент через кнопку ниже.",
		Color: 0x57F287,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Количество дней", Value: fmt.Sprintf("%d", vacation.Days), Inline: true},
			{Name: "Окончание отпуска", Value: service.DiscordTimestamp(vacation.ExpectedEndAt), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: brand,
		},
	}
}

func approvalDMComponents(vacationID int64) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Досрочно закончить отпуск",
					Style:    discordgo.DangerButton,
					CustomID: fmt.Sprintf("%s:%d", customEndBase, vacationID),
				},
			},
		},
	}
}

func rejectionDMEmbed(brand string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "Заявка на отпуск отклонена",
		Description: "Ваша заявка на отпуск была отклонена офицерами.",
		Color:       0xED4245,
		Footer: &discordgo.MessageEmbedFooter{
			Text: brand,
		},
	}
}

func vacationStartedLogEmbed(vacation *domain.Vacation, approvedBy string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "Отпуск начат",
		Color: 0x57F287,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "User", Value: userValue(vacation.UserID), Inline: false},
			{Name: "Days", Value: fmt.Sprintf("%d", vacation.Days), Inline: true},
			{Name: "Expected end", Value: service.DiscordTimestamp(vacation.ExpectedEndAt), Inline: true},
			{Name: "Approved by", Value: userValue(approvedBy), Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "XIII Vacation System"},
	}
}

func requestRejectedLogEmbed(request *domain.VacationRequest, rejectedBy string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: "Заявка на отпуск отклонена",
		Color: 0xED4245,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "User", Value: userValue(request.UserID), Inline: false},
			{Name: "Days", Value: fmt.Sprintf("%d", request.Days), Inline: true},
			{Name: "Rejected by", Value: userValue(rejectedBy), Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "XIII Vacation System"},
	}
}

func earlyEndLogEmbed(vacation *domain.Vacation, issue string) *discordgo.MessageEmbed {
	fields := []*discordgo.MessageEmbedField{
		{Name: "User", Value: userValue(vacation.UserID), Inline: false},
		{Name: "Started at", Value: service.DiscordTimestamp(vacation.StartedAt), Inline: true},
		{Name: "Expected end", Value: service.DiscordTimestamp(vacation.ExpectedEndAt), Inline: true},
	}
	if vacation.EndedAt != nil {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Actual end", Value: service.DiscordTimestamp(*vacation.EndedAt), Inline: true})
	}
	if issue != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Проблема", Value: issue, Inline: false})
	}

	return &discordgo.MessageEmbed{
		Title:  "Отпуск завершён досрочно",
		Color:  0xFEE75C,
		Fields: fields,
		Footer: &discordgo.MessageEmbedFooter{Text: "XIII Vacation System"},
	}
}

func activeVacationsEmbed(vacations []domain.ActiveVacationView) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title: "Активные отпуска XIII",
		Color: 0x5865F2,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "XIII Vacation System",
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if len(vacations) == 0 {
		embed.Description = "Сейчас активных отпусков нет."
		return embed
	}

	embed.Description = fmt.Sprintf("Сейчас в отпуске: %d", len(vacations))
	budget := len(embed.Title) + len(embed.Description)
	displayed := 0

	for index, vacation := range vacations {
		if displayed >= activeVacationsDisplayLimit {
			break
		}

		name := fmt.Sprintf("**%d. <@%s>**", index+1, vacation.UserID)
		value := fmt.Sprintf(
			"ID: `%s`\nС: %s\nДо: %s\nОсталось: %s\nПричина: %s",
			vacation.UserID,
			discordTimestamp(vacation.StartedAt, "F"),
			discordTimestamp(vacation.ExpectedEndAt, "F"),
			discordTimestamp(vacation.ExpectedEndAt, "R"),
			trimEmbedReason(vacation.Reason),
		)

		if budget+len(name)+len(value) > activeVacationsEmbedBudget {
			break
		}

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   name,
			Value:  value,
			Inline: false,
		})
		budget += len(name) + len(value)
		displayed++
	}

	if displayed < len(vacations) {
		embed.Description += fmt.Sprintf("\nПоказаны первые %d отпусков из %d.", displayed, len(vacations))
	}

	return embed
}

func confirmationComponents(vacationID int64) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "Да, закончить",
					Style:    discordgo.DangerButton,
					CustomID: fmt.Sprintf("%s:%d", customEndConfirm, vacationID),
				},
				discordgo.Button{
					Label:    "Отмена",
					Style:    discordgo.SecondaryButton,
					CustomID: fmt.Sprintf("%s:%d", customEndCancel, vacationID),
				},
			},
		},
	}
}

func userValue(userID string) string {
	return fmt.Sprintf("<@%s>\n`%s`", userID, userID)
}

func statusColor(status string) int {
	switch status {
	case "Одобрено":
		return 0x57F287
	case "Отклонено":
		return 0xED4245
	default:
		return 0xFEE75C
	}
}

func discordTimestamp(t time.Time, style string) string {
	return fmt.Sprintf("<t:%d:%s>", t.UTC().Unix(), style)
}

func trimEmbedReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "Не указана"
	}

	runes := []rune(reason)
	if len(runes) <= activeVacationReasonLimit {
		return reason
	}
	return string(runes[:activeVacationReasonLimit]) + "..."
}
