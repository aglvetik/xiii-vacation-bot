package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordToken      string
	GuildID           string
	PanelChannelID    string
	OfficerChannelID  string
	OfficerPingRoleID string
	VacationRoleID    string
	DatabasePath      string
	BrandName         string
	MaxVacationDays   int
	LogLevel          string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		DiscordToken:      strings.TrimSpace(os.Getenv("DISCORD_TOKEN")),
		GuildID:           strings.TrimSpace(os.Getenv("GUILD_ID")),
		PanelChannelID:    strings.TrimSpace(os.Getenv("PANEL_CHANNEL_ID")),
		OfficerChannelID:  strings.TrimSpace(os.Getenv("OFFICER_CHANNEL_ID")),
		OfficerPingRoleID: strings.TrimSpace(os.Getenv("OFFICER_PING_ROLE_ID")),
		VacationRoleID:    strings.TrimSpace(os.Getenv("VACATION_ROLE_ID")),
		DatabasePath:      strings.TrimSpace(os.Getenv("DATABASE_PATH")),
		BrandName:         strings.TrimSpace(os.Getenv("BRAND_NAME")),
		LogLevel:          strings.TrimSpace(os.Getenv("LOG_LEVEL")),
	}

	if cfg.BrandName == "" {
		cfg.BrandName = "XIII"
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = "./data/vacations.db"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	maxDaysRaw := strings.TrimSpace(os.Getenv("MAX_VACATION_DAYS"))
	if maxDaysRaw == "" {
		cfg.MaxVacationDays = 30
	} else {
		maxDays, err := strconv.Atoi(maxDaysRaw)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_VACATION_DAYS must be an integer: %w", err)
		}
		cfg.MaxVacationDays = maxDays
	}

	if cfg.MaxVacationDays < 1 {
		return Config{}, errors.New("MAX_VACATION_DAYS must be greater than zero")
	}

	missing := make([]string, 0)
	required := map[string]string{
		"DISCORD_TOKEN":      cfg.DiscordToken,
		"GUILD_ID":           cfg.GuildID,
		"PANEL_CHANNEL_ID":   cfg.PanelChannelID,
		"OFFICER_CHANNEL_ID": cfg.OfficerChannelID,
		"VACATION_ROLE_ID":   cfg.VacationRoleID,
	}
	for key, value := range required {
		if value == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}
