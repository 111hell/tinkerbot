package document

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Adapted from Agentiu exec.Host: bounded output, no inherited credentials,
// and cancellation of the subprocess group (including the Python child of uv).
func runConverter(ctx context.Context, bin string, args []string, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	for _, name := range []string{"PATH", "HOME", "LANG", "TMPDIR", "UV_CACHE_DIR", "UV_PYTHON_INSTALL_DIR"} {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	stdout := &capBuffer{max: MaxTextBytes}
	stderr := &capBuffer{max: 4096}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("document: conversion interrupted: %w", ctx.Err())
	}
	if stdout.truncated {
		return nil, fmt.Errorf("document: conversion output exceeds 2 MiB")
	}
	if err != nil {
		// Wrapper errors are local; downloaded presigned URLs are never passed in.
		detail := []rune(strings.TrimSpace(string(stderr.data)))
		if len(detail) > 500 {
			detail = detail[len(detail)-500:]
		}
		return nil, fmt.Errorf("document: conversion failed (check dependencies with uv run --script scripts/convert-document.py --check): %v: %s", err, string(detail))
	}
	return stdout.data, nil
}

type capBuffer struct {
	data      []byte
	max       int
	truncated bool
}

func (b *capBuffer) Write(p []byte) (int, error) {
	keep := min(len(p), b.max-len(b.data))
	b.data = append(b.data, p[:keep]...)
	b.truncated = b.truncated || keep < len(p)
	return len(p), nil
}
