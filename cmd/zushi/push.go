package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/urfave/cli/v2"
)

var pushCmd = cli.Command{
	Name:      "push",
	Usage:     "broadcast a raw transaction and mine a block",
	ArgsUsage: "<hex>",
	Action:    pushAction,
}

func pushAction(ctx *cli.Context) error {
	if isRunning, _ := nigiriState.GetBool("running"); !isRunning {
		return errors.New("zushi is not running")
	}

	if ctx.NArg() != 1 {
		return errors.New("usage: zushi push <hex>")
	}

	hex := ctx.Args().First()

	sendCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"sendrawtransaction", hex)
	out, err := sendCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sendrawtransaction failed: %s", strings.TrimSpace(string(out)))
	}
	txid := strings.TrimSpace(string(out))

	// Mine a block to confirm
	genCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"generate", "1")
	genCmd.Run()

	fmt.Println("txId: " + txid)
	return nil
}
