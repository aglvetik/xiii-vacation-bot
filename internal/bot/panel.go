package bot

import (
	"context"
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

const panelStateKey = "panel_message_id"

func (c *Client) EnsurePanel(ctx context.Context) error {
	embed := panelEmbed(c.cfg.BrandName)
	components := panelComponents()

	messageID, ok, err := c.state.Get(ctx, panelStateKey)
	if err != nil {
		return err
	}
	if ok && messageID != "" {
		_, err := c.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel:    c.cfg.PanelChannelID,
			ID:         messageID,
			Embeds:     &[]*discordgo.MessageEmbed{embed},
			Components: &components,
		})
		if err == nil {
			c.log.Info("vacation panel refreshed", slog.String("message_id", messageID))
			return nil
		}
		c.log.Warn("failed to edit existing panel, creating a new one", slog.String("message_id", messageID), slog.String("error", err.Error()))
	}

	message, err := c.session.ChannelMessageSendComplex(c.cfg.PanelChannelID, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	})
	if err != nil {
		return err
	}
	if err := c.state.Set(ctx, panelStateKey, message.ID); err != nil {
		return err
	}
	c.log.Info("vacation panel created", slog.String("message_id", message.ID))
	return nil
}
