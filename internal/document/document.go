// Package document adapts Agentiu's MarkItDown attachment extraction without
// its smolmem retention layer. Originals live only for one conversion.
package document

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxBytes      = 20 << 20
	MaxTotalBytes = 50 << 20
	MaxFiles      = 5
	MaxTextBytes  = 2 << 20
	Timeout       = 30 * time.Second
)

type Clip struct {
	Data     []byte
	Filename string
}

type Result struct{ Filename, Format, Text string }

type Converter struct{ script string }

// New uses the same pinned, offline uv script as Agentiu. Resolve once before
// any subprocess changes directory. Dependencies must be installed separately.
func New(script string) (*Converter, error) {
	path, err := filepath.Abs(script)
	if err != nil {
		return nil, err
	}
	return &Converter{script: path}, nil
}

func (c *Converter) Convert(ctx context.Context, clip Clip) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if len(clip.Data) > MaxBytes {
		return Result{}, fmt.Errorf("document: file exceeds 20 MiB")
	}
	format, err := Sniff(clip.Data, clip.Filename)
	if err != nil {
		return Result{}, err
	}
	uv, err := exec.LookPath("uv")
	if err != nil {
		return Result{}, fmt.Errorf("document: uv is required; install uv and run uv run --script scripts/convert-document.py --check")
	}
	if _, err := os.Stat(c.script); err != nil {
		return Result{}, fmt.Errorf("document: converter script unavailable; start TinkerBot from its project directory")
	}
	dir, err := os.MkdirTemp("", "tinkerbot-document-*")
	if err != nil {
		return Result{}, fmt.Errorf("document: create temporary directory: %w", err)
	}
	defer os.RemoveAll(dir)
	// Never use a user-supplied filename as a path or command argument.
	path := filepath.Join(dir, "input."+format)
	if err := os.WriteFile(path, clip.Data, 0600); err != nil {
		return Result{}, fmt.Errorf("document: save input: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	out, err := runConverter(ctx, uv, []string{"run", "--offline", "--no-python-downloads", "--script", c.script, path}, dir)
	if err != nil {
		return Result{}, err
	}
	if !utf8.Valid(out) {
		return Result{}, fmt.Errorf("document: converter returned invalid UTF-8")
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return Result{}, fmt.Errorf("document: no extractable text; scanned or image-only files require OCR")
	}
	filename := filepath.Base(clip.Filename)
	if filename == "." || filename == string(filepath.Separator) || strings.TrimSpace(filename) == "" {
		filename = "file." + format
	}
	filename = string([]rune(filename)[:min(255, utf8.RuneCountInString(filename))])
	return Result{Filename: filename, Format: format, Text: text}, nil
}
