package mcp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner executes the docmap CLI and returns stdout.
type Runner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

// ExecRunner shells out to a docmap binary.
type ExecRunner struct {
	// Binary is the path to the docmap executable. Empty uses the current
	// process executable (so `docmap mcp` wraps itself).
	Binary string
}

func (r ExecRunner) binary() (string, error) {
	if r.Binary != "" {
		return r.Binary, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve docmap binary: %w", err)
	}
	return exe, nil
}

// Run executes docmap with the given args and returns stdout (stderr on failure).
func (r ExecRunner) Run(ctx context.Context, args ...string) (string, error) {
	bin, err := r.binary()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("docmap %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
