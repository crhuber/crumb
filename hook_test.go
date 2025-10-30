package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"crumb/pkg/commands"
)

func TestHookCommandIntegration(t *testing.T) {
	tests := []struct {
		name          string
		shell         string
		wantContains  []string
		wantError     bool
		errorContains string
	}{
		{
			name:  "bash hook output",
			shell: "bash",
			wantContains: []string{
				"_crumb_hook()",
				"if [ -f .crumb.yaml ]",
				"export --shell bash",
				"PROMPT_COMMAND",
			},
			wantError: false,
		},
		{
			name:  "zsh hook output",
			shell: "zsh",
			wantContains: []string{
				"_crumb_hook()",
				"if [ -f .crumb.yaml ]",
				"export --shell bash",
				"precmd_functions",
				"chpwd_functions",
			},
			wantError: false,
		},
		{
			name:  "fish hook output",
			shell: "fish",
			wantContains: []string{
				"function _crumb_hook",
				"if test -f .crumb.yaml",
				"export --shell fish",
				"--on-variable PWD",
				"--on-event fish_prompt",
				"_crumb_hook",
			},
			wantError: false,
		},
		{
			name:          "unsupported shell",
			shell:         "powershell",
			wantError:     true,
			errorContains: "unsupported shell",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			var buf bytes.Buffer
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Create a mock command with the shell flag
			cmd := &cli.Command{
				Name:   "hook",
				Action: commands.HookCommand,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "shell",
						Usage: "Shell format (bash, zsh or fish)",
						Value: "bash",
					},
				},
			}

			// Build arguments
			args := []string{"hook"}
			if tt.shell != "" {
				args = append(args, "--shell", tt.shell)
			}

			// Execute command
			err := cmd.Run(context.Background(), args)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			buf.ReadFrom(r)
			output := buf.String()

			// Check error expectation
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v, output: %s", err, output)
				return
			}

			// Check that output contains expected strings
			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("output missing expected string %q\nGot output:\n%s", want, output)
				}
			}
		})
	}
}

func TestHookCommandExecutablePath(t *testing.T) {
	// Capture stdout
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Create a mock command with the shell flag
	cmd := &cli.Command{
		Name:   "hook",
		Action: commands.HookCommand,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "shell",
				Usage: "Shell format (bash, zsh or fish)",
				Value: "bash",
			},
		},
	}

	// Execute command
	err := cmd.Run(context.Background(), []string{"hook", "--shell", "bash"})

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("failed to run hook command: %v", err)
	}

	// The output should contain a reference to the crumb executable
	if !strings.Contains(output, "crumb") {
		t.Errorf("hook output should reference crumb executable, got: %s", output)
	}

	// Should contain the export command
	if !strings.Contains(output, "export --shell bash") {
		t.Errorf("hook output should contain export command, got: %s", output)
	}
}

func TestFishHookNoTabCharacters(t *testing.T) {
	// Capture stdout
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Create a mock command with the shell flag
	cmd := &cli.Command{
		Name:   "hook",
		Action: commands.HookCommand,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "shell",
				Usage: "Shell format (bash, zsh or fish)",
				Value: "bash",
			},
		},
	}

	// Execute command
	err := cmd.Run(context.Background(), []string{"hook", "--shell", "fish"})

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("failed to run hook command: %v", err)
	}

	// Fish hook should not contain any tab characters
	// Tab characters can cause issues with Fish shell parsing
	if strings.Contains(output, "\t") {
		t.Errorf("Fish hook output should not contain tab characters, found tabs in output")
	}

	// Verify the hook contains both expected functions
	if !strings.Contains(output, "function _crumb_hook --on-variable PWD") {
		t.Errorf("Fish hook should contain PWD change handler")
	}

	if !strings.Contains(output, "function _crumb_hook_prompt --on-event fish_prompt") {
		t.Errorf("Fish hook should contain prompt event handler")
	}

	// Verify the hook calls _crumb_hook immediately
	if !strings.Contains(output, "_crumb_hook") {
		t.Errorf("Fish hook should call _crumb_hook immediately after definition")
	}
}
