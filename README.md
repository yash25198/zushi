# zushi

A one-click Zcash regtest development environment. Inspired by [nigiri](https://github.com/vulpemventures/nigiri).

`zushi` spins up a full `zcashd` regtest node inside Docker, mines initial blocks so the wallet is funded, and gives you a simple CLI to interact with it. No configuration needed.

---

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (running)
- [Go 1.21+](https://go.dev/dl/) (to build from source)

## Install

```bash
git clone https://github.com/ysh/zushi.git
cd zushi
make build
```

The binary is at `./build/zushi-<os>-<arch>`. To install globally:

```bash
cp ./build/zushi-* /usr/local/bin/zushi
```

## Quickstart

```bash
# Start the environment (first run pulls images + zcash params, ~2 min)
zushi start

# Open the block explorer
open http://localhost:5001

# Check the chain
zushi rpc getblockchaininfo

# Get a fresh address
zushi rpc getnewaddress

# Send 10 ZEC to it
zushi faucet <address> 10

# Mine 5 more blocks
zushi generate 5

# Shield coinbase funds to a new shielded address
zushi shield

# Check balances (transparent + private)
zushi rpc z_gettotalbalance

# Done for the day
zushi stop

# Nuke everything and start fresh
zushi stop --delete
```

## Commands

| Command | Description |
|---|---|
| `zushi start` | Start zcashd in regtest + block explorer, mine 101 blocks, fund the wallet |
| `zushi start --headless` | Start without the block explorer |
| `zushi start --lightwalletd` | Also start [lightwalletd](https://github.com/zcash/lightwalletd) (gRPC on `:9067`) |
| `zushi stop` | Stop containers (data persists) |
| `zushi stop --delete` | Stop and wipe all data + volumes |
| `zushi rpc <cmd> [args]` | Run any `zcash-cli` command |
| `zushi faucet <addr> [amt]` | Send ZEC to an address and auto-mine a block |
| `zushi generate [n]` | Mine `n` blocks (default: 1) |
| `zushi push <hex>` | Broadcast a raw transaction and mine a block |
| `zushi shield [zaddr]` | Shield coinbase ZEC to a shielded address via `z_shieldcoinbase` |
| `zushi logs <service>` | Tail container logs (`zcashd`, `lightwalletd`, `explorer`) |
| `zushi update` | Pull latest Docker images |
| `zushi version` | Print version info |

## Services

| Service | Image | Ports | Purpose |
|---|---|---|---|
| `zcashd` | `electriccoinco/zcashd` | `18232` (RPC), `18233` (P2P) | Full node in regtest mode |
| `explorer` | built from `explorer/` | `5001` (HTTP) | Block explorer web UI |
| `lightwalletd` | `electriccoinco/lightwalletd` | `9067` (gRPC) | Light wallet server (opt-in) |

## Endpoints

After `zushi start`:

```
zcashd RPC     http://localhost:18232  (user: zcashrpc, pass: zcashpass)
explorer       http://localhost:5001   (block explorer)
lightwalletd   localhost:9067          (gRPC, --lightwalletd flag)
```

## Configuration

zushi stores its state and docker-compose files in an OS-appropriate data directory:

| OS | Path |
|---|---|
| macOS | `~/Library/Application Support/zushi/` |
| Linux | `~/.zushi/` |
| Windows | `%LOCALAPPDATA%\zushi\` |

Override with `--datadir`:

```bash
zushi --datadir /tmp/my-zcash start
```

### Regtest node config

The bundled `zcash.conf` activates all network upgrades (Overwinter through NU5) from block 1, enables `txindex`, and allows deprecated RPCs (`getnewaddress`, `z_getbalance`, etc.) for development convenience.

## Architecture

```
zushi (Go CLI)
  |
  |-- docker compose up -d zcashd
  |       |
  |       +-- electriccoinco/zcashd:latest (regtest)
  |              ports: 18232, 18233
  |
  |-- docker compose up -d explorer
  |       |
  |       +-- zushi-explorer (Go + embedded HTML)
  |              port: 5001  -->  zcashd RPC
  |
  |-- docker compose up -d lightwalletd  (optional)
  |       |
  |       +-- electriccoinco/lightwalletd:latest
  |              port: 9067 (gRPC)
  |
  +-- all commands run via:
        docker exec zcashd zcash-cli -regtest ...
```

The CLI embeds the `docker-compose.yml`, `zcash.conf`, and the explorer source as Go embedded resources. On first run, these are copied to the data directory and used to orchestrate containers. The explorer image is built locally from its Dockerfile on first `zushi start`.

## Development

```bash
# Install deps
make install

# Build
make build

# Run tests
make test

# Vet
make vet

# Format check
make fmt
```

### Release

Uses [goreleaser](https://goreleaser.com/) for cross-platform builds:

```bash
goreleaser --snapshot --skip-publish --rm-dist
```

## Block Explorer

`zushi start` includes a built-in block explorer at **http://localhost:5001**.

Features:
- **Dashboard** -- block height, chain supply, difficulty (auto-refreshes)
- **Block list** -- latest 20 blocks with tx counts
- **Block detail** -- all transactions with tagged badges:
  - `coinbase` (yellow) for miner rewards
  - `shielded in:N out:N` (purple) for Sapling/Orchard transactions
- **Transaction detail** -- transparent inputs/outputs with full addresses, shielded spends/outputs with nullifiers and commitments, block number
- **Address lookup** -- balance for both transparent and shielded addresses
- **Search** -- by block height, block hash, txid, or address

## How it compares to nigiri

[nigiri](https://github.com/vulpemventures/nigiri) is a Bitcoin/Liquid regtest box. zushi is the same idea for Zcash, with additions specific to the Zcash protocol:

- **Built-in block explorer** with shielded transaction support (Sapling spends/outputs, Orchard actions)
- **`shield`** command for `z_shieldcoinbase` (move transparent funds to shielded pool)
- **`generate`** command for quick block mining
- All Zcash network upgrades (Overwinter through NU5) active from block 1
- Opt-in `lightwalletd` for testing light wallet integrations

## License

MIT
