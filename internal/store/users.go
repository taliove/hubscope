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

// ListUsersAll returns every user, ordered by id. It is the super_admin list
// form; HTTP handlers must branch on the session user before reaching it
// (per the hub query-isolation invariant, spec 0005). password_hash is not
// exposed via API; callers that need the list for management render a DTO
// that omits it.
func (db *DB) ListUsersAll() ([]User, error) {
	rows, err := db.conn.Query("SELECT " + userColumns + " FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ListUsersByHub returns the users whose hub_id matches the given hub, ordered
// by id. super_admin (hub_id NULL) is excluded. It is the hub-scoped list
// form non-super_admin sessions must use.
func (db *DB) ListUsersByHub(hubID int64) ([]User, error) {
	rows, err := db.conn.Query(
		"SELECT "+userColumns+" FROM users WHERE hub_id = ? ORDER BY id",
		hubID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdateUser applies a partial patch to a user. A nil pointer leaves the
// corresponding column unchanged; role/hubID/enabled are the
// management-editable fields. Returns the updated row, or ErrUserNotFound
// when no row matches the id. The store layer does not hash; password
// rotation uses SetUserPassword.
func (db *DB) UpdateUser(id int64, role *string, hubID *int64, enabled *bool) (*User, error) {
	sets := []string{}
	args := []interface{}{}
	if role != nil {
		sets = append(sets, "role = ?")
		args = append(args, *role)
	}
	if hubID != nil {
		sets = append(sets, "hub_id = ?")
		args = append(args, *hubID)
	}
	if enabled != nil {
		flag := 0
		if *enabled {
			flag = 1
		}
		sets = append(sets, "enabled = ?")
		args = append(args, flag)
	}
	if len(sets) == 0 {
		// Nothing to update: return the current row without writing.
		return db.GetUserByID(id)
	}
	args = append(args, id)
	q := "UPDATE users SET " + joinStrings(sets, ", ") + " WHERE id = ?"
	if _, err := db.conn.Exec(q, args...); err != nil {
		return nil, err
	}
	return db.GetUserByID(id)
}

// SetUserPassword replaces the password_hash for a user. passwordHash must
// already be a bcrypt hash; the store does not hash. Returns ErrUserNotFound
// when no row matches (the UPDATE affects 0 rows when the id is absent).
func (db *DB) SetUserPassword(id int64, passwordHash string) error {
	res, err := db.conn.Exec("UPDATE users SET password_hash = ? WHERE id = ?", passwordHash, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// DeleteUser removes a user by id. Returns ErrUserNotFound when no row matches.
// The caller is responsible for authorization (preventing self-delete,
// cross-hub delete) before reaching this primitive.
func (db *DB) DeleteUser(id int64) error {
	res, err := db.conn.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// ClearUserHubID sets hub_id to NULL for a user. It is the invariant-
// restoring companion to a role promotion to super_admin (the store
// contract requires super_admin to be global). Returns ErrUserNotFound
// when no row matches.
func (db *DB) ClearUserHubID(id int64) error {
	res, err := db.conn.Exec("UPDATE users SET hub_id = NULL WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// joinStrings joins ss with sep. Kept unexported and local so the package
// does not pull in strings just for this one call site.
func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += sep + s
	}
	return out
}
