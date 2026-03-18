package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/urfave/cli/v2"
)

var faucetCmd = cli.Command{
	Name:      "faucet",
	Usage:     "generate and send ZEC to a given address",
	ArgsUsage: "<address> [amount]",
	Action:    faucetAction,
}

func faucetAction(ctx *cli.Context) error {
	if isRunning, _ := nigiriState.GetBool("running"); !isRunning {
		return errors.New("zushi is not running")
	}

	if ctx.NArg() < 1 || ctx.NArg() > 2 {
		return errors.New("usage: zushi faucet <address> [amount]")
	}

	address := ctx.Args().First()
	amount := "1.0"
	if ctx.Args().Len() >= 2 {
		amount = ctx.Args().Get(1)
	}

	// sendtoaddress via zcash-cli
	sendCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"sendtoaddress", address, amount)
	out, err := sendCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sendtoaddress failed: %s", strings.TrimSpace(string(out)))
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
