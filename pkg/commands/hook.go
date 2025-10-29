package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

// HookCommand handles the hook command for shell integration
func HookCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("usage: crumb hook <shell>\nSupported shells: bash, zsh, fish")
	}

	shell := cmd.Args().Get(0)
	
	// Get the path to the crumb binary
	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	selfPath, err = filepath.EvalSymlinks(selfPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	var hookScript string
	switch shell {
	case "bash":
		hookScript = bashHook(selfPath)
	case "zsh":
		hookScript = zshHook(selfPath)
	case "fish":
		hookScript = fishHook(selfPath)
	default:
		return fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish)", shell)
	}

	fmt.Print(hookScript)
	return nil
}

func bashHook(selfPath string) string {
	return fmt.Sprintf(`_crumb_hook() {
  local previous_exit_status=$?;
  if [ -f .crumb.yaml ]; then
    eval "$("%s" export --shell bash 2>/dev/null)";
  fi
  return $previous_exit_status;
};
if ! [[ ";${PROMPT_COMMAND[*]:-};" =~ ";_crumb_hook;" ]]; then
  if [[ "$(declare -p PROMPT_COMMAND 2>&1)" == "declare -a"* ]]; then
    PROMPT_COMMAND=(_crumb_hook "${PROMPT_COMMAND[@]}")
  else
    PROMPT_COMMAND="_crumb_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
  fi
fi
`, selfPath)
}

func zshHook(selfPath string) string {
	return fmt.Sprintf(`_crumb_hook() {
  if [ -f .crumb.yaml ]; then
    eval "$("%s" export --shell bash 2>/dev/null)"
  fi
}
typeset -ag precmd_functions
if (( ! ${precmd_functions[(I)_crumb_hook]} )); then
  precmd_functions=(_crumb_hook $precmd_functions)
fi
typeset -ag chpwd_functions
if (( ! ${chpwd_functions[(I)_crumb_hook]} )); then
  chpwd_functions=(_crumb_hook $chpwd_functions)
fi
`, selfPath)
}

func fishHook(selfPath string) string {
	return fmt.Sprintf(`function _crumb_hook --on-variable PWD --description 'crumb hook'
  if test -f .crumb.yaml
    eval (%s export --shell fish 2>/dev/null)
  end
end

function _crumb_hook_prompt --on-event fish_prompt --description 'crumb hook on prompt'
  if test -f .crumb.yaml
    eval (%s export --shell fish 2>/dev/null)
  end
end
`, selfPath, selfPath)
}
