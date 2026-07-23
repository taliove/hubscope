package store

import (
	"testing"
)

// openTestDB opens a real SQLite database in a temporary directory for store
// package tests. It mirrors the server test helper but lives in-package so
// store CRUD can be exercised directly.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestCreateUser verifies a super_admin (hub_id NULL) is inserted with the
// expected fields and enabled defaults to true.
func TestCreateUser(t *testing.T) {
	db := openTestDB(t)

	u, err := db.CreateUser("admin", "fake-hash", nil, RoleSuperAdmin)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if u.Username != "admin" {
		t.Fatalf("username: expected admin, got %q", u.Username)
	}
	if u.PasswordHash != "fake-hash" {
		t.Fatalf("password_hash: expected fake-hash, got %q", u.PasswordHash)
	}
	if u.HubID != nil {
		t.Fatalf("super_admin hub_id must be nil, got %v", *u.HubID)
	}
	if u.Role != RoleSuperAdmin {
		t.Fatalf("role: expected %q, got %q", RoleSuperAdmin, u.Role)
	}
	if !u.Enabled {
		t.Fatal("new user must be enabled")
	}
	if u.CreatedAt.IsZero() {
		t.Fatal("created_at must be set")
	}
}

// TestCreateUserDuplicate verifies the UNIQUE(username) constraint is surfaced
// as ErrUsernameTaken, so the CLI can give a clear "already exists" message.
func TestCreateUserDuplicate(t *testing.T) {
	db := openTestDB(t)

	if _, err := db.CreateUser("admin", "hash-1", nil, RoleSuperAdmin); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := db.CreateUser("admin", "hash-2", nil, RoleSuperAdmin)
	if err != ErrUsernameTaken {
		t.Fatalf("duplicate create: expected ErrUsernameTaken, got %v", err)
	}
}

// TestGetUserByUsername verifies the login lookup path returns the stored
// hash and identity fields.
func TestGetUserByUsername(t *testing.T) {
	db := openTestDB(t)

	if _, err := db.CreateUser("alice", "hashed", nil, RoleSuperAdmin); err != nil {
		t.Fatalf("create: %v", err)
	}

	u, err := db.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("get by username: %v", err)
	}
	if u.Username != "alice" || u.PasswordHash != "hashed" || u.Role != RoleSuperAdmin {
		t.Fatalf("unexpected user: %+v", u)
	}
}

// TestGetUserByUsernameNotFound verifies a missing username returns
// ErrUserNotFound (the login path distinguishes "no such user" from DB
// errors; the API layer unifies the response to prevent enumeration).
func TestGetUserByUsernameNotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := db.GetUserByUsername("nobody")
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

// TestGetUserByID verifies the /auth/me identity path.
func TestGetUserByID(t *testing.T) {
	db := openTestDB(t)

	created, err := db.CreateUser("bob", "bob-hash", nil, RoleSuperAdmin)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	u, err := db.GetUserByID(created.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if u.Username != "bob" || u.PasswordHash != "bob-hash" {
		t.Fatalf("unexpected user: %+v", u)
	}
}

// TestGetUserByIDNotFound verifies a missing ID returns ErrUserNotFound.
func TestGetUserByIDNotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := db.GetUserByID(9999)
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

// TestUserHubIDNotNilForHubUser verifies hub_id is correctly stored and
// retrieved as a non-nil pointer for hub-scoped roles.
func TestUserHubIDNotNilForHubUser(t *testing.T) {
	db := openTestDB(t)

	// Create a hub first so the FK is valid.
	hub, err := db.CreateHub("Test Hub", "http://example.com", "fake-token")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}

	hubID := hub.ID
	u, err := db.CreateUser("carol", "carol-hash", &hubID, RoleAdmin)
	if err != nil {
		t.Fatalf("create hub user: %v", err)
	}
	if u.HubID == nil || *u.HubID != hubID {
		t.Fatalf("hub_id: expected %d, got %v", hubID, u.HubID)
	}

	// round-trip through the DB
	found, err := db.GetUserByID(u.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found.HubID == nil || *found.HubID != hubID {
		t.Fatalf("round-trip hub_id: expected %d, got %v", hubID, found.HubID)
	}
}

// TestSessionSecretSettingRoundTrip verifies the settings table can store and
// retrieve the session_secret key, which server.New relies on.
func TestSessionSecretSettingRoundTrip(t *testing.T) {
	db := openTestDB(t)

	// Before any write, GetSetting returns the default.
	v, err := db.GetSetting(SettingSessionSecret, "")
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if v != "" {
		t.Fatalf("expected empty default, got %q", v)
	}

	secret := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
	if err := db.SetSetting(SettingSessionSecret, secret); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := db.GetSetting(SettingSessionSecret, "")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != secret {
		t.Fatalf("expected %q, got %q", secret, got)
	}
}
