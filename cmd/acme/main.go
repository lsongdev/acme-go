// Command acme runs the ACME certificate manager: a JSON API, a web UI and
// an HTTP-01 challenge responder backed by a SQLite database.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lsongdev/acme-go/store"
	"github.com/lsongdev/acme-go/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "acme:", err)
		os.Exit(1)
	}
}

const usage = `usage: acme serve [flags]

Serves the certificate manager UI and API. Set ACME_PASSWORD to require
HTTP basic authentication (user "admin") for the UI and API.

Flags:
`

func run(args []string) error {
	if len(args) == 0 || args[0] != "serve" {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("unknown command")
	}
	flags := flag.NewFlagSet("acme serve", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(flags.Output(), usage); flags.PrintDefaults() }
	listen := flags.String("listen", "127.0.0.1:8080", "address for the UI and API")
	http01 := flags.String("http01", "", "address for a separate HTTP-01 challenge listener, e.g. :80 (challenges are always served on -listen too)")
	dbPath := flags.String("db", defaultDatabasePath(), "SQLite database path")
	renewInterval := flags.Duration("renew-interval", 12*time.Hour, "how often to check for certificates due for renewal (0 disables)")
	renewBefore := flags.Duration("renew-before", 30*24*time.Hour, "renew certificates this long before they expire")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		return err
	}
	certs, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer certs.Close()
	manager, err := web.NewWithStore(ctx, certs, web.Options{RenewBefore: *renewBefore, Logger: logger})
	if err != nil {
		return err
	}
	defer manager.Close()

	password := os.Getenv("ACME_PASSWORD")
	if password == "" && !isLoopback(*listen) {
		logger.Warn("serving the UI without authentication on a non-loopback address; set ACME_PASSWORD")
	}
	// Challenges are public and answered ahead of the UI.
	challenges := manager.Service.HTTP01Handler()
	root := http.NewServeMux()
	root.Handle("/.well-known/acme-challenge/", challenges)
	root.Handle("/", manager.Standalone(password))

	if *renewInterval > 0 {
		go manager.Service.RunAutoRenew(ctx, *renewInterval)
	}
	logger.Info("acme listening", "ui", "http://"+*listen, "http01", *http01, "db", *dbPath)
	errs := make(chan error, 2)
	go func() { errs <- listenAndServe(ctx, *listen, root) }()
	if *http01 != "" {
		challengeOnly := http.NewServeMux()
		challengeOnly.Handle("/.well-known/acme-challenge/", challenges)
		go func() { errs <- listenAndServe(ctx, *http01, challengeOnly) }()
	}
	// The first listener to stop (an error, or shutdown) ends the command.
	return <-errs
}

// listenAndServe serves until ctx is cancelled, then shuts down gracefully.
func listenAndServe(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

func isLoopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}

func defaultDatabasePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "acme.db"
	}
	return filepath.Join(dir, "acme", "acme.db")
}
