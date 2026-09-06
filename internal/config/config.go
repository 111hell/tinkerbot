package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
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

// EnabledTools resolves channel override, global default, then legacy defaults.
// Explicit empty lists are preserved and disable tools at that scope.
func (c *Config) EnabledTools(ch ChannelConfig) []string {
	if ch.Tools != nil {
		return ch.Tools
	}
	if c.Tools != nil {
		return c.Tools
	}
	if ch.Type == "siu" {
		return []string{"siu_list_messages", "siu_search_contacts"}
	}
	return nil
}

func (c *Config) EnabledChannels() []ChannelConfig {
	if c.Channels != nil {
		return c.Channels
	}
	if c.Channel.Type != "" {
		return []ChannelConfig{c.Channel}
	}
	return []ChannelConfig{{Type: "cli"}}
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
		case "cli", "siu":
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
	return nil
}

func validateTools(names []string) error {
	for _, name := range names {
		switch name {
		case "siu_list_messages", "siu_search_contacts":
		default:
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
	cfg := &Config{
		Agent: AgentConfig{
			SystemPrompt: "You are a helpful assistant. Answer the user directly and concisely. Never claim an action succeeded without evidence.",
			RunTimeout:   2 * time.Minute,
		},
		Model:      ModelConfig{BaseURL: "https://api.deepseek.com", Name: "deepseek-chat"},
		SQLite:     SQLiteConfig{Path: "myagent.db"},
		ListenAddr: ":8080",
		LogLevel:   "info",
	}
	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(data))), cfg); err != nil {
		return nil, fmt.Errorf("decode config %q: %w", path, err)
	}
	return cfg, nil
}
