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

	// Verify the hook contains the PWD change handler
	if !strings.Contains(output, "function _crumb_hook --on-variable PWD") {
		t.Errorf("Fish hook should contain PWD change handler")
	}

	// Verify the hook does NOT contain the prompt event handler (should only run on directory change)
	if strings.Contains(output, "function _crumb_hook_prompt --on-event fish_prompt") {
		t.Errorf("Fish hook should NOT contain prompt event handler - should only run on directory change")
	}

	// Verify the hook calls _crumb_hook immediately
	if !strings.Contains(output, "_crumb_hook") {
		t.Errorf("Fish hook should call _crumb_hook immediately after definition")
	}
}

func TestHookOutputSilent(t *testing.T) {
	shells := []string{"bash", "zsh", "fish"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
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
			err := cmd.Run(context.Background(), []string{"hook", "--shell", shell})

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			buf.ReadFrom(r)
			output := buf.String()

			if err != nil {
				t.Fatalf("failed to run hook command: %v", err)
			}

			// Verify the hook does NOT contain echo statements for "Loading crumb secrets..."
			// This ensures the hooks run silently without printing messages to the terminal
			if strings.Contains(output, "Loading crumb secrets") {
				t.Errorf("%s hook should not contain 'Loading crumb secrets' message, found in output:\n%s", shell, output)
			}

			// Verify that the export command is still present (functionality preserved)
			if !strings.Contains(output, "export --shell") {
				t.Errorf("%s hook should still contain export command, got: %s", shell, output)
			}
		})
	}
}

func TestHookOnlyRunsOnDirectoryChange(t *testing.T) {
	tests := []struct {
		name             string
		shell            string
		shouldNotContain []string
		shouldContain    []string
	}{
		{
			name:  "bash hook only runs on directory change",
			shell: "bash",
			shouldContain: []string{
				"_CRUMB_LAST_DIR",              // Tracks previous directory
				"$PWD\" != \"$_CRUMB_LAST_DIR", // Compares current vs last directory
			},
			shouldNotContain: []string{
				// Bash uses PROMPT_COMMAND but with directory tracking, so no specific anti-pattern
			},
		},
		{
			name:  "zsh hook only runs on directory change",
			shell: "zsh",
			shouldContain: []string{
				"chpwd_functions", // Directory change hook
			},
			shouldNotContain: []string{
				"precmd_functions", // Should NOT use prompt hook
			},
		},
		{
			name:  "fish hook only runs on directory change",
			shell: "fish",
			shouldContain: []string{
				"--on-variable PWD", // Directory change trigger
			},
			shouldNotContain: []string{
				"--on-event fish_prompt", // Should NOT use prompt event
			},
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

			// Execute command
			err := cmd.Run(context.Background(), []string{"hook", "--shell", tt.shell})

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			buf.ReadFrom(r)
			output := buf.String()

			if err != nil {
				t.Fatalf("failed to run hook command: %v", err)
			}

			// Verify required patterns are present
			for _, pattern := range tt.shouldContain {
				if !strings.Contains(output, pattern) {
					t.Errorf("%s hook missing required pattern %q for directory-change-only behavior\nGot output:\n%s",
						tt.shell, pattern, output)
				}
			}

			// Verify unwanted patterns are absent
			for _, pattern := range tt.shouldNotContain {
				if strings.Contains(output, pattern) {
					t.Errorf("%s hook should NOT contain %q (would cause execution on every prompt)\nGot output:\n%s",
						tt.shell, pattern, output)
				}
			}
		})
	}
}
