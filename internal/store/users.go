package store

import (
	"database/sql"
	"errors"
	"time"
)

// User roles. super_admin is global (hub_id NULL); the rest are scoped to a
// specific hub via hub_id.
const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleOperator   = "operator"
	RoleViewer     = "viewer"
)

// ErrUsernameTaken is returned when creating a user whose username already
// exists.
var ErrUsernameTaken = errors.New("username already taken")

// ErrUserNotFound is returned when no user matches the lookup criterion.
var ErrUserNotFound = errors.New("user not found")

// User is one authenticated identity. password_hash stores a bcrypt hash;
// hub_id is NULL for super_admin.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	HubID        *int64
	Role         string
	Enabled      bool
	CreatedAt    time.Time
}

// userColumns is the canonical column list for scanning a User.
const userColumns = "id, username, password_hash, hub_id, role, enabled, created_at"

// scanUser scans a row containing userColumns into a User.
func scanUser(s rowScanner) (User, error) {
	var u User
	var hubID sql.NullInt64
	var enabled int
	var createdAt string
	if err := s.Scan(&u.ID, &u.Username, &u.PasswordHash, &hubID, &u.Role, &enabled, &createdAt); err != nil {
		return User{}, err
	}
	if hubID.Valid {
		v := hubID.Int64
		u.HubID = &v
	}
	u.Enabled = enabled == 1
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return u, nil
}

// CreateUser inserts a new user. password_hash must already be a bcrypt hash;
// the store layer does not hash. hub_id is nil for super_admin. Returns
// ErrUsernameTaken when the username UNIQUE constraint is violated.
func (db *DB) CreateUser(username, passwordHash string, hubID *int64, role string) (*User, error) {
	now := time.Now().UTC()
	result, err := db.conn.Exec(
		"INSERT INTO users (username, password_hash, hub_id, role, enabled, created_at) VALUES (?, ?, ?, ?, 1, ?)",
		username, passwordHash, hubID, role, now.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUsernameTaken
		}
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		HubID:        hubID,
		Role:         role,
		Enabled:      true,
		CreatedAt:    now,
	}, nil
}

// GetUserByUsername looks up a user by username. Returns ErrUserNotFound when
// no row matches; this is the login lookup path.
func (db *DB) GetUserByUsername(username string) (*User, error) {
	u, err := scanUser(db.conn.QueryRow(
		"SELECT "+userColumns+" FROM users WHERE username = ?",
		username,
	))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

// SetUserEnabled flips the enabled flag on a user. Disabling a user causes
// requireSession to reject the user's still-valid session cookie (defense
// against the window between a credential being revoked and its token TTL
// expiring). It is a store primitive used by the auth gate and (later) the
// user-management API; it does not itself perform any authorization.
func (db *DB) SetUserEnabled(username string, enabled bool) error {
	flag := 0
	if enabled {
		flag = 1
	}
	_, err := db.conn.Exec("UPDATE users SET enabled = ? WHERE username = ?", flag, username)
	return err
}

// GetUserByID looks up a user by primary key. Returns ErrUserNotFound when no
// row matches; this is the /auth/me identity path.
func (db *DB) GetUserByID(id int64) (*User, error) {
	u, err := scanUser(db.conn.QueryRow(
		"SELECT "+userColumns+" FROM users WHERE id = ?",
		id,
	))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}
