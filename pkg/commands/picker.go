package commands

import (
	"fmt"
	"os"

	fuzzyfinder "github.com/ktr0731/go-fuzzyfinder"
	"github.com/urfave/cli/v3"

	"crumb/pkg/storage"
)

func pickSecretPath(cmd *cli.Command) (string, error) {
	cfg, b, err := resolveBackend(cmd)
	if err != nil {
		return "", err
	}

	secrets, err := storage.LoadSecrets(cfg.PrivateKeyPath, b)
	if err != nil {
		return "", err
	}

	keys := storage.GetFilteredKeys(secrets, "")
	if len(keys) == 0 {
		return "", fmt.Errorf("no secrets found; create one first with 'crumb set'")
	}

	if len(keys) == 1 {
		fmt.Fprintf(os.Stderr, "Auto-selected: %s\n", keys[0])
		return keys[0], nil
	}

	idx, err := fuzzyfinder.Find(keys, func(i int) string {
		return keys[i]
	})
	if err != nil {
		if err == fuzzyfinder.ErrAbort {
			return "", fmt.Errorf("selection cancelled")
		}
		return "", err
	}

	return keys[idx], nil
}
