package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/urfave/cli/v2"
)

var rpcCmd = cli.Command{
	Name:      "rpc",
	Usage:     "invoke zcash-cli inside the zcashd container",
	ArgsUsage: "<command> [args...]",
	Action:    rpcAction,
}

func rpcAction(ctx *cli.Context) error {
	if isRunning, _ := nigiriState.GetBool("running"); !isRunning {
		return errors.New("zushi is not running")
	}

	rpcArgs := []string{"exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass"}
	cmdArgs := append(rpcArgs, ctx.Args().Slice()...)
	bashCmd := exec.Command("docker", cmdArgs...)

	r, w := io.Pipe()
	bashCmd.Stdout = w
	bashCmd.Stderr = os.Stderr

	go func() {
		if err := bashCmd.Run(); err != nil {
			w.CloseWithError(err)
		} else {
			w.Close()
		}
	}()

	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	output := buf.Bytes()

	// Try to pretty-print JSON output
	var v interface{}
	if err := json.Unmarshal(output, &v); err == nil {
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return err
		}

		var prettyBuf bytes.Buffer
		if err := json.Indent(&prettyBuf, jsonBytes, "", "    "); err != nil {
			return err
		}

		lines := strings.Split(prettyBuf.String(), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[") {
				fmt.Println(line)
			} else if strings.Contains(line, ":") {
				parts := strings.SplitN(line, ":", 2)
				key := parts[0]
				value := parts[1]
				fmt.Printf("%s: %s\n",
					aurora.BrightBlue(key).String(),
					aurora.BrightCyan(value).String(),
				)
			} else {
				fmt.Println(line)
			}
		}
	} else {
		fmt.Print(string(output))
	}

	return nil
}
