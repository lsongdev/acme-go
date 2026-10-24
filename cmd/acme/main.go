// Command acme runs the ACME certificate manager: a JSON API, a web UI and
// an HTTP-01 challenge responder backed by a SQLite database.
package main

import (
	"context"
	"crypto/subtle"
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

	"github.com/lsongdev/acme-go/api"
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
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	service, err := api.NewService(ctx, db, api.Options{RenewBefore: *renewBefore, Logger: logger})
	if err != nil {
		return err
	}
	defer service.Close()

	app := http.NewServeMux()
	app.Handle("/api/", http.StripPrefix("/api", service.Handler()))
	app.Handle("/", web.Handler())
	var handler http.Handler = http.NewCrossOriginProtection().Handler(app)
	if password := os.Getenv("ACME_PASSWORD"); password != "" {
		handler = basicAuth(handler, "admin", password)
	} else if !isLoopback(*listen) {
		logger.Warn("serving the UI without authentication on a non-loopback address; set ACME_PASSWORD")
	}
	challenges := service.HTTP01Handler()
	root := http.NewServeMux()
	root.Handle("/.well-known/acme-challenge/", challenges)
	root.Handle("/", handler)

	servers := []*http.Server{{Addr: *listen, Handler: root, ReadHeaderTimeout: 10 * time.Second}}
	if *http01 != "" {
		challengeOnly := http.NewServeMux()
		challengeOnly.Handle("/.well-known/acme-challenge/", challenges)
		servers = append(servers, &http.Server{Addr: *http01, Handler: challengeOnly, ReadHeaderTimeout: 10 * time.Second})
	}
	errs := make(chan error, len(servers))
	for _, server := range servers {
		go func() { errs <- server.ListenAndServe() }()
	}
	logger.Info("acme listening", "ui", "http://"+*listen, "http01", *http01, "db", *dbPath)

	if *renewInterval > 0 {
		go service.RunAutoRenew(ctx, *renewInterval)
	}

	select {
	case err = <-errs:
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, server := range servers {
		_ = server.Shutdown(shutdown)
	}
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

func basicAuth(next http.Handler, user, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(user)) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="acme", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func defaultDatabasePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "acme.db"
	}
	return filepath.Join(dir, "acme", "acme.db")
}
