package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/oej/opentea/internal/config"
	"github.com/oej/opentea/internal/db"
	"github.com/oej/opentea/internal/model"
	"github.com/oej/opentea/internal/repo"
)

// runCreateAdmin bootstraps the first (or an additional) admin user against
// the same database the server would use, then exits. This is the only way
// to create a user before any admin exists to log into the GUI/admin API
// with.
func runCreateAdmin(args []string) {
	fs := flag.NewFlagSet("createadmin", flag.ExitOnError)
	username := fs.String("username", "", "username for the new admin user (required)")
	password := fs.String("password", "", "password for the new admin user (required)")
	_ = fs.Parse(args)

	if *username == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "usage: opentea createadmin -username=<name> -password=<password>")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	sqlDB, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open database: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = sqlDB.Close() }()

	r := repo.New(sqlDB)
	user, err := r.CreateUser(context.Background(), *username, *password, model.RoleAdmin)
	if errors.Is(err, repo.ErrUsernameTaken) {
		fmt.Fprintf(os.Stderr, "username %q is already taken\n", *username)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "create admin user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("created admin user %q (uuid %s)\n", user.Username, user.UUID)
}
