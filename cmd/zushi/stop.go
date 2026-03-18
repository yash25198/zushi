package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/urfave/cli/v2"
	"github.com/ysh/zushi/internal/config"
)

var stopCmd = cli.Command{
	Name:   "stop",
	Usage:  "stop zushi",
	Action: stopAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "delete",
			Usage: "clean node data directories and remove containers",
			Value: false,
		},
	},
}

func stopAction(ctx *cli.Context) error {
	deleteData := ctx.Bool("delete")
	datadir := ctx.String("datadir")
	composePath := filepath.Join(datadir, config.DefaultCompose)

	bashCmd := runDockerCompose(composePath, "stop")
	if deleteData {
		bashCmd = runDockerCompose(composePath, "down", "--volumes", "--remove-orphans")
	}

	bashCmd.Stdout = os.Stdout
	bashCmd.Stderr = os.Stderr

	if err := bashCmd.Run(); err != nil {
		// If down fails, force-remove known containers
		if deleteData {
			for _, name := range []string{"zcashd", "zushi-explorer", "lightwalletd"} {
				rm := exec.Command("docker", "rm", "-f", name)
				rm.Run()
			}
		} else {
			return err
		}
	}

	// Always force-remove named containers on --delete to avoid conflicts on next start
	if deleteData {
		for _, name := range []string{"zcashd", "zushi-explorer", "lightwalletd"} {
			rm := exec.Command("docker", "rm", "-f", name)
			rm.Run()
		}
	}

	if deleteData {
		fmt.Println("Removing data from volumes...")

		if err := os.RemoveAll(datadir); err != nil {
			fmt.Printf("Warning: could not remove data directory: %v\n", err)
			sudoCmd := exec.Command("sudo", "rm", "-rf", datadir)
			if err := sudoCmd.Run(); err != nil {
				return fmt.Errorf("failed to remove data directory even with sudo: %w", err)
			}
		}

		if err := provisionResourcesToDatadir(datadir); err != nil {
			return err
		}

		fmt.Println("zushi has been cleaned up successfully.")
	} else {
		if err := nigiriState.Set(map[string]string{
			"running": strconv.FormatBool(false),
		}); err != nil {
			return err
		}
	}

	return nil
}
