package bot

import (
	"context"
	"fmt"
	"log/slog"

	"xiii-vacation-bot/internal/config"
	"xiii-vacation-bot/internal/database"
	"xiii-vacation-bot/internal/scheduler"
	"xiii-vacation-bot/internal/service"

	"github.com/bwmarrin/discordgo"
)

type Client struct {
	cfg           config.Config
	db            *database.DB
	log           *slog.Logger
	session       *discordgo.Session
	state         *database.StateRepository
	requests      *database.RequestRepository
	vacations     *database.VacationRepository
	vacationSvc   *service.VacationService
	permissionSvc *service.PermissionService
	notification  *service.NotificationService
	worker        *scheduler.ExpirationWorker
}

func New(cfg config.Config, db *database.DB, log *slog.Logger) (*Client, error) {
	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMembers |
		discordgo.IntentsDirectMessages

	stateRepo := database.NewStateRepository(db)
	requestRepo := database.NewRequestRepository(db)
	vacationRepo := database.NewVacationRepository(db)
	vacationSvc := service.NewVacationService(cfg, db, requestRepo, vacationRepo)
	notificationSvc := service.NewNotificationService(cfg, session, log)
	permissionSvc := service.NewPermissionService(cfg, session)

	client := &Client{
		cfg:           cfg,
		db:            db,
		log:           log,
		session:       session,
		state:         stateRepo,
		requests:      requestRepo,
		vacations:     vacationRepo,
		vacationSvc:   vacationSvc,
		permissionSvc: permissionSvc,
		notification:  notificationSvc,
	}

	client.worker = scheduler.NewExpirationWorker(cfg, session, log, vacationSvc, notificationSvc)
	client.registerHandlers()
	return client, nil
}

func (c *Client) Start(ctx context.Context) error {
	if err := c.session.Open(); err != nil {
		return fmt.Errorf("open discord gateway: %w", err)
	}
	if err := c.EnsurePanel(ctx); err != nil {
		return fmt.Errorf("ensure panel: %w", err)
	}
	c.worker.Start(ctx)
	return nil
}

func (c *Client) Stop() {
	c.worker.Stop()
	c.session.Close()
}
