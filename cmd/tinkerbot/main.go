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

	"github.com/111hell/tinkerbot/internal/channel"
	"github.com/111hell/tinkerbot/internal/channel/cli"
	siuchannel "github.com/111hell/tinkerbot/internal/channel/siu"
	"github.com/111hell/tinkerbot/internal/chat"
	"github.com/111hell/tinkerbot/internal/config"
	"github.com/111hell/tinkerbot/internal/logging"
	"github.com/111hell/tinkerbot/internal/siu"
	"github.com/111hell/tinkerbot/internal/skills"
	"github.com/111hell/tinkerbot/internal/storage"
	"github.com/111hell/tinkerbot/internal/tools"
	webtools "github.com/111hell/tinkerbot/internal/tools/web"
	"github.com/111hell/tinkerbot/internal/toolscope"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runMain(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("tinkerbot stopped", "error", err)
		os.Exit(1)
	}
}

func runMain(ctx context.Context, args []string, input io.ReadCloser, output, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("tinkerbot", flag.ContinueOnError)
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
		switch ch.Type {
		case config.ChannelSIU:
			needSIU = true
		default:
			needSIU = tools.RequiresSIU(cfg.EnabledTools(ch))
		}
		if needSIU {
			break
		}
	}
	if needSIU {
		client, err = siu.NewClient(cfg.SIU.BaseURL, cfg.SIU.BotToken, &http.Client{Timeout: 40 * time.Second})
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
	var legacySkills []agent.Skill
	var skillCatalog string
	var skillTool *agent.Tool
	if cfg.SkillsDir != "" {
		defs, err := skills.LoadDefinitions(cfg.SkillsDir)
		if err != nil {
			return err
		}
		var progressive []skills.Definition
		for _, d := range defs {
			if d.Description != "" {
				progressive = append(progressive, d)
			} else {
				legacySkills = append(legacySkills, agent.Skill{Name: d.Name, Instructions: d.Instructions})
			}
		}
		if len(progressive) > 0 {
			skillCatalog = skills.Catalog(progressive)
			t := skills.NewLoadSkillTool(progressive)
			skillTool = &t
		}
	}
	registered := webtools.NewClient(cfg.Web.APIKey).Tools()
	if client != nil {
		registered = append(registered, tools.All(client)...)
	}
	if skillTool != nil {
		registered = append(registered, *skillTool)
	}
	registered = toolscope.Guard(registered)

	sysPrompt := config.AssistantPrompt + "\n\n" + cfg.Agent.SystemPrompt
	if skillCatalog != "" {
		sysPrompt += "\n\n" + skillCatalog
	}

	runtime, err := agent.New(agent.Config{
		Model: toolscope.Model{Model: model}, SystemPrompt: sysPrompt, Skills: legacySkills, Tools: registered,
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
	sessions := chat.NewService(runtime, cfg.Agent.RunTimeout, store,
		chat.WithMaxHistoryMessages(cfg.Agent.MaxHistoryMessages),
		chat.WithMaxSteps(cfg.Agent.MaxSteps),
		chat.WithMemoryCompaction(true),
	)
	var transports []channel.Channel
	for _, ch := range cfg.EnabledChannels() {
		switch ch.Type {
		case config.ChannelCLI:
			transports = append(transports, cli.New(input, output, diagnostics, sessions.Session("cli:default", cfg.EnabledTools(ch)...)))
		case config.ChannelSIU:
			transports = append(transports, siuchannel.New(client, sessions, legacyStore, logger, cfg.ListenAddr, cfg.SIU.WebhookURL, cfg.SIU.ReceiveMode, cfg.EnabledTools(ch)))
		default:
			return fmt.Errorf("unsupported channel %q", ch.Type)
		}
	}
	return channel.RunAll(ctx, transports...)
}
