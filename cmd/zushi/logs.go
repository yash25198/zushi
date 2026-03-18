package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
	"github.com/ysh/zushi/internal/config"
)

var logsCmd = cli.Command{
	Name:      "logs",
	Usage:     "check service logs",
	ArgsUsage: "<service>",
	Action:    logsAction,
}

func logsAction(ctx *cli.Context) error {
	if isRunning, _ := nigiriState.GetBool("running"); !isRunning {
		return errors.New("zushi is not running")
	}

	if ctx.NArg() != 1 {
		return errors.New("usage: zushi logs <service>\n  services: zcashd, lightwalletd, zcash-faucet")
	}

	serviceName := ctx.Args().First()
	datadir := ctx.String("datadir")
	composePath := filepath.Join(datadir, config.DefaultCompose)

	bashCmd := runDockerCompose(composePath, "logs", serviceName)
	bashCmd.Stdout = os.Stdout
	bashCmd.Stderr = os.Stderr

	if err := bashCmd.Run(); err != nil {
		return err
	}

	return nil
}
