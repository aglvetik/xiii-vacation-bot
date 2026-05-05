package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

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

	activeVacationsPanelMu     sync.Mutex
	activeVacationsPanelCancel context.CancelFunc
	activeVacationsPanelWG     sync.WaitGroup
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

	client.worker = scheduler.NewExpirationWorker(cfg, session, log, vacationSvc, notificationSvc, client.refreshActiveVacationsPanelAsync)
	client.registerHandlers()
	return client, nil
}

func (c *Client) Start(ctx context.Context) error {
	if err := c.session.Open(); err != nil {
		return fmt.Errorf("open discord gateway: %w", err)
	}
	if err := c.RegisterCommands(); err != nil {
		return fmt.Errorf("register application commands: %w", err)
	}
	if err := c.EnsurePanel(ctx); err != nil {
		return fmt.Errorf("ensure panel: %w", err)
	}
	if err := c.EnsureActiveVacationsPanel(ctx); err != nil {
		return fmt.Errorf("ensure active vacations panel: %w", err)
	}
	c.StartActiveVacationsPanelRefresher(ctx)
	c.worker.Start(ctx)
	return nil
}

func (c *Client) Stop() {
	c.StopActiveVacationsPanelRefresher()
	c.worker.Stop()
	c.session.Close()
}

func (c *Client) RegisterCommands() error {
	appID, err := c.applicationID()
	if err != nil {
		return err
	}

	command := &discordgo.ApplicationCommand{
		Name:        vacationsCommandName,
		Description: "Показать активные отпуска XIII",
	}
	if _, err := c.session.ApplicationCommandCreate(appID, c.cfg.GuildID, command); err != nil {
		return fmt.Errorf("create /%s guild command: %w", vacationsCommandName, err)
	}

	c.log.Info("guild application command registered", slog.String("command", vacationsCommandName), slog.String("guild_id", c.cfg.GuildID))
	return nil
}

func (c *Client) applicationID() (string, error) {
	if c.session.State != nil && c.session.State.User != nil && c.session.State.User.ID != "" {
		return c.session.State.User.ID, nil
	}

	user, err := c.session.User("@me")
	if err != nil {
		return "", fmt.Errorf("load current bot user: %w", err)
	}
	if user == nil || user.ID == "" {
		return "", fmt.Errorf("current bot user id is empty")
	}
	return user.ID, nil
}
