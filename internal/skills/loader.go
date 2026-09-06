package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/111hell/tinker/agent"
	"gopkg.in/yaml.v3"
)

func Load(directory string) ([]agent.Skill, error) {
	root := os.DirFS(directory)
	paths, err := fs.Glob(root, "*/SKILL.md")
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}

	loaded := make([]agent.Skill, 0, len(paths))
	for _, skillPath := range paths {
		content, err := fs.ReadFile(root, skillPath)
		if err != nil {
			return nil, fmt.Errorf("read skill %q: %w", skillPath, err)
		}
		skill, err := parse(content)
		if err != nil {
			return nil, fmt.Errorf("parse skill %q: %w", skillPath, err)
		}
		loaded = append(loaded, skill)
	}
	return loaded, nil
}

func parse(content []byte) (agent.Skill, error) {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return agent.Skill{}, errors.New("missing YAML frontmatter")
	}
	frontmatter, body, found := strings.Cut(normalized[4:], "\n---\n")
	if !found {
		return agent.Skill{}, errors.New("unterminated YAML frontmatter")
	}
	var metadata struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
		return agent.Skill{}, err
	}
	return agent.Skill{
		Name:         strings.TrimSpace(metadata.Name),
		Instructions: strings.TrimSpace(body),
	}, nil
}
