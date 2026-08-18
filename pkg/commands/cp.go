package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"

	"crumb/pkg/config"
	"crumb/pkg/storage"
)

// Provider pushes a single secret to an external system.
type Provider interface {
	PushSecret(name, value string) error
}

// githubProvider pushes secrets to a GitHub repository via the gh CLI.
type githubProvider struct {
	repo string
}

func (g githubProvider) PushSecret(name, value string) error {
	cmd := exec.Command("gh", "secret", "set", name, "--repo", g.repo) // #nosec G204 -- name/repo are user-supplied identifiers; the secret value is piped via stdin, never passed as a CLI arg
	cmd.Stdin = strings.NewReader(value)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// newProvider constructs a Provider for the given provider name, failing fast
// if the required external tooling isn't available.
func newProvider(name, destination string) (Provider, error) {
	switch name {
	case "github":
		if _, err := exec.LookPath("gh"); err != nil {
			return nil, fmt.Errorf("gh CLI not found on PATH — install from https://cli.github.com")
		}
		return githubProvider{repo: destination}, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q (supported: github)", name)
	}
}

// collectSecretsToCopy resolves a key path to the set of secrets it refers to,
// mapping each to the name it should be pushed under. A trailing slash on path
// selects every secret under that prefix; otherwise path names a single secret.
func collectSecretsToCopy(secrets storage.SecretStore, path string) (map[string]string, error) {
	toCopy := make(map[string]string)

	if pathPrefix, isPathPrefix := strings.CutSuffix(path, "/"); isPathPrefix {
		for secretPath, secretValue := range storage.GetSecretsForPath(secrets, pathPrefix) {
			if name := storage.ConvertPathToEnvVar(secretPath, pathPrefix); name != "" {
				toCopy[name] = secretValue
			}
		}
	} else if entry, exists := storage.SecretExists(secrets, path); exists {
		if name := storage.ExtractVarName(path); name != "" {
			toCopy[name] = entry.Value
		}
	}

	if len(toCopy) == 0 {
		return nil, fmt.Errorf("no secrets found at path %q", path)
	}

	return toCopy, nil
}

// CpCommand copies secrets from a path to an external provider.
func CpCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 2 {
		return fmt.Errorf("usage: crumb cp <path> <destination>")
	}
	path := cmd.Args().Get(0)
	destination := cmd.Args().Get(1)

	if err := config.ValidateKeyPath(path); err != nil {
		return err
	}

	provider, err := newProvider(cmd.String("provider"), destination)
	if err != nil {
		return err
	}

	cfg, b, err := resolveBackend(cmd)
	if err != nil {
		return err
	}

	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, b)
	if err != nil {
		return err
	}

	toCopy, err := collectSecretsToCopy(secrets, path)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(toCopy))
	for name := range toCopy {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("Copying %d secret(s) from %s to %s via %s:\n", len(names), path, destination, cmd.String("provider"))
	for _, name := range names {
		fmt.Printf("  - %s\n", name)
	}

	if !cmd.Bool("yes") {
		fmt.Print("Proceed? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	for _, name := range names {
		if err := provider.PushSecret(name, toCopy[name]); err != nil {
			return fmt.Errorf("failed to copy %s: %w", name, err)
		}
	}

	fmt.Printf("Copied %d secret(s) to %s.\n", len(names), destination)
	return nil
}
