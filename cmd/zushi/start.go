package main

import (
	"encoding/json"
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
		// Mine initial blocks so the wallet has funds. 110 (not 101) so that
		// at least 10 coinbase outputs reach the 100-conf maturity threshold
		// — z_shieldcoinbase only selects mature coinbase, and a single
		// mature utxo (~6.25 ZEC) is tight for repeated faucet calls.
		fmt.Println("Mining initial blocks for regtest wallet funding...")
		mineCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
			"-rpcuser=zcashrpc", "-rpcpassword=zcashpass", "generate", "110")
		mineOut, err := mineCmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Warning: could not mine initial blocks: %v\n%s\n", err, string(mineOut))
		} else {
			fmt.Println("Mined 110 blocks - wallet is funded!")
		}

		// Bootstrap a funded shielded UA so `zushi faucet --shielded` can
		// just work. Without this, faucet falls into the
		// findFundedAddress() ANY_TADDR / coinbase-t-addr fallback path,
		// which trips zcashd's "no transparent change" / "ANY_TADDR
		// excludes coinbase" / privacyPolicy rules in various combinations.
		// With a pre-shielded UA in the wallet, faucet finds it via
		// listaddresses and the z_sendmany is a clean shielded → shielded
		// send. Idempotent: skips if any UA already holds funds (so
		// stop/start cycles don't re-shield).
		if err := bootstrapShieldedUA(); err != nil {
			fmt.Printf("Warning: shielded bootstrap failed: %v\n", err)
			fmt.Println("Transparent faucet still works; --shielded may need manual z_shieldcoinbase.")
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

// bootstrapShieldedUA ensures the wallet has at least one funded Unified
// Address with an Orchard receiver, so `zushi faucet --shielded` can use
// it as a shielded source for z_sendmany.
//
// Steps (only run if no UA already holds funds):
//   1. z_getnewaccount                      → account 0 (or next free)
//   2. z_getaddressforaccount <acct> [orchard] → Orchard UA
//   3. z_shieldcoinbase * <UA>              → shield mature coinbase
//      (privacyPolicy=AllowLinkingAccountAddresses required, otherwise
//      zcashd refuses the implicit linking of multiple miner t-addrs;
//      default AllowRevealedSenders is too strict for "*")
//   4. Poll opid until success/fail
//   5. generate 10 to confirm the shielding tx
func bootstrapShieldedUA() error {
	if walletHasFundedUA() {
		fmt.Println("Shielded UA already funded — skipping bootstrap.")
		return nil
	}

	fmt.Println("Bootstrapping shielded UA for faucet...")

	// (1) account
	if _, err := zcashCli("z_getnewaccount"); err != nil {
		return fmt.Errorf("z_getnewaccount: %w", err)
	}

	// (2) UA. Pin to account 0; on a fresh wallet getnewaccount returns 0.
	// On a wallet where bootstrap previously partially completed, account
	// numbering may differ — but walletHasFundedUA() above would have
	// short-circuited that case if the UA actually has balance.
	uaOut, err := zcashCli("z_getaddressforaccount", "0", `["orchard"]`)
	if err != nil {
		return fmt.Errorf("z_getaddressforaccount: %w", err)
	}
	var uaResp struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(uaOut), &uaResp); err != nil || uaResp.Address == "" {
		return fmt.Errorf("parse UA response: %s", uaOut)
	}
	ua := uaResp.Address
	fmt.Printf("Created shielded UA: %s\n", ua)

	// (3) shield. Args: from, to, fee=null, limit=50, memo='',
	// privacyPolicy=AllowLinkingAccountAddresses
	shieldOut, err := zcashCli("z_shieldcoinbase", "*", ua,
		"null", "50", "", "AllowLinkingAccountAddresses")
	if err != nil {
		return fmt.Errorf("z_shieldcoinbase: %w (%s)", err, shieldOut)
	}
	var shieldResp struct {
		OpID            string  `json:"opid"`
		ShieldingUTXOs  int     `json:"shieldingUTXOs"`
		ShieldingValue  float64 `json:"shieldingValue"`
	}
	if err := json.Unmarshal([]byte(shieldOut), &shieldResp); err != nil {
		return fmt.Errorf("parse shield response: %s", shieldOut)
	}
	fmt.Printf("Shielding %d coinbase utxos (%.2f ZEC), opid=%s\n",
		shieldResp.ShieldingUTXOs, shieldResp.ShieldingValue, shieldResp.OpID)

	// (4) poll
	if err := waitForOpid(shieldResp.OpID, 60*time.Second); err != nil {
		return fmt.Errorf("shield op: %w", err)
	}

	// (5) confirm
	if _, err := zcashCli("generate", "10"); err != nil {
		return fmt.Errorf("generate confirms: %w", err)
	}

	fmt.Println("Shielded UA funded and confirmed. `zushi faucet --shielded` ready.")
	return nil
}

// walletHasFundedUA reports whether any UA in the wallet currently has a
// positive balance. Used to make bootstrap idempotent across stop/start.
func walletHasFundedUA() bool {
	out, err := zcashCli("listaddresses")
	if err != nil {
		return false
	}
	var addrs []struct {
		Unified []struct {
			Addresses []struct {
				Address string `json:"address"`
			} `json:"addresses"`
		} `json:"unified"`
	}
	if err := json.Unmarshal([]byte(out), &addrs); err != nil {
		return false
	}
	for _, a := range addrs {
		for _, u := range a.Unified {
			for _, addr := range u.Addresses {
				if addr.Address == "" {
					continue
				}
				balOut, err := zcashCli("z_getbalance", addr.Address)
				if err != nil {
					continue
				}
				var bal float64
				fmt.Sscanf(strings.TrimSpace(balOut), "%f", &bal)
				if bal > 0 {
					return true
				}
			}
		}
	}
	return false
}

// waitForOpid polls z_getoperationstatus until the operation succeeds,
// fails, or the deadline passes.
func waitForOpid(opid string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(1 * time.Second)
		out, err := zcashCli("z_getoperationstatus", fmt.Sprintf(`["%s"]`, opid))
		if err != nil {
			continue
		}
		var ops []struct {
			Status string `json:"status"`
			Error  struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(out), &ops); err != nil || len(ops) == 0 {
			continue
		}
		switch ops[0].Status {
		case "success":
			return nil
		case "failed":
			return fmt.Errorf("opid %s failed: %s", opid, ops[0].Error.Message)
		}
	}
	return fmt.Errorf("opid %s timed out after %s", opid, timeout)
}

// zcashCli runs `docker exec zcashd zcash-cli -regtest ...args` and
// returns trimmed stdout. Wraps the boilerplate scattered through the
// other commands.
func zcashCli(args ...string) (string, error) {
	full := append([]string{"exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass"}, args...)
	out, err := exec.Command("docker", full...).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), err
	}
	return strings.TrimSpace(string(out)), nil
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
