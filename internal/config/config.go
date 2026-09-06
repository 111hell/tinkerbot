package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/111hell/tinkerbot/internal/tools"
)

const (
	ChannelCLI = "cli"
	ChannelSIU = "siu"
)

type Config struct {
	Tools      []string        `yaml:"tools"`
	Channels   []ChannelConfig `yaml:"channels"`
	Channel    ChannelConfig   `yaml:"channel"`
	Agent      AgentConfig     `yaml:"agent"`
	Model      ModelConfig     `yaml:"model"`
	SIU        SIUConfig       `yaml:"siu"`
	SQLite     SQLiteConfig    `yaml:"sqlite"`
	ListenAddr string          `yaml:"listen_addr"`
	SkillsDir  string          `yaml:"skills_dir"`
	LogLevel   string          `yaml:"log_level"`
}

type ChannelConfig struct {
	Type  string   `yaml:"type"`
	Tools []string `yaml:"tools"`
}

// EnabledTools combines global and channel tools. When neither is configured,
// it falls back to the legacy channel defaults.
func (c *Config) EnabledTools(ch ChannelConfig) []string {
	if c.Tools == nil && ch.Tools == nil {
		switch ch.Type {
		case ChannelSIU:
			return tools.DefaultSIUNames()
		default:
			return nil
		}
	}
	return mergeTools(c.Tools, ch.Tools)
}

func mergeTools(lists ...[]string) []string {
	merged := make([]string, 0)
	seen := make(map[string]struct{})
	for _, list := range lists {
		for _, name := range list {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			merged = append(merged, name)
		}
	}
	return merged
}

func (c *Config) EnabledChannels() []ChannelConfig {
	if c.Channels != nil {
		return c.Channels
	}
	if c.Channel.Type != "" {
		return []ChannelConfig{c.Channel}
	}
	return []ChannelConfig{{Type: ChannelCLI}}
}

type AgentConfig struct {
	SystemPrompt string        `yaml:"system_prompt"`
	RunTimeout   time.Duration `yaml:"run_timeout"`
}

func (c *Config) Validate() error {
	if err := validateTools(c.Tools); err != nil {
		return fmt.Errorf("global tools: %w", err)
	}
	if c.Channels != nil && c.Channel.Type != "" {
		return fmt.Errorf("configure channels or legacy channel, not both")
	}
	channels := c.EnabledChannels()
	if len(channels) == 0 {
		return fmt.Errorf("at least one channel is required")
	}
	seen := make(map[string]bool)
	for _, ch := range channels {
		switch ch.Type {
		case ChannelCLI:
		case ChannelSIU:
			if strings.TrimSpace(c.ListenAddr) == "" {
				return fmt.Errorf("listen_addr is required for siu channel")
			}
		default:
			return fmt.Errorf("unsupported channel %q (available: cli, siu)", ch.Type)
		}
		if seen[ch.Type] {
			return fmt.Errorf("duplicate channel %q", ch.Type)
		}
		seen[ch.Type] = true
		if err := validateTools(c.EnabledTools(ch)); err != nil {
			return fmt.Errorf("channel %q: %w", ch.Type, err)
		}
	}
	if c.Agent.RunTimeout <= 0 {
		return fmt.Errorf("agent.run_timeout must be positive")
	}
	if strings.TrimSpace(c.Model.BaseURL) == "" {
		return fmt.Errorf("model.base_url is required")
	}
	if strings.TrimSpace(c.Model.Name) == "" {
		return fmt.Errorf("model.name is required")
	}
	if strings.TrimSpace(c.LogLevel) == "" {
		return fmt.Errorf("log_level is required")
	}
	return nil
}

func validateTools(names []string) error {
	for _, name := range names {
		if !tools.IsKnown(name) {
			return fmt.Errorf("unknown tool %q", name)
		}
	}
	return nil
}

type ModelConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Name    string `yaml:"name"`
}

type SIUConfig struct {
	BaseURL    string `yaml:"base_url"`
	BotToken   string `yaml:"bot_token"`
	WebhookURL string `yaml:"webhook_url"`
}

type SQLiteConfig struct {
	Path string `yaml:"path"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(data))), cfg); err != nil {
		return nil, fmt.Errorf("decode config %q: %w", path, err)
	}
	return cfg, nil
}
