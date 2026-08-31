// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/oej/opentea/internal/openteapublisher"
)

// runCreateStaff bootstraps the first (or an additional) staff account
// against the same database the server would use, then exits -- mirrors
// cmd/opentea/createadmin.go exactly. This is the only way to create an
// account before any staff member exists to log in with.
func runCreateStaff(args []string) {
	fs := flag.NewFlagSet("createstaff", flag.ExitOnError)
	username := fs.String("username", "", "username for the new staff account (required)")
	password := fs.String("password", "", "password for the new staff account (required)")
	_ = fs.Parse(args)

	if *username == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "usage: openteapublisher createstaff -username=<name> -password=<password>")
		os.Exit(1)
	}

	cfg := loadConfig()
	sqlDB, err := openteapublisher.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open database: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = sqlDB.Close() }()

	r := openteapublisher.New(sqlDB)
	staff, err := r.CreateStaff(context.Background(), *username, *password)
	if errors.Is(err, openteapublisher.ErrUsernameTaken) {
		fmt.Fprintf(os.Stderr, "username %q is already taken\n", *username)
		os.Exit(1)
	}
	if errors.Is(err, openteapublisher.ErrPasswordTooShort) {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "create staff account: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("created staff account %q (uuid %s)\n", staff.Username, staff.UUID)
}
