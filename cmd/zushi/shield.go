package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/urfave/cli/v2"
)

var shieldCmd = cli.Command{
	Name:      "shield",
	Usage:     "shield transparent ZEC to a shielded address (z_shieldcoinbase)",
	ArgsUsage: "[zaddr]",
	Action:    shieldAction,
}

func shieldAction(ctx *cli.Context) error {
	if isRunning, _ := nigiriState.GetBool("running"); !isRunning {
		return errors.New("zushi is not running")
	}

	zaddr := ""
	if ctx.NArg() >= 1 {
		zaddr = ctx.Args().First()
	} else {
		// Generate a new Sapling address
		addrCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
			"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
			"z_getnewaddress", "sapling")
		out, err := addrCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to get new shielded address: %s", strings.TrimSpace(string(out)))
		}
		zaddr = strings.TrimSpace(string(out))
		fmt.Println("Generated shielded address: " + zaddr)
	}

	// Shield coinbase to the z-address
	shieldExecCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"z_shieldcoinbase", "*", zaddr)
	shieldExecCmd.Stdout = os.Stdout
	shieldExecCmd.Stderr = os.Stderr

	if err := shieldExecCmd.Run(); err != nil {
		return fmt.Errorf("z_shieldcoinbase failed: %w", err)
	}

	// Mine a block to confirm
	genCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"generate", "1")
	genCmd.Run()

	fmt.Println("Shielding operation submitted and block mined.")
	return nil
}
