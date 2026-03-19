package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"
)

var mineCmd = cli.Command{
	Name:  "mine",
	Usage: "auto-mine blocks at a fixed interval (simulates mainnet cadence)",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:    "interval",
			Aliases: []string{"i"},
			Usage:   "seconds between blocks (zcash mainnet = 75)",
			Value:   75,
		},
		&cli.IntFlag{
			Name:    "blocks",
			Aliases: []string{"n"},
			Usage:   "blocks per round (default 1)",
			Value:   1,
		},
		&cli.IntFlag{
			Name:  "limit",
			Usage: "stop after this many rounds (0 = unlimited)",
			Value: 0,
		},
	},
	Action: mineAction,
}

func mineAction(ctx *cli.Context) error {
	if isRunning, _ := nigiriState.GetBool("running"); !isRunning {
		return errors.New("zushi is not running")
	}

	interval := ctx.Int("interval")
	blocks := ctx.Int("blocks")
	limit := ctx.Int("limit")

	if interval < 1 {
		interval = 1
	}

	fmt.Printf("Auto-mining %d block(s) every %ds", blocks, interval)
	if limit > 0 {
		fmt.Printf(" (limit: %d rounds)", limit)
	}
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	// Handle graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	round := 0
	numBlocks := fmt.Sprintf("%d", blocks)

	// Mine first block immediately
	mineBlock(numBlocks, round)
	round++

	for {
		select {
		case <-sig:
			fmt.Println("\nStopping auto-mine.")
			return nil
		case <-ticker.C:
			if limit > 0 && round >= limit {
				fmt.Printf("Reached limit of %d rounds.\n", limit)
				return nil
			}
			mineBlock(numBlocks, round)
			round++
		}
	}
}

func mineBlock(numBlocks string, round int) {
	cmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"generate", numBlocks)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("[round %d] error: %s\n", round, strings.TrimSpace(string(out)))
		return
	}

	// Get current height
	heightCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"getblockcount")
	heightOut, _ := heightCmd.CombinedOutput()
	height := strings.TrimSpace(string(heightOut))

	fmt.Printf("[%s] round %d  height: %s  mined: %s block(s)\n",
		time.Now().Format("15:04:05"), round, height, numBlocks)
}
