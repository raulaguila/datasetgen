// Command admin runs operator actions against the database.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/raulaguila/dataset/config"
	"github.com/raulaguila/dataset/internal/composition"
	ports "github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/internal/infra"
	"github.com/raulaguila/dataset/pkg/logger"
)

const usage = "usage: admin [bootstrap] | admin promote | admin disable | admin enable"

// action runs against the database and returns the line to print.
type action func(context.Context, ports.AdminService) (string, error)

func main() {
	run, err := parse(os.Args[1:], os.Stdin, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	cfg := config.MustLoad()
	log := logger.New(logger.Config{Level: "warn", Format: cfg.Log.Format})
	res, err := infra.New(cfg, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer res.Shutdown(log)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := run(ctx, composition.NewAdmin(res))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		res.Shutdown(log)
		os.Exit(1)
	}
	fmt.Println(out)
}

// parse picks the action, bootstrap when no argument is given, and asks its input
// here, before the clock of main starts.
func parse(args []string, in *os.File, out io.Writer) (action, error) {
	if len(args) == 0 || (len(args) == 1 && args[0] == "bootstrap") {
		cmd, err := readBootstrap(in, out)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, svc ports.AdminService) (string, error) {
			created, err := svc.Bootstrap(ctx, cmd)
			if !created {
				return "an admin already exists: nothing done", err
			}
			return "admin created: " + cmd.Email, err
		}, nil
	}
	if len(args) != 1 {
		return nil, errors.New(usage)
	}
	command, ok := map[string]func(ports.AdminService, context.Context, string) error{
		"promote": (ports.AdminService).Promote,
		"disable": (ports.AdminService).Disable,
		"enable":  (ports.AdminService).Enable,
	}[args[0]]
	if !ok {
		return nil, errors.New(usage)
	}
	email, err := ask(bufio.NewReader(in), out, "Email")
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, svc ports.AdminService) (string, error) {
		return args[0] + " ok: " + email, command(svc, ctx, email)
	}, nil
}

func ask(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	_, _ = fmt.Fprint(out, label, ": ")
	value, err := reader.ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || value == "") {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return strings.TrimRight(value, "\r\n"), nil
}

// readBootstrap prompts for the first admin. The password is never an argument,
// which would leak through ps and the shell history; on a terminal it is not
// echoed, and a pipe (CI) gives one value per line.
func readBootstrap(in *os.File, out io.Writer) (ports.BootstrapCommand, error) {
	var cmd ports.BootstrapCommand
	reader := bufio.NewReader(in)
	secret := func(label string) (string, error) {
		if !term.IsTerminal(int(in.Fd())) {
			return ask(reader, out, label)
		}
		_, _ = fmt.Fprint(out, label, ": ")
		value, err := term.ReadPassword(int(in.Fd()))
		_, _ = fmt.Fprintln(out)
		return string(value), err
	}

	var err error
	if cmd.Name, err = ask(reader, out, "Name"); err != nil {
		return cmd, err
	}
	if cmd.Email, err = ask(reader, out, "Email"); err != nil {
		return cmd, err
	}
	if cmd.Password, err = secret("Password"); err != nil {
		return cmd, err
	}
	confirm, err := secret("Confirm password")
	if err != nil {
		return cmd, err
	}
	if confirm != cmd.Password {
		return cmd, errors.New("passwords do not match")
	}
	return cmd, nil
}
