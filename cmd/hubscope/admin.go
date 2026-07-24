package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/taliove/hubscope/internal/store"
)

// minPasswordLen is the minimum acceptable password length at the CLI layer.
// It guards against trivially weak bootstrap credentials; stronger policy is
// the operator's responsibility.
const minPasswordLen = 8

// validHubRoles is the set of roles a hub-scoped user may hold. super_admin is
// intentionally excluded: it is global (hub_id NULL) and created only when
// neither --hub nor --role is supplied.
var validHubRoles = map[string]bool{
	store.RoleAdmin:    true,
	store.RoleOperator: true,
	store.RoleViewer:   true,
}

// runAdmin is the entry point for the 'admin' subcommand. It opens the store
// (DATA_PATH) and dispatches to the named subcommand. main.go routes here
// when os.Args[1] == "admin".
func runAdmin(args []string) error {
	if len(args) == 0 {
		return errors.New("admin: missing subcommand (expected 'create')")
	}
	dataPath := envOr("DATA_PATH", defaultDataPath)
	db, err := store.Open(dataPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	switch args[0] {
	case "create":
		return runAdminCreate(db, args[1:])
	default:
		return fmt.Errorf("admin: unknown subcommand %q", args[0])
	}
}

// runAdminCreate implements 'hubscope admin create'. It parses
// --username/--password (required) and --hub/--role (optional, must
// co-occur). With neither --hub nor --role it creates a global super_admin
// (hub_id NULL); with both it creates a hub-scoped user. The password is
// bcrypt-hashed (DefaultCost) before reaching the store; the store never
// receives plaintext.
func runAdminCreate(db *store.DB, args []string) error {
	fs := flag.NewFlagSet("admin create", flag.ContinueOnError)
	username := fs.String("username", "", "username (required)")
	password := fs.String("password", "", "password (required, min 8 chars)")
	hubID := fs.Int64("hub", 0, "hub id (required with --role for a hub-scoped user)")
	role := fs.String("role", "", "role for a hub-scoped user (admin|operator|viewer, required with --hub)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *username == "" {
		return errors.New("admin create: --username is required")
	}
	if len(*password) < minPasswordLen {
		return fmt.Errorf("admin create: --password must be at least %d characters", minPasswordLen)
	}

	hubSet := *hubID != 0
	roleSet := *role != ""
	if hubSet != roleSet {
		return errors.New("admin create: --hub and --role must both be set (or both omitted for super_admin)")
	}
	hubScoped := hubSet

	var targetHubID *int64
	assignRole := store.RoleSuperAdmin
	if hubScoped {
		if !validHubRoles[*role] {
			return fmt.Errorf("admin create: --role %q is not valid (admin|operator|viewer)", *role)
		}
		h, err := db.GetHub(*hubID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("admin create: hub %d not found", *hubID)
			}
			return fmt.Errorf("admin create: look up hub: %w", err)
		}
		_ = h // existence confirmed; no need for the row
		id := *hubID
		targetHubID = &id
		assignRole = *role
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("admin create: hash password: %w", err)
	}

	if _, err := db.CreateUser(*username, string(hash), targetHubID, assignRole); err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			return fmt.Errorf("admin create: %w (username %q)", store.ErrUsernameTaken, *username)
		}
		return fmt.Errorf("admin create: %w", err)
	}
	return nil
}
