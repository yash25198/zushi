package main

import (
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"github.com/ysh/zushi/internal/config"
)

var updateCmd = cli.Command{
	Name:   "update",
	Usage:  "pull latest docker images",
	Action: updateAction,
}

func updateAction(ctx *cli.Context) error {
	datadir := ctx.String("datadir")
	composePath := filepath.Join(datadir, config.DefaultCompose)

	bashCmd := runDockerCompose(composePath, "pull")
	bashCmd.Stdout = os.Stdout
	bashCmd.Stderr = os.Stderr

	if err := bashCmd.Run(); err != nil {
		return err
	}

	return nil
}
