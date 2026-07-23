package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/taliove2009/hubscope/internal/store"
)

// patchAs issues a JSON PATCH with the given client and returns the response.
func patchAs(t *testing.T, client *http.Client, url string, body interface{}) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PATCH", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", url, err)
	}
	return resp
}

// deleteAs issues a DELETE with the given client and returns the response.
func deleteAs(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("DELETE", url, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	return resp
}

// userDTO mirrors server.userDTO for test decoding. password_hash is never
// present, which the leak test asserts on the raw body instead.
type userDTO struct {
	ID       int64   `json:"id"`
	Username string  `json:"username"`
	Role     string  `json:"role"`
	HubID    *int64  `json:"hub_id"`
	HubName  *string `json:"hub_name"`
	Enabled  bool    `json:"enabled"`
}

// decodeUsersEnvelope decodes the {"data": [...users]} envelope into a slice.
func decodeUsersEnvelope(t *testing.T, body io.Reader) []userDTO {
	t.Helper()
	var env envelope
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		t.Fatalf("decode users envelope: %v", err)
	}
	var users []userDTO
	if err := json.Unmarshal(env.Data, &users); err != nil {
		t.Fatalf("unmarshal users: %v", err)
	}
	return users
}

// TestUserManagementCreateAndIsolation covers the POST authorization matrix:
// admin can create a viewer in the own hub (201), admin cannot create a
// viewer in another hub (403), admin cannot create an admin (403),
// super_admin can create any role including super_admin (201), and operator
// is blocked from the whole route (403).
func TestUserManagementCreateAndIsolation(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	hubA, err := db.CreateHub("um-hub-a", "http://a.test", "tok-um-a-0000")
	if err != nil {
		t.Fatalf("create hub a: %v", err)
	}
	hubB, err := db.CreateHub("um-hub-b", "http://b.test", "tok-um-b-0000")
	if err != nil {
		t.Fatalf("create hub b: %v", err)
	}

	seedUserWithRole(t, db, "um-a-admin", store.RoleAdmin, &hubA.ID)
	seedUserWithRole(t, db, "um-sa", store.RoleSuperAdmin, nil)
	seedUserWithRole(t, db, "um-oper", store.RoleOperator, &hubA.ID)

	aAdminClient := loginAsClient(t, ts.URL, "um-a-admin")
	saClient := loginAsClient(t, ts.URL, "um-sa")
	operClient := loginAsClient(t, ts.URL, "um-oper")

	// admin creates a viewer in own hub → 201.
	resp := postAs(t, aAdminClient, ts.URL+"/api/users", map[string]interface{}{
		"username": "um-a-viewer",
		"password": "fake-pw-um-a-viewer-001",
		"role":     store.RoleViewer,
		"hub_id":   hubA.ID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("admin create own-hub viewer: expected 201, got %d", resp.StatusCode)
	}
	created := decodeSingleUser(t, resp.Body)
	resp.Body.Close()
	if created.Role != store.RoleViewer || created.HubID == nil || *created.HubID != hubA.ID {
		t.Errorf("created viewer payload mismatch: %+v", created)
	}

	// admin creates a viewer in Hub-B → 403 (cross-hub; returned before the
	// hub lookup, so the target hub existence is not leaked).
	resp = postAs(t, aAdminClient, ts.URL+"/api/users", map[string]interface{}{
		"username": "um-b-viewer",
		"password": "fake-pw-um-b-viewer-001",
		"role":     store.RoleViewer,
		"hub_id":   hubB.ID,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("admin create other-hub viewer: expected 403, got %d", resp.StatusCode)
	}

	// admin creates an admin → 403 (role not allowed for admin).
	resp = postAs(t, aAdminClient, ts.URL+"/api/users", map[string]interface{}{
		"username": "um-a-admin2",
		"password": "fake-pw-um-a-admin2-001",
		"role":     store.RoleAdmin,
		"hub_id":   hubA.ID,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("admin create admin: expected 403, got %d", resp.StatusCode)
	}

	// super_admin creates a super_admin (no hub_id) → 201.
	resp = postAs(t, saClient, ts.URL+"/api/users", map[string]interface{}{
		"username": "um-sa2",
		"password": "fake-pw-um-sa2-000001",
		"role":     store.RoleSuperAdmin,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("super_admin create super_admin: expected 201, got %d", resp.StatusCode)
	}
	sa2 := decodeSingleUser(t, resp.Body)
	resp.Body.Close()
	if sa2.HubID != nil {
		t.Errorf("super_admin hub_id must be nil, got %v", sa2.HubID)
	}

	// super_admin creates an admin in Hub-B → 201.
	resp = postAs(t, saClient, ts.URL+"/api/users", map[string]interface{}{
		"username": "um-b-admin",
		"password": "fake-pw-um-b-admin-0001",
		"role":     store.RoleAdmin,
		"hub_id":   hubB.ID,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("super_admin create Hub-B admin: expected 201, got %d", resp.StatusCode)
	}

	// operator POST /api/users → 403 (route is admin+super_admin only).
	resp = postAs(t, operClient, ts.URL+"/api/users", map[string]interface{}{
		"username": "um-oper-viewer",
		"password": "fake-pw-um-oper-vw-001",
		"role":     store.RoleViewer,
		"hub_id":   hubA.ID,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("operator create user: expected 403, got %d", resp.StatusCode)
	}

	// short password → 400.
	resp = postAs(t, saClient, ts.URL+"/api/users", map[string]interface{}{
		"username": "um-short",
		"password": "short",
		"role":     store.RoleViewer,
		"hub_id":   hubA.ID,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("short password create: expected 400, got %d", resp.StatusCode)
	}
}

// TestUserManagementListIsolation verifies GET /api/users keeps hub-scoped
// admins from seeing other hubs' users, while super_admin sees everyone.
// Also asserts the response never carries password_hash.
func TestUserManagementListIsolation(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	hubA, err := db.CreateHub("list-hub-a", "http://a.test", "tok-list-a-0000")
	if err != nil {
		t.Fatalf("create hub a: %v", err)
	}
	hubB, err := db.CreateHub("list-hub-b", "http://b.test", "tok-list-b-0000")
	if err != nil {
		t.Fatalf("create hub b: %v", err)
	}

	seedUserWithRole(t, db, "list-a-admin", store.RoleAdmin, &hubA.ID)
	seedUserWithRole(t, db, "list-b-admin", store.RoleAdmin, &hubB.ID)
	seedUserWithRole(t, db, "list-sa", store.RoleSuperAdmin, nil)
	seedUserWithRole(t, db, "list-a-oper", store.RoleOperator, &hubA.ID)
	seedUserWithRole(t, db, "list-b-oper", store.RoleOperator, &hubB.ID)

	aAdminClient := loginAsClient(t, ts.URL, "list-a-admin")
	saClient := loginAsClient(t, ts.URL, "list-sa")

	// Hub-A admin: sees Hub-A users, not Hub-B's.
	resp, err := aAdminClient.Get(ts.URL + "/api/users")
	if err != nil {
		t.Fatalf("admin GET users: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin GET users: expected 200, got %d", resp.StatusCode)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(bodyBytes), "password_hash") {
		t.Errorf("admin GET users: response leaks password_hash: %s", string(bodyBytes))
	}
	if strings.Contains(string(bodyBytes), "list-b-oper") {
		t.Errorf("admin GET users: Hub-A admin sees Hub-B user (list-b-oper): %s", string(bodyBytes))
	}
	if !strings.Contains(string(bodyBytes), "list-a-oper") {
		t.Errorf("admin GET users: expected own-hub user list-a-oper present, got %s", string(bodyBytes))
	}

	// super_admin: sees both hubs' users.
	resp, err = saClient.Get(ts.URL + "/api/users")
	if err != nil {
		t.Fatalf("sa GET users: %v", err)
	}
	bodyBytes, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(bodyBytes), "list-a-oper") || !strings.Contains(string(bodyBytes), "list-b-oper") {
		t.Errorf("sa GET users: expected both hubs' users, got %s", string(bodyBytes))
	}
}

// TestUserManagementDisabledLoginAndReset verifies the end-to-end effect of
// the enabled flag and password reset on the login path: a disabled user
// cannot log in, and after a reset the new password works.
func TestUserManagementDisabledLoginAndReset(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	hub, err := db.CreateHub("dl-hub", "http://dl.test", "tok-dl-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	seedUserWithRole(t, db, "dl-sa", store.RoleSuperAdmin, nil)
	seedUserWithRole(t, db, "dl-view", store.RoleViewer, &hub.ID)
	saClient := loginAsClient(t, ts.URL, "dl-sa")

	// Disabled user cannot log in. Disable via PATCH (admin path).
	listBefore := decodeUsersList(t, saClient, ts.URL+"/api/users")
	var dlViewID int64
	for _, u := range listBefore {
		if u.Username == "dl-view" {
			dlViewID = u.ID
		}
	}
	if dlViewID == 0 {
		t.Fatal("dl-view not found in list")
	}
	falseVal := false
	resp := patchAs(t, saClient, fmt.Sprintf("%s/api/users/%d", ts.URL, dlViewID),
		map[string]interface{}{"enabled": falseVal})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable user: expected 200, got %d", resp.StatusCode)
	}

	// Login as the now-disabled user must fail (401).
	loginResp, err := http.Post(ts.URL+"/api/auth/login", "application/json",
		bytes.NewBufferString(`{"username":"dl-view","password":"`+roleTestPassword("dl-view")+`"}`))
	if err != nil {
		t.Fatalf("disabled login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("disabled login: expected 401, got %d", loginResp.StatusCode)
	}

	// Reset the password via PUT (super_admin). The old password is not
	// required — the caller is already authorized.
	newPw := "fake-pw-reset-newpassword-001"
	resp = putAs(t, saClient, fmt.Sprintf("%s/api/users/%d/password", ts.URL, dlViewID),
		map[string]interface{}{"password": newPw})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset password: expected 204, got %d", resp.StatusCode)
	}

	// Re-enable the user (still disabled), then log in with the new password.
	trueVal := true
	resp = patchAs(t, saClient, fmt.Sprintf("%s/api/users/%d", ts.URL, dlViewID),
		map[string]interface{}{"enabled": trueVal})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("re-enable user: expected 200, got %d", resp.StatusCode)
	}
	loginResp, err = http.Post(ts.URL+"/api/auth/login", "application/json",
		bytes.NewBufferString(`{"username":"dl-view","password":"`+newPw+`"}`))
	if err != nil {
		t.Fatalf("reset login: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Errorf("login after reset: expected 200, got %d", loginResp.StatusCode)
	}
}

// TestUserManagementSelfChangeGuards verifies the self-protection rules:
// self role change (PATCH role) and self-delete are both 403.
func TestUserManagementSelfChangeGuards(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	hub, err := db.CreateHub("self-hub", "http://self.test", "tok-self-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}
	seedUserWithRole(t, db, "self-admin", store.RoleAdmin, &hub.ID)
	aClient := loginAsClient(t, ts.URL, "self-admin")

	list := decodeUsersList(t, aClient, ts.URL+"/api/users")
	if len(list) != 1 {
		t.Fatalf("expected only the session user in list, got %d", len(list))
	}
	myID := list[0].ID

	// Self role change → 403.
	resp := patchAs(t, aClient, fmt.Sprintf("%s/api/users/%d", ts.URL, myID),
		map[string]interface{}{"role": store.RoleViewer})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("self role change: expected 403, got %d", resp.StatusCode)
	}

	// Self delete → 403.
	resp = deleteAs(t, aClient, fmt.Sprintf("%s/api/users/%d", ts.URL, myID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("self delete: expected 403, got %d", resp.StatusCode)
	}
}

// TestUserManagementCrossHubPatchIs403 verifies an admin cannot PATCH/DELETE a
// user that belongs to another hub (returns 403, not 404, so the target's
// existence is not leaked — consistent with requireRole).
func TestUserManagementCrossHubPatchIs403(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)

	hubA, err := db.CreateHub("xh-hub-a", "http://a.test", "tok-xh-a-0000")
	if err != nil {
		t.Fatalf("create hub a: %v", err)
	}
	hubB, err := db.CreateHub("xh-hub-b", "http://b.test", "tok-xh-b-0000")
	if err != nil {
		t.Fatalf("create hub b: %v", err)
	}
	seedUserWithRole(t, db, "xh-a-admin", store.RoleAdmin, &hubA.ID)
	seedUserWithRole(t, db, "xh-b-viewer", store.RoleViewer, &hubB.ID)
	aClient := loginAsClient(t, ts.URL, "xh-a-admin")

	// Hub-A admin cannot disable a Hub-B user → 403.
	resp := patchAs(t, aClient, ts.URL+"/api/users/9999",
		map[string]interface{}{"enabled": false})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-hub patch nonexistent: expected 404 or 403, got %d", resp.StatusCode)
	}

	// Find the Hub-B viewer id via the store (not via the API, which the
	// admin cannot reach).
	bView, err := db.GetUserByUsername("xh-b-viewer")
	if err != nil {
		t.Fatalf("get b-viewer: %v", err)
	}
	falseVal := false
	resp = patchAs(t, aClient, fmt.Sprintf("%s/api/users/%d", ts.URL, bView.ID),
		map[string]interface{}{"enabled": falseVal})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-hub patch: expected 403, got %d", resp.StatusCode)
	}

	// Cross-hub delete → 403.
	resp = deleteAs(t, aClient, fmt.Sprintf("%s/api/users/%d", ts.URL, bView.ID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-hub delete: expected 403, got %d", resp.StatusCode)
	}
}

// decodeSingleUser decodes a single {"data": {...user}} envelope.
func decodeSingleUser(t *testing.T, body io.Reader) userDTO {
	t.Helper()
	var env envelope
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		t.Fatalf("decode user envelope: %v", err)
	}
	var u userDTO
	if err := json.Unmarshal(env.Data, &u); err != nil {
		t.Fatalf("unmarshal user: %v", err)
	}
	return u
}

// decodeUsersList GETs the users endpoint and decodes the list.
func decodeUsersList(t *testing.T, client *http.Client, url string) []userDTO {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET users: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET users: expected 200, got %d", resp.StatusCode)
	}
	return decodeUsersEnvelope(t, resp.Body)
}
