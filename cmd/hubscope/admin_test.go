package main

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/taliove2009/hubscope/internal/store"
)

// newAdminTestDB opens a real temp SQLite store for admin CLI tests. It
// mirrors the W1 seam (real DB, no internal mocks) at the cmd layer.
func newAdminTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestAdminCreateSuperAdmin verifies that a super_admin (no --hub/--role) is
// created with hub_id=nil, a bcrypt (non-plaintext) password hash, and is
// retrievable via GetUserByUsername.
func TestAdminCreateSuperAdmin(t *testing.T) {
	db := newAdminTestDB(t)

	if err := runAdminCreate(db, []string{
		"--username", "root",
		"--password", "supersecret",
	}); err != nil {
		t.Fatalf("runAdminCreate: %v", err)
	}

	u, err := db.GetUserByUsername("root")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if u.Role != store.RoleSuperAdmin {
		t.Fatalf("role: expected %q, got %q", store.RoleSuperAdmin, u.Role)
	}
	if u.HubID != nil {
		t.Fatalf("super_admin hub_id must be nil, got %v", *u.HubID)
	}
	if u.PasswordHash == "" || u.PasswordHash == "supersecret" {
		t.Fatalf("password_hash must be a bcrypt hash, got %q", u.PasswordHash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("supersecret")); err != nil {
		t.Fatalf("bcrypt compare: %v", err)
	}
}

// TestAdminCreateDuplicate verifies that re-creating an existing username
// returns the store.ErrUsernameTaken error wrapped in a friendly message.
func TestAdminCreateDuplicate(t *testing.T) {
	db := newAdminTestDB(t)

	args := []string{"--username", "dup", "--password", "password1"}
	if err := runAdminCreate(db, args); err != nil {
		t.Fatalf("first create: %v", err)
	}
	err := runAdminCreate(db, args)
	if !errors.Is(err, store.ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
}

// TestAdminCreateShortPassword verifies that a password shorter than 8 chars
// is rejected before any DB write.
func TestAdminCreateShortPassword(t *testing.T) {
	db := newAdminTestDB(t)

	err := runAdminCreate(db, []string{
		"--username", "short",
		"--password", "123",
	})
	if err == nil {
		t.Fatal("expected error for short password, got nil")
	}
	if _, lookupErr := db.GetUserByUsername("short"); !errors.Is(lookupErr, store.ErrUserNotFound) {
		t.Fatalf("short-password user must not be written, got lookupErr=%v", lookupErr)
	}
}

// TestAdminCreateMissingHubRolePair verifies that --hub without --role (or
// vice versa) is rejected: the pair must co-occur.
func TestAdminCreateMissingHubRolePair(t *testing.T) {
	db := newAdminTestDB(t)
	hub, err := db.CreateHub("h1", "http://example.com", "fake-token")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}

	// --hub without --role
	err = runAdminCreate(db, []string{
		"--username", "a", "--password", "password1",
		"--hub", strconvInt(hub.ID),
	})
	if err == nil || !strings.Contains(err.Error(), "must both be set") {
		t.Fatalf("--hub without --role: expected co-occurrence error, got %v", err)
	}

	// --role without --hub
	err = runAdminCreate(db, []string{
		"--username", "b", "--password", "password1",
		"--role", store.RoleViewer,
	})
	if err == nil || !strings.Contains(err.Error(), "must both be set") {
		t.Fatalf("--role without --hub: expected co-occurrence error, got %v", err)
	}
}

// TestAdminCreateHubScoped verifies that --hub + --role creates a hub-scoped
// user with the given role and hub_id.
func TestAdminCreateHubScoped(t *testing.T) {
	db := newAdminTestDB(t)
	hub, err := db.CreateHub("h1", "http://example.com", "fake-token")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}

	if err := runAdminCreate(db, []string{
		"--username", "viewer1",
		"--password", "password1",
		"--hub", strconvInt(hub.ID),
		"--role", store.RoleViewer,
	}); err != nil {
		t.Fatalf("runAdminCreate: %v", err)
	}

	u, err := db.GetUserByUsername("viewer1")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if u.Role != store.RoleViewer {
		t.Fatalf("role: expected %q, got %q", store.RoleViewer, u.Role)
	}
	if u.HubID == nil || *u.HubID != hub.ID {
		t.Fatalf("hub_id: expected %d, got %v", hub.ID, u.HubID)
	}
}

// TestAdminCreateHubNotFound verifies that a non-existent --hub id is
// rejected before any user is written.
func TestAdminCreateHubNotFound(t *testing.T) {
	db := newAdminTestDB(t)

	err := runAdminCreate(db, []string{
		"--username", "x", "--password", "password1",
		"--hub", "9999", "--role", store.RoleAdmin,
	})
	if err == nil {
		t.Fatal("expected hub-not-found error, got nil")
	}
	if _, lookupErr := db.GetUserByUsername("x"); !errors.Is(lookupErr, store.ErrUserNotFound) {
		t.Fatalf("user must not be written for missing hub, got %v", lookupErr)
	}
}

// TestAdminCreateInvalidRole verifies that an unknown --role is rejected.
func TestAdminCreateInvalidRole(t *testing.T) {
	db := newAdminTestDB(t)
	hub, err := db.CreateHub("h1", "http://example.com", "fake-token")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}

	err = runAdminCreate(db, []string{
		"--username", "y", "--password", "password1",
		"--hub", strconvInt(hub.ID), "--role", "wizard",
	})
	if err == nil {
		t.Fatal("expected invalid-role error, got nil")
	}
}

// TestAdminCreateEmptyUsername verifies that an empty username is rejected.
func TestAdminCreateEmptyUsername(t *testing.T) {
	db := newAdminTestDB(t)

	err := runAdminCreate(db, []string{"--password", "password1"})
	if err == nil {
		t.Fatal("expected error for empty username, got nil")
	}
}

// strconvInt is a tiny local int->string helper to avoid pulling strconv into
// every test case literal; kept local so the test file stays self-contained.
func strconvInt(v int64) string {
	// minimal decimal conversion (id is small and positive in tests)
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
