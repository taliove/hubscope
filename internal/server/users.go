package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/taliove/hubscope/internal/store"
)

// userDTO is the API representation of a User. It never carries the password
// hash (W6 — credentials are never echoed back). hub_name is resolved for
// display so the management UI does not need a second round-trip.
type userDTO struct {
	ID        int64   `json:"id"`
	Username  string  `json:"username"`
	Role      string  `json:"role"`
	HubID     *int64  `json:"hub_id"`
	HubName   *string `json:"hub_name"`
	Enabled   bool    `json:"enabled"`
	CreatedAt string  `json:"created_at"`
}

// toUserDTO maps a store.User to its API representation, resolving the hub
// name for display. A missing hub row (deleted after the user was created)
// leaves hub_name nil rather than failing the whole list.
func (s *Server) toUserDTO(u store.User) userDTO {
	dto := userDTO{
		ID:        u.ID,
		Username:  u.Username,
		Role:      u.Role,
		HubID:     u.HubID,
		Enabled:   u.Enabled,
		CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if u.HubID != nil {
		if hub, err := s.db.GetHub(*u.HubID); err == nil {
			name := hub.Name
			dto.HubName = &name
		}
	}
	return dto
}

// minPasswordLen is the minimum length enforced at the handler layer for both
// create and reset. The store does not hash, so it does not enforce length.
const minPasswordLen = 8

// createUserRequest is the body for POST /api/users.
type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
	// HubID is required for hub-scoped roles (admin/operator/viewer) and must
	// be null for super_admin. super_admin sets it explicitly (any hub);
	// admin omits it and the handler pins it to the session's hub.
	HubID *int64 `json:"hub_id"`
}

// handleListUsers handles GET /api/users. super_admin sees all users; admin
// sees only users in the session user's hub. Per the hub query-isolation
// invariant (spec 0005), the branch on session role is the only path to
// ListUsersAll.
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	u := sessionUser(r)
	var users []store.User
	var err error
	if u.Role == store.RoleSuperAdmin {
		users, err = s.db.ListUsersAll()
	} else if u.HubID == nil {
		// A hub-scoped role without a hub_id is a data inconsistency; fall
		// back to an empty result rather than leaking the full set.
		users = []store.User{}
	} else {
		users, err = s.db.ListUsersByHub(*u.HubID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	dtos := make([]userDTO, 0, len(users))
	for _, usr := range users {
		dtos = append(dtos, s.toUserDTO(usr))
	}
	writeData(w, http.StatusOK, dtos)
}

// handleCreateUser handles POST /api/users.
//
// Authorization:
//   - super_admin may create any role and set hub_id freely (super_admin's
//     hub_id must be null; a non-null hub_id for super_admin is rejected so
//     a typo cannot create an inconsistent global user).
//   - admin may only create operator/viewer in the session user's own hub;
//     any other role, or a hub_id that is not the session hub, is 403.
//
// The caller never learns whether a target hub exists when they are not
// authorized (403 is returned before any DB lookup for cross-hub attempts).
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	u := sessionUser(r)
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Role = strings.TrimSpace(req.Role)
	if req.Username == "" || req.Password == "" || req.Role == "" {
		writeError(w, http.StatusBadRequest, "username, password, and role are required")
		return
	}
	if len(req.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("password must be at least %d characters", minPasswordLen))
		return
	}
	if !isValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, "invalid role")
		return
	}

	// Authorization: resolve the effective hub_id and validate role scope.
	hubID, err := resolveCreateHubID(u, req.Role, req.HubID)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	// Verify the target hub exists (for hub-scoped roles). super_admin
	// targeting a nonexistent hub gets a 400; admin's own hub is known to
	// exist (the session would not be valid otherwise), so the lookup is
	// skipped for admin to avoid a redundant query.
	if req.Role != store.RoleSuperAdmin {
		if hubID == nil {
			writeError(w, http.StatusBadRequest, "hub_id is required for this role")
			return
		}
		if _, herr := s.db.GetHub(*hubID); herr != nil {
			writeError(w, http.StatusBadRequest, "hub not found")
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	created, err := s.db.CreateUser(req.Username, string(hash), hubID, req.Role)
	if err != nil {
		if err == store.ErrUsernameTaken {
			writeError(w, http.StatusConflict, "username already taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	s.audit(r, "user.create", "user", strconv.FormatInt(created.ID, 10),
		fmt.Sprintf("username=%q role=%s hub_id=%v", created.Username, created.Role, created.HubID), "success")
	writeData(w, http.StatusCreated, s.toUserDTO(*created))
}

// resolveCreateHubID applies the authorization rules for POST /api/users and
// returns the effective hub_id pointer (nil for super_admin). An error
// means the request is forbidden.
func resolveCreateHubID(u *SessionUser, role string, requested *int64) (*int64, error) {
	if u.Role == store.RoleSuperAdmin {
		if role == store.RoleSuperAdmin {
			if requested != nil {
				return nil, fmt.Errorf("super_admin must not carry a hub_id")
			}
			return nil, nil
		}
		return requested, nil
	}
	// admin: only operator/viewer in the session's own hub.
	if role != store.RoleOperator && role != store.RoleViewer {
		return nil, fmt.Errorf("admin can only create operator or viewer")
	}
	if u.HubID == nil {
		return nil, fmt.Errorf("insufficient role")
	}
	if requested != nil && *requested != *u.HubID {
		return nil, fmt.Errorf("admin can only create users in the own hub")
	}
	return u.HubID, nil
}

// patchUserRequest is the body for PATCH /api/users/{id}. Every field is
// optional; a nil/absent field leaves the column unchanged. hub_id is NOT
// patchable here (ticket 67: PATCH covers role/enabled only); when a
// super_admin promotes a user to super_admin the handler clears hub_id to
// NULL to keep the "super_admin is global" invariant (store contract).
type patchUserRequest struct {
	Role    *string `json:"role"`
	Enabled *bool   `json:"enabled"`
}

// handlePatchUser handles PATCH /api/users/{id}.
//
// Authorization:
//   - super_admin may change any user's role/enabled.
//   - admin may only change users in the session user's own hub, and only
//     the enabled flag (not role — admin reshuffling roles is a
//     super_admin concern; an admin promoting themselves would also
//     sidestep the self-demotion guard). Cross-hub target is 403.
//   - Self role change is forbidden (u.UserID == id and role != nil → 403)
//     so a user cannot promote or demote themselves (prevents lockout and
//     privilege escalation).
func (s *Server) handlePatchUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	u := sessionUser(r)

	var req patchUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Role != nil {
		trimmed := strings.TrimSpace(*req.Role)
		req.Role = &trimmed
		if !isValidRole(*req.Role) {
			writeError(w, http.StatusBadRequest, "invalid role")
			return
		}
	}

	// Self role change is forbidden (prevents both self-promotion and the
	// lockout self-demotion would cause).
	if req.Role != nil && u.UserID == id {
		writeError(w, http.StatusForbidden, "cannot change your own role")
		return
	}

	// Load the target so cross-hub access can be checked before any write.
	target, err := s.db.GetUserByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := assertCanManageUser(u, target); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	// admin may only flip enabled, not role. Pin role to nil so a stray
	// body field is ignored rather than honored.
	if u.Role == store.RoleAdmin {
		req.Role = nil
	}

	// When super_admin promotes a user to super_admin, clear hub_id to keep
	// the "super_admin is global" invariant. A super_admin carrying a
	// non-NULL hub_id is a data inconsistency the store contract forbids.
	clearHub := req.Role != nil && *req.Role == store.RoleSuperAdmin && target.HubID != nil

	updated, err := s.db.UpdateUser(id, req.Role, nil, req.Enabled)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	if clearHub {
		if err := s.db.ClearUserHubID(id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to clear hub binding")
			return
		}
		updated.HubID = nil
	}

	s.audit(r, "user.update", "user", strconv.FormatInt(id, 10), patchUserDetail(req, clearHub), "success")
	writeData(w, http.StatusOK, s.toUserDTO(*updated))
}

// resetUserPasswordRequest is the body for PUT /api/users/{id}/password.
type resetUserPasswordRequest struct {
	Password string `json:"password"`
}

// handleResetUserPassword handles PUT /api/users/{id}/password. super_admin
// may reset any user; admin may reset users in the session user's own hub.
// The new password must meet the minimum length; this is a forced reset, so
// the old password is not required (the caller is already authorized).
func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	u := sessionUser(r)

	var req resetUserPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("password must be at least %d characters", minPasswordLen))
		return
	}

	target, err := s.db.GetUserByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := assertCanManageUser(u, target); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := s.db.SetUserPassword(id, string(hash)); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}

	s.audit(r, "user.reset_password", "user", strconv.FormatInt(id, 10), "", "success")
	writeNoContent(w)
}

// handleDeleteUser handles DELETE /api/users/{id}. super_admin may delete any
// user; admin may delete users in the session user's own hub. Self-delete is
// forbidden (would orphan the current session and leave no admin to recover).
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	u := sessionUser(r)

	if u.UserID == id {
		writeError(w, http.StatusForbidden, "cannot delete your own account")
		return
	}

	target, err := s.db.GetUserByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := assertCanManageUser(u, target); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	if err := s.db.DeleteUser(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	s.audit(r, "user.delete", "user", strconv.FormatInt(id, 10), "", "success")
	writeNoContent(w)
}

// assertCanManageUser enforces the cross-hub boundary for write operations
// on a single user. super_admin may act on any user; admin may act only when
// the target's hub_id equals the session user's hub_id (both must be non-nil
// and equal). A hub-scoped admin with a nil hub_id is treated as
// insufficient role (data inconsistency, fail closed).
func assertCanManageUser(u *SessionUser, target *store.User) error {
	if u.Role == store.RoleSuperAdmin {
		return nil
	}
	if u.Role != store.RoleAdmin {
		return fmt.Errorf("insufficient role")
	}
	if u.HubID == nil || target.HubID == nil || *u.HubID != *target.HubID {
		return fmt.Errorf("insufficient role")
	}
	return nil
}

// isValidRole reports whether r is one of the four defined roles.
func isValidRole(r string) bool {
	switch r {
	case store.RoleSuperAdmin, store.RoleAdmin, store.RoleOperator, store.RoleViewer:
		return true
	}
	return false
}

// patchUserDetail summarizes which fields a PATCH touched, for the audit
// row. A role promotion to super_admin is annotated so the hub-binding
// clear is visible in the audit trail.
func patchUserDetail(req patchUserRequest, clearHub bool) string {
	fields := []string{}
	if req.Role != nil {
		fields = append(fields, "role")
	}
	if clearHub {
		fields = append(fields, "hub_id(cleared)")
	}
	if req.Enabled != nil {
		fields = append(fields, "enabled="+strconv.FormatBool(*req.Enabled))
	}
	if len(fields) == 0 {
		return "no-op"
	}
	return "fields=" + strings.Join(fields, ",")
}
