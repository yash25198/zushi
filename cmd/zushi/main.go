package main

import (
	"embed"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/ysh/zushi/internal/config"
	"github.com/ysh/zushi/internal/state"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"

	nigiriState = state.New(config.DefaultPath, config.InitialState)
)

var datadirFlag = cli.StringFlag{
	Name:  "datadir",
	Usage: "use different data directory",
	Value: config.DefaultDatadir,
}

//go:embed resources/docker-compose.yml
//go:embed resources/zcash.conf
//go:embed resources/explorer/main.go
//go:embed resources/explorer/index.html
//go:embed resources/explorer/Dockerfile
var f embed.FS

func main() {
	app := cli.NewApp()

	app.Version = formatVersion()
	app.Name = "zushi"
	app.Usage = "one-click Zcash regtest development environment"
	app.Flags = append(app.Flags, &datadirFlag)
	app.Commands = []*cli.Command{
		&startCmd,
		&stopCmd,
		&logsCmd,
		&updateCmd,
		&versionCmd,

		&rpcCmd,
		&faucetCmd,
		&pushCmd,
		&generateCmd,
		&shieldCmd,
		&mineCmd,
	}

	app.Before = func(ctx *cli.Context) error {
		dataDir := config.DefaultDatadir

		if ctx.IsSet("datadir") {
			dataDir = cleanAndExpandPath(ctx.String("datadir"))
			nigiriState = state.New(filepath.Join(dataDir, config.DefaultName), config.InitialState)
		}

		if err := provisionResourcesToDatadir(dataDir); err != nil {
			return err
		}

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "[zushi] %v\n", err)
	os.Exit(1)
}

func provisionResourcesToDatadir(datadir string) error {
	isReady, err := nigiriState.GetBool("ready")
	if err != nil {
		return err
	}

	if isReady {
		return nil
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	uid, err := strconv.Atoi(currentUser.Uid)
	if err != nil {
		return fmt.Errorf("parse uid: %w", err)
	}

	gid, err := strconv.Atoi(currentUser.Gid)
	if err != nil {
		return fmt.Errorf("parse gid: %w", err)
	}

	// Create data directory
	if err := makeDirectoryIfNotExists(datadir, uid, gid); err != nil {
		return err
	}

	// Copy docker-compose.yml
	if err := copyFromResourcesToDatadir(
		filepath.Join("resources", config.DefaultCompose),
		filepath.Join(datadir, config.DefaultCompose),
		uid, gid,
	); err != nil {
		return err
	}

	// Copy zcash.conf (bind-mounted into the container)
	if err := copyFromResourcesToDatadir(
		filepath.Join("resources", "zcash.conf"),
		filepath.Join(datadir, "zcash.conf"),
		uid, gid,
	); err != nil {
		return err
	}

	// Copy explorer files
	explorerDir := filepath.Join(datadir, "explorer")
	if err := makeDirectoryIfNotExists(explorerDir, uid, gid); err != nil {
		return err
	}
	for _, name := range []string{"main.go", "index.html", "Dockerfile"} {
		if err := copyFromResourcesToDatadir(
			filepath.Join("resources", "explorer", name),
			filepath.Join(explorerDir, name),
			uid, gid,
		); err != nil {
			return err
		}
	}

	if err := nigiriState.Set(map[string]string{"ready": strconv.FormatBool(true)}); err != nil {
		return err
	}

	return nil
}

func formatVersion() string {
	return fmt.Sprintf(
		"\nVersion: %s\nCommit: %s\nDate: %s",
		version, commit, date,
	)
}

func copyFromResourcesToDatadir(src string, dest string, uid, gid int) error {
	data, err := f.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read embed: %w", err)
	}

	err = ioutil.WriteFile(dest, data, 0660)
	if err != nil {
		return fmt.Errorf("write %s to %s: %w", src, dest, err)
	}

	if err := os.Chown(dest, uid, gid); err != nil {
		return fmt.Errorf("chown %s: %w", dest, err)
	}

	return nil
}

func makeDirectoryIfNotExists(path string, uid, gid int) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0770); err != nil {
			return err
		}
		if err := os.Chown(path, uid, gid); err != nil {
			return fmt.Errorf("chown %s: %w", path, err)
		}
	}
	return nil
}

func cleanAndExpandPath(path string) string {
	if path == "" {
		return ""
	}

	if strings.HasPrefix(path, "~") {
		var homeDir string
		u, err := user.Current()
		if err == nil {
			homeDir = u.HomeDir
		} else {
			homeDir = os.Getenv("HOME")
		}
		path = strings.Replace(path, "~", homeDir, 1)
	}

	return filepath.Clean(os.ExpandEnv(path))
}

// runDockerCompose runs docker-compose with the given arguments.
func runDockerCompose(composePath string, args ...string) *exec.Cmd {
	var cmd *exec.Cmd

	_, err := exec.LookPath("docker-compose")
	if err == nil {
		cmd = exec.Command("docker-compose", append([]string{"-f", composePath}, args...)...)
	} else {
		cmd = exec.Command("docker", append([]string{"compose", "-f", composePath}, args...)...)
	}

	if err := setupCmdWithUser(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to setup user environment: %v\n", err)
	}

	return cmd
}

func setupCmdWithUser(cmd *exec.Cmd) error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	cmd.Env = append(os.Environ(),
		"UID="+currentUser.Uid,
		"GID="+currentUser.Gid,
	)

	return nil
}
