package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/111hell/tinker/agent"
	"gopkg.in/yaml.v3"
)

type Definition struct {
	Name         string
	Description  string
	Instructions string
}

func LoadDefinitions(directory string) ([]Definition, error) {
	root := os.DirFS(directory)
	paths, err := fs.Glob(root, "*/SKILL.md")
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}

	loaded := make([]Definition, 0, len(paths))
	for _, skillPath := range paths {
		content, err := fs.ReadFile(root, skillPath)
		if err != nil {
			return nil, fmt.Errorf("read skill %q: %w", skillPath, err)
		}
		def, err := parse(content)
		if err != nil {
			return nil, fmt.Errorf("parse skill %q: %w", skillPath, err)
		}
		loaded = append(loaded, def)
	}
	return loaded, nil
}

func Load(directory string) ([]agent.Skill, error) {
	defs, err := LoadDefinitions(directory)
	if err != nil {
		return nil, err
	}
	skills := make([]agent.Skill, len(defs))
	for i, d := range defs {
		skills[i] = agent.Skill{Name: d.Name, Instructions: d.Instructions}
	}
	return skills, nil
}

func parse(content []byte) (Definition, error) {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return Definition{}, errors.New("missing YAML frontmatter")
	}
	frontmatter, body, found := strings.Cut(normalized[4:], "\n---\n")
	if !found {
		return Definition{}, errors.New("unterminated YAML frontmatter")
	}
	var metadata struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
		return Definition{}, err
	}
	name := strings.TrimSpace(metadata.Name)
	if name == "" {
		return Definition{}, errors.New("skill name is required")
	}
	return Definition{
		Name:         name,
		Description:  strings.TrimSpace(metadata.Description),
		Instructions: strings.TrimSpace(body),
	}, nil
}

func Catalog(defs []Definition) string {
	if len(defs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Available skills:\n")
	for _, d := range defs {
		desc := d.Description
		if desc == "" {
			desc = "Specialized capability"
		}
		fmt.Fprintf(&b, "- %s: %s\n", d.Name, desc)
	}
	b.WriteString("When relevant to fulfilling the user's request, use the load_skill tool to load the complete instructions.")
	return b.String()
}

func NewLoadSkillTool(defs []Definition) agent.Tool {
	store := make(map[string]string, len(defs))
	for _, d := range defs {
		store[d.Name] = d.Instructions
	}
	return agent.Tool{
		Name:        "load_skill",
		Description: "Load detailed instructions and guidelines for an available skill by name.",
		InputSchema: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`,
		Execute: func(ctx context.Context, arguments string) (string, error) {
			var args struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal([]byte(arguments), &args); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			instructions, ok := store[args.Name]
			if !ok {
				return "", fmt.Errorf("skill %q not found", args.Name)
			}
			return instructions, nil
		},
	}
}
