package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
)

var faucetCmd = cli.Command{
	Name:      "faucet",
	Usage:     "generate and send ZEC to a given address",
	ArgsUsage: "[--shielded] <address> [amount]",
	Action:    faucetAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "shielded",
			Aliases: []string{"s"},
			Usage:   "send via z_sendmany (required for unified/sapling addresses)",
			Value:   false,
		},
	},
}

func faucetAction(ctx *cli.Context) error {
	if isRunning, _ := nigiriState.GetBool("running"); !isRunning {
		return errors.New("zushi is not running")
	}

	if ctx.NArg() < 1 || ctx.NArg() > 2 {
		return errors.New("usage: zushi faucet [--shielded] <address> [amount]")
	}

	address := ctx.Args().First()
	amount := "1.0"
	if ctx.Args().Len() >= 2 {
		amount = ctx.Args().Get(1)
	}

	if ctx.Bool("shielded") {
		return faucetShielded(address, amount)
	}
	return faucetTransparent(address, amount)
}

// faucetTransparent sends ZEC to a transparent address via sendtoaddress
func faucetTransparent(address, amount string) error {
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

// faucetShielded sends ZEC to a shielded address via z_sendmany
// Uses the wallet's first available funded address as source.
func faucetShielded(address, amount string) error {
	// Find a funded source address
	fromAddr, err := findFundedAddress()
	if err != nil {
		return fmt.Errorf("no funded address found: %w", err)
	}

	// z_sendmany <from> [{"address":<to>,"amount":<amt>}] 1 null <policy>
	//
	// Policy = NoPrivacy. The regtest faucet spends coinbase from an
	// explicit t-addr → UA recipient, which produces:
	//   - transparent INPUT  (coinbase t-utxo)         → needs AllowRevealedSenders
	//   - shielded OUTPUT   (UA's orchard/sapling rcv) → fine
	//   - transparent CHANGE (back to source t-addr)   → needs AllowRevealedRecipients
	// Zcash privacy policies are NOT a strict subset hierarchy; they are
	// orthogonal flags ("AllowRevealedSenders" alone won't cover transparent
	// change). NoPrivacy permits every flag at once. This is a regtest
	// faucet — no privacy to protect, so just use the kitchen-sink policy.
	recipients := fmt.Sprintf(`[{"address":"%s","amount":%s}]`, address, amount)
	sendCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"z_sendmany", fromAddr, recipients, "1", "null", "NoPrivacy")
	out, err := sendCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("z_sendmany failed: %s", strings.TrimSpace(string(out)))
	}
	opid := strings.TrimSpace(string(out))
	fmt.Printf("operation: %s\n", opid)

	// Poll for completion (max 120s)
	txid := ""
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)

		statusCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
			"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
			"z_getoperationstatus", fmt.Sprintf(`["%s"]`, opid))
		statusOut, err := statusCmd.CombinedOutput()
		if err != nil {
			continue
		}

		var ops []struct {
			Status string `json:"status"`
			Result struct {
				Txid string `json:"txid"`
			} `json:"result"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(statusOut, &ops); err != nil || len(ops) == 0 {
			continue
		}

		switch ops[0].Status {
		case "success":
			txid = ops[0].Result.Txid
			goto done
		case "failed":
			return fmt.Errorf("z_sendmany failed: %s", ops[0].Error.Message)
		default:
			fmt.Printf("\rwaiting... (%s)", ops[0].Status)
		}
	}
	return fmt.Errorf("z_sendmany timed out after 120s (opid: %s)", opid)

done:
	fmt.Printf("\r")

	// Mine a block to confirm
	genCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"generate", "1")
	genCmd.Run()

	fmt.Println("txId: " + txid)
	return nil
}

// findFundedAddress returns a wallet-owned address that holds spendable
// funds usable by z_sendmany.
//
// Order matters: shielded > explicit t-addr > ANY_TADDR.
//
//   1. listaddresses → unified/sapling with positive z_getbalance.
//      Cleanest source for z_sendmany (no policy bumps needed).
//   2. listunspent (minconf=100) → first MATURE coinbase t-utxo's address.
//      Required because z_sendmany REFUSES to select coinbase when source
//      is "ANY_TADDR" — that's a hard disqualifier independent of
//      privacyPolicy. Passing the explicit t-addr that owns the coinbase
//      utxo bypasses the rule. Coinbase needs 100 confirmations to mature
//      in regtest, so minconf=100 (lower → "Insufficient funds: have 0.00").
//   3. ANY_TADDR fallback — only useful if wallet has non-coinbase t-funds
//      (e.g. someone sent us regular t-zec). Won't help on a fresh nigiri
//      where the only funds are mined coinbase.
func findFundedAddress() (string, error) {
	// (1) shielded
	listCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"listaddresses")
	listOut, err := listCmd.CombinedOutput()
	if err == nil {
		var addrs []struct {
			Unified []struct {
				Addresses []struct {
					Address string `json:"address"`
				} `json:"addresses"`
			} `json:"unified"`
			Sapling []struct {
				Addresses []string `json:"addresses"`
			} `json:"sapling"`
			Transparent struct {
				Addresses []string `json:"addresses"`
			} `json:"transparent"`
		}
		if err := json.Unmarshal(listOut, &addrs); err == nil {
			// listaddresses returns one entry per source (mnemonic_seed,
			// legacy_random, imported, …). Scan ALL sources.
			for _, a := range addrs {
				for _, u := range a.Unified {
					for _, addr := range u.Addresses {
						if addr.Address != "" && getBalance(addr.Address) > 0 {
							return addr.Address, nil
						}
					}
				}
				for _, s := range a.Sapling {
					for _, addr := range s.Addresses {
						if getBalance(addr) > 0 {
							return addr, nil
						}
					}
				}
			}
		}
	}

	// (2) explicit mature coinbase t-addr via listunspent
	unspentCmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"listunspent", "100")
	unspentOut, err := unspentCmd.CombinedOutput()
	if err == nil {
		var utxos []struct {
			Address       string  `json:"address"`
			Amount        float64 `json:"amount"`
			Confirmations int     `json:"confirmations"`
		}
		if err := json.Unmarshal(unspentOut, &utxos); err == nil {
			for _, u := range utxos {
				if u.Address != "" && u.Amount > 0 {
					return u.Address, nil
				}
			}
		}
	}

	// (3) ANY_TADDR fallback (literal — NOT "*", which zcashd rejects with
	// "Invalid from address: should be a taddr, zaddr, UA, or the string
	// 'ANY_TADDR'"). Only sees non-coinbase t-funds.
	return "ANY_TADDR", nil
}

func getBalance(addr string) float64 {
	cmd := exec.Command("docker", "exec", "zcashd", "zcash-cli", "-regtest",
		"-rpcuser=zcashrpc", "-rpcpassword=zcashpass",
		"z_getbalance", addr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(out))
	var bal float64
	fmt.Sscanf(s, "%f", &bal)
	return bal
}
