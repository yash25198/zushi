package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var versionCmd = cli.Command{
	Name:   "version",
	Usage:  "show version information",
	Action: versionAction,
}

func versionAction(ctx *cli.Context) error {
	fmt.Println("zushi CLI")
	fmt.Println(formatVersion())
	return nil
}
