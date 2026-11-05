// Package flk calls the flk CLI, flkforge's only window into flk (SPEC 2.3):
// `flk config show --json` to validate a flk.toml, `flk help --all --json`
// for flk's version and schema, `flk apps list --json` for host hints.
// Never flk's state directory.
package flk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Schema is the newest `flk help --all --json` schema flkforge knows.
const Schema = 1

// Runner runs `flk ARGS...` in a directory and returns stdout.
type Runner interface {
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// Error is a failed flk command; Stderr is flk's own message.
type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	if s := strings.TrimSpace(e.Stderr); s != "" {
		return strings.TrimPrefix(s, "Error: ")
	}
	return fmt.Sprintf("flk %s: %v", strings.Join(e.Args, " "), e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// Exec runs the real binary.
type Exec struct{ Path string }

// Find resolves the flk binary: $FLKFORGE_FLK, then PATH.
func Find() (Exec, error) {
	p := os.Getenv("FLKFORGE_FLK")
	if p == "" {
		p = "flk"
	}
	path, err := exec.LookPath(p)
	if err != nil {
		return Exec{}, fmt.Errorf("flk isn't on PATH (%s): install it, or set FLKFORGE_FLK", p)
	}
	return Exec{Path: path}, nil
}

func (e Exec) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, e.Path, args...)
	cmd.Dir = dir
	cmd.Stdin = nil
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), &Error{Args: args, Stderr: errb.String(), Err: err}
	}
	return out.Bytes(), nil
}

// Client runs flk commands with a timeout.
type Client struct {
	Runner  Runner
	Timeout time.Duration
}

func New(r Runner) *Client { return &Client{Runner: r, Timeout: 30 * time.Second} }

func (c *Client) run(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	return c.Runner.Run(ctx, dir, args...)
}

// Config is `flk config show --json`: what flk understood from a flk.toml.
type Config struct {
	File          string         `json:"file"`
	Parsed        map[string]any `json:"parsed"`
	MachineConfig map[string]any `json:"machine_config"`
	Warnings      []string       `json:"warnings"`
}

// ConfigShow parses DIR/flk.toml with flk, without registering or
// deploying anything. With app set, it reads that registered app's
// flk.toml instead (read-only: `flk config show --json -a APP`).
func (c *Client) ConfigShow(dir, app string) (*Config, error) {
	args := []string{"config", "show", "--json"}
	if app != "" {
		args = append(args, "-a", app)
	}
	out, err := c.run(dir, args...)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(out, &cfg); err != nil {
		return nil, fmt.Errorf("flk config show --json: %v", err)
	}
	return &cfg, nil
}

// Help is the part of `flk help --all --json` flkforge reads.
type Help struct {
	Schema  int    `json:"schema"`
	Version string `json:"version"`
}

// HelpAll reads flk's version and schema.
func (c *Client) HelpAll() (*Help, error) {
	out, err := c.run("", "help", "--all", "--json")
	if err != nil {
		return nil, err
	}
	var h Help
	if err := json.Unmarshal(out, &h); err != nil {
		return nil, fmt.Errorf("flk help --all --json: %v", err)
	}
	return &h, nil
}

// App is one entry of `flk apps list --json`.
type App struct {
	Name       string `json:"name"`
	ConfigPath string `json:"config_path"`
}

// Apps lists the registered apps.
func (c *Client) Apps() ([]App, error) {
	out, err := c.run("", "apps", "list", "--json")
	if err != nil {
		return nil, err
	}
	var apps []App
	if err := json.Unmarshal(out, &apps); err != nil {
		return nil, fmt.Errorf("flk apps list --json: %v", err)
	}
	return apps, nil
}

// Volume is one entry of `flk volumes list --json`.
type Volume struct {
	Name   string `json:"name"`
	SizeGB int    `json:"size_gb"`
}

// Volumes lists a registered app's volumes.
func (c *Client) Volumes(app string) ([]Volume, error) {
	out, err := c.run("", "volumes", "list", "--json", "-a", app)
	if err != nil {
		return nil, err
	}
	var vols []Volume
	if err := json.Unmarshal(out, &vols); err != nil {
		return nil, fmt.Errorf("flk volumes list --json: %v", err)
	}
	return vols, nil
}
