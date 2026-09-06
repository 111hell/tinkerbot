package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/111hell/tinker/agent"
	"github.com/111hell/tinker/model/openaicompat"

	"myagent/internal/channel"
	"myagent/internal/channel/cli"
	siuchannel "myagent/internal/channel/siu"
	"myagent/internal/chat"
	"myagent/internal/config"
	"myagent/internal/logging"
	"myagent/internal/siu"
	"myagent/internal/skills"
	"myagent/internal/storage"
	"myagent/internal/tools"
	"myagent/internal/toolscope"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runMain(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("myagent stopped", "error", err)
		os.Exit(1)
	}
}

func runMain(ctx context.Context, args []string, input io.ReadCloser, output, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("myagent", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	path := flags.String("config", "config.yaml", "YAML configuration file")
	kind := flags.String("channel", "", "override channels (comma-separated: cli,siu)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("expected at most one configuration path")
	}
	if flags.NArg() == 1 {
		*path = flags.Arg(0)
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	if *kind != "" {
		configured := make(map[string]config.ChannelConfig)
		for _, ch := range cfg.EnabledChannels() {
			configured[ch.Type] = ch
		}
		cfg.Channel = config.ChannelConfig{}
		cfg.Channels = nil
		for _, name := range strings.Split(*kind, ",") {
			name = strings.TrimSpace(name)
			ch, ok := configured[name]
			if !ok {
				ch = config.ChannelConfig{Type: name}
			}
			cfg.Channels = append(cfg.Channels, ch)
		}
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)
	var client *siu.Client
	needSIU := false
	for _, ch := range cfg.EnabledChannels() {
		if ch.Type == "siu" || len(cfg.EnabledTools(ch)) > 0 {
			needSIU = true
		}
	}
	if needSIU {
		client, err = siu.NewClient(cfg.SIU.BaseURL, cfg.SIU.BotToken, &http.Client{Timeout: 30 * time.Second})
		if err != nil {
			return err
		}
	}
	model, err := openaicompat.New(openaicompat.Config{
		APIKey: cfg.Model.APIKey, BaseURL: cfg.Model.BaseURL, Model: cfg.Model.Name,
	})
	if err != nil {
		return err
	}
	var loaded []agent.Skill
	if cfg.SkillsDir != "" {
		loaded, err = skills.Load(cfg.SkillsDir)
		if err != nil {
			return err
		}
	}
	var registered []agent.Tool
	if client != nil {
		registered = toolscope.Guard(tools.All(client))
	}
	runtime, err := agent.New(agent.Config{
		Model: toolscope.Model{Model: model}, SystemPrompt: cfg.Agent.SystemPrompt, Skills: loaded, Tools: registered,
	})
	if err != nil {
		return err
	}
	var store chat.Store = chat.NewMemoryStore()
	var legacyStore chat.Store = chat.NewMemoryStore()
	if cfg.SQLite.Path != "" {
		db, err := storage.OpenSQLite(cfg.SQLite.Path)
		if err != nil {
			return err
		}
		defer db.Close()
		store = db.Sessions()
		legacyStore = db
	}
	sessions := chat.NewService(runtime, cfg.Agent.RunTimeout, store)
	var transports []channel.Channel
	for _, ch := range cfg.EnabledChannels() {
		switch ch.Type {
		case "cli":
			transports = append(transports, cli.New(input, output, diagnostics, sessions.Session("cli:default", cfg.EnabledTools(ch)...)))
		case "siu":
			transports = append(transports, siuchannel.New(client, sessions, legacyStore, logger, cfg.ListenAddr, cfg.SIU.WebhookURL, cfg.EnabledTools(ch)))
		}
	}
	return channel.RunAll(ctx, transports...)
}
