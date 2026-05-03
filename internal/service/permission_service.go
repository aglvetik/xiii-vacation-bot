package service

import (
	"fmt"

	"xiii-vacation-bot/internal/config"

	"github.com/bwmarrin/discordgo"
)

type PermissionService struct {
	cfg     config.Config
	session *discordgo.Session
}

func NewPermissionService(cfg config.Config, session *discordgo.Session) *PermissionService {
	return &PermissionService{cfg: cfg, session: session}
}

func (s *PermissionService) CanReviewRequests(userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	guild, err := s.session.State.Guild(s.cfg.GuildID)
	if err != nil {
		guild, err = s.session.Guild(s.cfg.GuildID)
		if err != nil {
			return false, fmt.Errorf("load guild for permission check: %w", err)
		}
	}
	if guild.OwnerID == userID {
		return true, nil
	}

	member, err := s.session.GuildMember(s.cfg.GuildID, userID)
	if err != nil {
		return false, fmt.Errorf("load member for permission check: %w", err)
	}
	roles, err := s.session.GuildRoles(s.cfg.GuildID)
	if err != nil {
		return false, fmt.Errorf("load roles for permission check: %w", err)
	}
	channel, err := s.session.Channel(s.cfg.OfficerChannelID)
	if err != nil {
		return false, fmt.Errorf("load officer channel for permission check: %w", err)
	}

	permissions := computeMemberPermissions(guild.ID, member, roles, channel)
	if permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
		return true, nil
	}
	return permissions&discordgo.PermissionViewChannel == discordgo.PermissionViewChannel, nil
}

func computeMemberPermissions(guildID string, member *discordgo.Member, roles []*discordgo.Role, channel *discordgo.Channel) int64 {
	roleByID := make(map[string]*discordgo.Role, len(roles))
	for _, role := range roles {
		roleByID[role.ID] = role
	}

	var permissions int64
	if everyone := roleByID[guildID]; everyone != nil {
		permissions = everyone.Permissions
	}
	for _, roleID := range member.Roles {
		if role := roleByID[roleID]; role != nil {
			permissions |= role.Permissions
		}
	}
	if permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
		return permissions
	}

	for _, overwrite := range channel.PermissionOverwrites {
		if overwrite.ID == guildID && overwrite.Type == discordgo.PermissionOverwriteTypeRole {
			permissions &^= overwrite.Deny
			permissions |= overwrite.Allow
			break
		}
	}

	memberRoleSet := make(map[string]struct{}, len(member.Roles))
	for _, roleID := range member.Roles {
		memberRoleSet[roleID] = struct{}{}
	}

	var roleDeny int64
	var roleAllow int64
	for _, overwrite := range channel.PermissionOverwrites {
		if overwrite.Type != discordgo.PermissionOverwriteTypeRole {
			continue
		}
		if _, ok := memberRoleSet[overwrite.ID]; ok {
			roleDeny |= overwrite.Deny
			roleAllow |= overwrite.Allow
		}
	}
	permissions &^= roleDeny
	permissions |= roleAllow

	if member.User != nil {
		for _, overwrite := range channel.PermissionOverwrites {
			if overwrite.ID == member.User.ID && overwrite.Type == discordgo.PermissionOverwriteTypeMember {
				permissions &^= overwrite.Deny
				permissions |= overwrite.Allow
				break
			}
		}
	}

	return permissions
}
