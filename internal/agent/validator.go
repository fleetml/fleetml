package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// validatorTimeout bounds a validator or hook command.
const validatorTimeout = 5 * time.Minute

// runCommand runs a shell command with {file} replaced by the quoted file
// path and the model described in the environment. It returns the last
// lines of output on failure.
func runCommand(ctx context.Context, command, file string, env map[string]string) error {
	ctx, cancel := context.WithTimeout(ctx, validatorTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", strings.ReplaceAll(command, "{file}", shellQuote(file)))
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		tail := strings.TrimSpace(string(out))
		if len(tail) > 300 {
			tail = tail[len(tail)-300:]
		}
		if tail != "" {
			return fmt.Errorf("%w: %s", err, tail)
		}
		return err
	}
	return nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
