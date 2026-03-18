package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/urfave/cli/v2"
)

var generateCmd = cli.Command{
	Name:      "generate",
	Usage:     "mine blocks on the regtest chain",
	ArgsUsage: "[numblocks]",
	Action:    generateAction,
}

func generateAction(ctx *cli.Context) error {
	if isRunning, _ := nigiriState.GetBool("running"); !isRunning {
		return errors.New("zushi is not running")
	}

	numBlocks := "1"
	if ctx.NArg() >= 1 {
		numBlocks = ctx.Args().First()
	}

	bashCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"generate", numBlocks)
	bashCmd.Stdout = os.Stdout
	bashCmd.Stderr = os.Stderr

	if err := bashCmd.Run(); err != nil {
		return fmt.Errorf("failed to generate blocks: %w", err)
	}

	return nil
}
