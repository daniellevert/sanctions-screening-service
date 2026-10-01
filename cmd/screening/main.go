package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/daniellevert/sanctions-screening-service/internal/screening"
)

func run() error {
	accountsPath := flag.String("accounts", "data/accounts.csv", "account CSV path")
	localSDNPath := flag.String("sdn", "", "explicit local SDN XML path (skips download)")
	snapshotPath := flag.String("snapshot", "data/sdn.xml", "destination for a downloaded SDN snapshot")
	listenAddress := flag.String("listen", "127.0.0.1:8080", "HTTP listen address")
	cacheSize := flag.Int("cache-size", 16000, "maximum cached name searches; 0 disables")
	weakAliases := flag.String("weak-aliases", "include", "weak alias policy: include or exclude")
	flag.Parse()

	var includeWeak bool
	switch *weakAliases {
	case "include":
		includeWeak = true
	case "exclude":
		includeWeak = false
	default:
		return fmt.Errorf("-weak-aliases must be include or exclude")
	}

	accountsFile, err := os.Open(*accountsPath)
	if err != nil {
		return fmt.Errorf("open accounts: %w", err)
	}
	accounts, err := screening.LoadAccounts(accountsFile)
	closeErr := accountsFile.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return fmt.Errorf("close accounts: %w", closeErr)
	}

	var snapshot screening.Snapshot
	if *localSDNPath != "" {
		file, err := os.Open(*localSDNPath)
		if err != nil {
			return fmt.Errorf("open local SDN: %w", err)
		}
		snapshot, err = screening.LoadSDN(file)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return fmt.Errorf("close local SDN: %w", closeErr)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(*snapshotPath), 0755); err != nil {
			return fmt.Errorf("create snapshot directory: %w", err)
		}
		client := &http.Client{Timeout: 2 * time.Minute}
		snapshot, err = screening.DownloadSDN(context.Background(), client, screening.SDNURL, *snapshotPath)
		if err != nil {
			return err
		}
	}

	service, err := screening.NewService(accounts, snapshot, *cacheSize, includeWeak)
	if err != nil {
		return err
	}
	service.SetLogger(log.Default())
	server := &http.Server{
		Addr:              *listenAddress,
		Handler:           service.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("loaded %d accounts and %d SDN records (published %s); listening on %s", len(accounts), len(snapshot.Records), snapshot.Published, *listenAddress)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
