package bot

import (
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

func (c *Client) registerHandlers() {
	c.session.AddHandler(c.onReady)
	c.session.AddHandler(c.onInteractionCreate)
}

func (c *Client) onReady(_ *discordgo.Session, ready *discordgo.Ready) {
	if ready.User == nil {
		c.log.Info("discord gateway ready")
		return
	}
	c.log.Info("discord gateway ready", slog.String("bot_user", ready.User.String()))
}

func (c *Client) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i == nil || i.Interaction == nil {
		return
	}

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		c.handleApplicationCommand(s, i)
	case discordgo.InteractionMessageComponent:
		c.handleMessageComponent(s, i)
	case discordgo.InteractionModalSubmit:
		c.handleModalSubmit(s, i)
	default:
		return
	}
}
