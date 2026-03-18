package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/urfave/cli/v2"
	"github.com/ysh/zushi/internal/config"
	"github.com/ysh/zushi/internal/docker"
)

var lightwalletdFlag = cli.BoolFlag{
	Name:  "lightwalletd",
	Usage: "enable lightwalletd (gRPC light wallet server)",
	Value: false,
}

var headlessFlag = cli.BoolFlag{
	Name:  "headless",
	Usage: "start without the block explorer",
	Value: false,
}

var startCmd = cli.Command{
	Name:   "start",
	Usage:  "start zushi",
	Action: startAction,
	Flags: []cli.Flag{
		&lightwalletdFlag,
		&headlessFlag,
	},
}

func startAction(ctx *cli.Context) error {
	if isRunning, _ := nigiriState.GetBool("running"); isRunning {
		return errors.New("zushi is already running, please stop it first")
	}

	datadir := ctx.String("datadir")
	composePath := filepath.Join(datadir, config.DefaultCompose)

	services := []string{"zcashd"}

	if ctx.Bool("lightwalletd") {
		services = append(services, "lightwalletd")
	}

	if !ctx.Bool("headless") {
		services = append(services, "explorer")
	}

	bashCmd := runDockerCompose(composePath, append([]string{"up", "-d"}, services...)...)
	bashCmd.Stdout = os.Stdout
	bashCmd.Stderr = os.Stderr

	if err := bashCmd.Run(); err != nil {
		return err
	}

	fmt.Printf("zushi configuration located at %s\n", nigiriState.FilePath())
	if err := nigiriState.Set(map[string]string{
		"running":      strconv.FormatBool(true),
		"lightwalletd": strconv.FormatBool(ctx.Bool("lightwalletd")),
	}); err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	// Wait for zcashd RPC to be ready (zcash-fetch-params + startup can be slow)
	done := make(chan bool)
	go spinner(done, "waiting for zcashd to become ready (first run downloads ~1.7GB params)...")

	zcashdReady := false
	timeout := time.After(300 * time.Second) // 5 minutes for first-time param download
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			done <- true
			fmt.Println("\nWarning: zcashd may still be starting up (param download can be slow on first run)")
			fmt.Println("Check status with: zushi logs zcashd")
			goto showEndpoints
		case <-ticker.C:
			// Check if zcash-cli can connect to the RPC
			checkCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
				"-rpcuser=zcashrpc", "-rpcpassword=zcashpass", "getblockchaininfo")
			if out, err := checkCmd.CombinedOutput(); err == nil && strings.Contains(string(out), "regtest") {
				done <- true
				fmt.Println("zcashd is ready!")
				zcashdReady = true
				goto showEndpoints
			}
		}
	}

showEndpoints:
	if zcashdReady {
		// Mine initial blocks so the wallet has funds
		fmt.Println("Mining initial blocks for regtest wallet funding...")
		mineCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
			"-rpcuser=zcashrpc", "-rpcpassword=zcashpass", "generate", "101")
		mineOut, err := mineCmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Warning: could not mine initial blocks: %v\n%s\n", err, string(mineOut))
		} else {
			fmt.Println("Mined 101 blocks - wallet is funded!")
		}
	}

	client := docker.NewDefaultClient()
	endpoints, err := client.GetEndpoints(composePath)
	if err != nil {
		return fmt.Errorf("failed to get endpoints: %w", err)
	}

	filteredEndpoints := make(map[string]string)
	for name, endpoint := range endpoints {
		if !ctx.Bool("lightwalletd") && strings.Contains(name, "lightwalletd") {
			continue
		}
		filteredEndpoints[name] = endpoint
	}

	fmt.Println("\nENDPOINTS")
	for name, endpoint := range filteredEndpoints {
		fmt.Printf("%s %s: %s\n",
			aurora.Green("->"),
			aurora.Blue(name),
			endpoint,
		)
	}

	return nil
}

func spinner(done chan bool, message string) {
	frames := []string{"|", "/", "-", "\\"}
	i := 0
	for {
		select {
		case <-done:
			fmt.Printf("\r%s\r", strings.Repeat(" ", len(message)+3))
			return
		default:
			fmt.Printf("\r%s %s", frames[i], message)
			time.Sleep(150 * time.Millisecond)
			i = (i + 1) % len(frames)
		}
	}
}
