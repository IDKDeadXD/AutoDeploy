//go:build !unix

package deployer

import "os/exec"

func prepareProcess(cmd *exec.Cmd) {}
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
