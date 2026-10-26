package model

import (
	"os/exec"
	"time"
)

func configureVibeCancellation(cmd *exec.Cmd) {
	cmd.WaitDelay = 3 * time.Second
}
