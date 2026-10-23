// Command socrates-engine is the engine of Socrates: it is started by the shell (the desktop window or the phone app), prints one line
// of JSON on standard output (the port it listens on, the per-run token and its version), serves the shell on the loopback address only,
// and stops when the shell closes its standard input or asks it to stop.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"socrates/internal/api"
	"socrates/internal/keystore"
	"socrates/internal/privacy"
	"socrates/internal/secrets"
	"socrates/internal/store"
	"socrates/internal/transport"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "0.0.0-dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "socrates-engine:", err)
		os.Exit(1)
	}
}

func dataDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".socrates")
}

func run() error {
	dir := flag.String("data-dir", dataDir(), "the folder of the database and the key settings")
	port := flag.Int("port", 0, "the loopback port (0: a free one chosen by the system)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return nil
	}

	// The log goes to standard error through the scrubber; standard output carries the handshake only.
	log.SetOutput(privacy.Default)
	log.SetFlags(log.LstdFlags | log.LUTC)
	enableContentLogging() // a build for developers only; a release build never shows content
	if err := secrets.HardenProcess(); err != nil {
		log.Printf("[engine] could not make the process non-dumpable: %v", err)
	}
	if err := os.MkdirAll(*dir, 0o700); err != nil {
		return fmt.Errorf("cannot prepare the data folder: %w", err)
	}

	var origins []string
	if v := os.Getenv("SOCRATES_ALLOWED_ORIGINS"); v != "" {
		origins = strings.Split(v, ",")
	}
	policy, err := transport.NewPolicy(origins)
	if err != nil {
		return err
	}
	lis, err := transport.Listen(*port)
	if err != nil {
		return err
	}
	policy.SetPort(transport.PortOf(lis))

	quit := make(chan struct{}, 1)
	requestQuit := func() {
		select {
		case quit <- struct{}{}:
		default:
		}
	}
	var start transport.StartupSwitch
	srv := transport.NewHTTPServer(policy.Middleware(&start))
	go srv.Serve(lis)
	if err := transport.WriteHandshake(os.Stdout, transport.Handshake{Port: transport.PortOf(lis), Token: policy.Token(), Version: version}); err != nil {
		return err
	}
	transport.WatchParent(os.Stdin, requestQuit)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		requestQuit()
	}()
	defer shutdown(srv)

	// Until the database key is available only the key gate answers.
	dbPath := filepath.Join(*dir, "socrates.db")
	keys := keystore.Open(*dir, dbPath, keystore.SystemKeyring{})
	if keys.Status().State != keystore.StateReady {
		log.Printf("[engine] waiting for the database key (%s)", keys.Status().State)
		gate := transport.KeyGate(keys, version)
		transport.RegisterShutdown(gate, quit)
		start.Set(gate)
		select {
		case <-keys.Ready():
		case <-quit:
			return nil
		}
		start.Set(nil) // back to "starting" while the database opens
	}
	key, _ := keys.Key()
	db, err := store.Open(dbPath, key)
	if err != nil {
		return fmt.Errorf("cannot open the database: %w", err)
	}
	defer db.Close()

	mux, conversations := api.New(api.Deps{Store: db, Version: version})
	defer conversations.Wait() // titles and summaries of the last turns are written before the database closes
	transport.RegisterShutdown(mux, quit)
	limiter := transport.NewRateLimiter(transport.DefaultLimits)
	activity := transport.NewActivity()
	start.Set(activity.Wrap(transport.Limit(limiter, nil, mux)))
	log.Printf("[engine] ready on port %d", transport.PortOf(lis))
	<-quit
	return nil
}

func shutdown(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
