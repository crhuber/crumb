package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"crumb/pkg/commands"
)

// Version information (injected by GoReleaser)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cli.VersionPrinter = func(cmd *cli.Command) {
		fmt.Printf("version=%s commit=%s date=%s\n", cmd.Root().Version, commit, date)
	}
	cmd := &cli.Command{
		Name:    "crumb",
		Usage:   "Securely store, manage, and export API keys and secrets",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "profile",
				Usage:   "Profile to use for configuration",
				Value:   "default",
				Sources: cli.EnvVars("CRUMB_PROFILE"),
			},
			&cli.StringFlag{
				Name:    "storage",
				Usage:   "Storage file path",
				Sources: cli.EnvVars("CRUMB_STORAGE"),
			},
		},
		Commands: []*cli.Command{
			{
				Name:   "setup",
				Usage:  "Initialize the secure storage backend",
				Action: commands.SetupCommand,
			},
			{
				Name:      "list",
				Aliases:   []string{"ls"},
				Usage:     "List stored secret keys",
				Action:    commands.ListCommand,
				ArgsUsage: "[path]",
			},
			{
				Name:      "set",
				Usage:     "Add or update a secret key-value pair",
				Action:    commands.SetCommand,
				ArgsUsage: "<key-path>",
			},
			{
				Name:      "get",
				Usage:     "Retrieve a secret by its key path",
				Action:    commands.GetCommand,
				ArgsUsage: "<key-path>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "show",
						Usage: "Display the actual secret value instead of masking it",
					},
					&cli.BoolFlag{
						Name:  "export",
						Usage: "Output in shell-compatible format for sourcing",
					},
					&cli.StringFlag{
						Name:  "shell",
						Usage: "Shell format for export (bash or fish)",
						Value: "bash",
					},
				},
			},
			{
				Name:   "init",
				Usage:  "Create a YAML configuration file in current directory",
				Action: commands.InitCommand,
			},
			{
				Name:      "delete",
				Aliases:   []string{"rm"},
				Usage:     "Delete a secret key-value pair",
				Action:    commands.DeleteCommand,
				ArgsUsage: "<key-path>",
			},
			{
				Name:      "move",
				Aliases:   []string{"mv"},
				Usage:     "Rename a secret key to a new path (preserves value)",
				Action:    commands.MoveCommand,
				ArgsUsage: "<old-key-path> <new-key-path>",
			},
			{
				Name:      "import",
				Usage:     "Import secrets from a .env file",
				Action:    commands.ImportCommand,
				ArgsUsage: "--file <path> --path <destination-path>",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "file",
						Aliases:  []string{"f"},
						Usage:    "Path to .env file to import",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "path",
						Aliases:  []string{"p"},
						Usage:    "Destination path where secrets will be stored (e.g., /dev/foo)",
						Required: true,
					},
				},
			},
			{
				Name:  "export",
				Usage: "Export secrets as shell-compatible environment variables",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "shell",
						Usage: "Shell format (bash or fish)",
						Value: "bash",
					},
					&cli.StringFlag{
						Name:    "file",
						Aliases: []string{"f"},
						Usage:   "Configuration file to use (default: .crumb.yaml)",
						Value:   ".crumb.yaml",
					},
					&cli.StringFlag{
						Name:  "path",
						Usage: "Export all secrets from a specific path (bypasses .crumb.yaml)",
					},
					&cli.StringFlag{
						Name:  "env",
						Usage: "Environment to export from .crumb.yaml (default: default)",
						Value: "default",
					},
				},
				Action: commands.ExportCommand,
			},
			{
				Name:      "hook",
				Usage:     "Output shell hook script for automatic secret loading",
				ArgsUsage: "<shell>",
				Action:    commands.HookCommand,
			},
			{
				Name:  "storage",
				Usage: "Manage storage file configuration",
				Commands: []*cli.Command{
					{
						Name:      "set",
						Usage:     "Set storage file path for current profile",
						ArgsUsage: "<path>",
						Action:    commands.StorageSetCommand,
					},
					{
						Name:   "get",
						Usage:  "Show current storage file path for current profile",
						Action: commands.StorageGetCommand,
					},
					{
						Name:   "clear",
						Usage:  "Clear storage file path for current profile (use default)",
						Action: commands.StorageClearCommand,
					},
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
