package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/111hell/tinkerbot/internal/channel"
)

type CLI struct {
	input       io.ReadCloser
	output      io.Writer
	diagnostics io.Writer
	session     channel.Session
}

// New takes ownership of input and closes it when Run returns.
func New(input io.ReadCloser, output, diagnostics io.Writer, session channel.Session) *CLI {
	return &CLI{input: input, output: output, diagnostics: diagnostics, session: session}
}

func (c *CLI) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer c.input.Close()
	type line struct {
		text string
		err  error
	}
	lines := make(chan line)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(c.input)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			select {
			case lines <- line{text: scanner.Text()}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case lines <- line{err: err}:
			case <-ctx.Done():
			}
		}
	}()
	if _, err := fmt.Fprintln(c.diagnostics, "TinkerBot · /new 清空会话 · /exit 退出 · Ctrl-D 结束输入"); err != nil {
		return err
	}
	for {
		if _, err := fmt.Fprint(c.diagnostics, "You: "); err != nil {
			return err
		}
		var next line
		select {
		case <-ctx.Done():
			return ctx.Err()
		case value, ok := <-lines:
			if !ok {
				return nil
			}
			next = value
		}
		if next.err != nil {
			return fmt.Errorf("read input: %w", next.err)
		}
		input := strings.TrimSpace(next.text)
		switch input {
		case "":
			continue
		case "/exit", "/quit":
			return nil
		case "/new":
			if err := c.session.Reset(ctx); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(c.diagnostics, "已清空会话。"); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprint(c.diagnostics, "Assistant: "); err != nil {
			return err
		}
		var outputErr error
		err := c.session.Reply(ctx, input, func(text string) error {
			_, outputErr = io.WriteString(c.output, text)
			return outputErr
		})
		if outputErr != nil {
			return outputErr
		}
		if _, writeErr := fmt.Fprintln(c.output); writeErr != nil {
			return writeErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if _, writeErr := fmt.Fprintf(c.diagnostics, "本轮失败（未保存到会话）：%v\n", err); writeErr != nil {
				return writeErr
			}
		}
	}
}
