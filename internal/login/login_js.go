//go:build js

package login

import (
	"context"
	"os/exec"
)

func RunOpenCmd(ctx context.Context, input string) error {
	return exec.CommandContext(ctx, "xdg-open", input).Run()
}
