package store

import (
	"database/sql"
	"time"
)

// ShareLink is one token-gated read-only door onto a single campaign report
// (ADR 0006). Revocation writes revoked_at; the row is never deleted so the
// audit trail of what was shared stays intact.
type ShareLink struct {
	ID         int64
	Token      string
	CampaignID int64
	CreatedBy  string
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

// shareLinkColumns is the canonical share_links column list.
const shareLinkColumns = "id, token, campaign_id, created_by, created_at, revoked_at"

// scanShareLink scans one share_links row.
func scanShareLink(s rowScanner) (ShareLink, error) {
	var l ShareLink
	var createdAt string
	var revokedAt sql.NullString
	if err := s.Scan(&l.ID, &l.Token, &l.CampaignID, &l.CreatedBy, &createdAt, &revokedAt); err != nil {
		return ShareLink{}, err
	}
	l.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if revokedAt.Valid {
		t, _ := time.Parse(time.RFC3339, revokedAt.String)
		l.RevokedAt = &t
	}
	return l, nil
}

// CreateShareLink inserts a live share link and returns the stored copy. The
// caller mints the token (crypto/rand at the server layer); the UNIQUE
// constraint is the backstop against collisions.
func (db *DB) CreateShareLink(token string, campaignID int64, createdBy string, now time.Time) (*ShareLink, error) {
	now = now.UTC()
	result, err := db.conn.Exec(`
		INSERT INTO share_links (token, campaign_id, created_by, created_at)
		VALUES (?, ?, ?, ?)
	`, token, campaignID, createdBy, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	link, err := scanShareLink(db.conn.QueryRow(
		"SELECT "+shareLinkColumns+" FROM share_links WHERE id = ?", id))
	if err != nil {
		return nil, err
	}
	return &link, nil
}

// ListShareLinks returns every share link, newest first, live and revoked
// alike (the admin management view shows both).
func (db *DB) ListShareLinks() ([]ShareLink, error) {
	rows, err := db.conn.Query(
		"SELECT " + shareLinkColumns + " FROM share_links ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ShareLink{}
	for rows.Next() {
		l, err := scanShareLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetShareLinkByToken returns the link for a token, or (nil, nil) when no
// link carries it. Callers treat a revoked link exactly like an absent one
// (uniform 404, no enumeration oracle).
func (db *DB) GetShareLinkByToken(token string) (*ShareLink, error) {
	l, err := scanShareLink(db.conn.QueryRow(
		"SELECT "+shareLinkColumns+" FROM share_links WHERE token = ?", token))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// RevokeShareLink stamps revoked_at on a live link, keeping the row for
// audit. It reports whether the link exists at all; revoking an
// already-revoked link is a no-op that still reports true (idempotent).
func (db *DB) RevokeShareLink(id int64, now time.Time) (bool, error) {
	var exists int
	if err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM share_links WHERE id = ?", id).Scan(&exists); err != nil {
		return false, err
	}
	if exists == 0 {
		return false, nil
	}
	_, err := db.conn.Exec(
		"UPDATE share_links SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL",
		now.UTC().Format(time.RFC3339), id)
	if err != nil {
		return false, err
	}
	return true, nil
}
