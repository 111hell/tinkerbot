package siu

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"myagent/internal/chat"
	"myagent/internal/conversation"
	"myagent/internal/events"
	"myagent/internal/run"
	siuapi "myagent/internal/siu"
)

// Channel owns only SIU protocol adaptation and delivery lifecycle.
type Channel struct {
	client     *siuapi.Client
	sessions   *chat.Service
	store      chat.Store
	logger     *slog.Logger
	listenAddr string
	webhookURL string
	tools      []string
}

func New(client *siuapi.Client, sessions *chat.Service, store chat.Store, logger *slog.Logger, listenAddr, webhookURL string, tools []string) *Channel {
	return &Channel{client: client, sessions: sessions, store: store, logger: logger, listenAddr: listenAddr, webhookURL: webhookURL, tools: append([]string(nil), tools...)}
}

func (c *Channel) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	bot, err := c.client.WhoAmI(ctx)
	if err != nil {
		return err
	}
	c.logger.Info("SIU bot authorized", "bot_id", bot.ID, "username", bot.Username)
	runs := run.NewManager()
	conversations := conversation.NewService(c.store, c.client)
	handler := events.NewHandler(c.logger, conversations, runs, c.sessions, c.client, c.tools)
	scheduler := events.NewScheduler(ctx, handler, 8, 100, 5*time.Minute)
	defer scheduler.Close()
	dispatcher := events.NewDispatcher(runs, scheduler)
	listener, err := net.Listen("tcp", c.listenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := c.client.SetWebhook(ctx, c.webhookURL); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("POST /webhook", events.NewWebhookHandler(dispatcher))
	server := &http.Server{Handler: mux}
	go func() { <-ctx.Done(); _ = server.Close() }()
	c.logger.Info("SIU channel started", "listen_addr", listener.Addr().String())
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
