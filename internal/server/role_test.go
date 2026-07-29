package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/taliove/hubscope/internal/server"
	"github.com/taliove/hubscope/internal/store"
)

// roleTestPassword is the password for every role-test user, made distinct per
// username. It is not a real credential.
func roleTestPassword(username string) string { return "role-test-" + username }

// seedUserWithRole creates a user with the given role and optional hub_id.
// For non-super_admin roles hubID must point at an existing hub. The password
// is roleTestPassword(username) (bcrypt cost 4 keeps tests fast).
func seedUserWithRole(t *testing.T, db *store.DB, username, role string, hubID *int64) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(roleTestPassword(username)), 4)
	if err != nil {
		t.Fatalf("bcrypt hash: %v", err)
	}
	if _, err := db.CreateUser(username, string(hash), hubID, role); err != nil {
		t.Fatalf("seed user %s: %v", username, err)
	}
}

// loginAsClient logs in as username and returns a cookie-bearing client. It
// fatals if login does not return 200.
func loginAsClient(t *testing.T, baseURL, username string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}
	resp, err := client.Post(baseURL+"/api/auth/login", "application/json",
		bytes.NewBufferString(`{"username":"`+username+`","password":"`+roleTestPassword(username)+`"}`))
	if err != nil {
		t.Fatalf("login %s: %v", username, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login %s: expected 200, got %d", username, resp.StatusCode)
	}
	return client
}

// postAs issues a JSON POST with the given client and returns the response.
func postAs(t *testing.T, client *http.Client, url string, body interface{}) *http.Response {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	}
	resp, err := client.Post(url, "application/json", reqBody)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// putAs issues a JSON PUT with the given client and returns the response.
func putAs(t *testing.T, client *http.Client, url string, body interface{}) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", url, err)
	}
	return resp
}

// TestRoleWriteMatrix verifies the role-gated write permissions: viewer is
// blocked from all writes; operator may write within a hub but not global
// resources (hubs/settings/classification); admin may write within a hub but
// not create hubs; super_admin may do everything.
func TestRoleWriteMatrix(t *testing.T) {
	db := openTempDB(t) // seeds super_admin "admin"
	// Synchronous eval execution: the operator-eval trigger below otherwise
	// leaves a detached goroutine whose tail writes (task log, campaign
	// settle, alert hook) race TempDir cleanup even after the run polls done.
	ts := httptest.NewServer(server.New(db,
		server.WithRateLimits(server.RateLimits{}),
		server.WithSessionSecret(testSessionSecret),
		server.WithSyncEval(),
		server.WithSyncDiscovery(),
	))
	t.Cleanup(ts.Close)
	stubHub := newStubHubServer()
	defer stubHub.Close()
	stubHub.SetMode("success")

	hub, err := db.CreateHub("role-hub", stubHub.URL, "test-token-0000")
	if err != nil {
		t.Fatalf("create hub: %v", err)
	}

	seedUserWithRole(t, db, "oper", store.RoleOperator, &hub.ID)
	seedUserWithRole(t, db, "view", store.RoleViewer, &hub.ID)
	seedUserWithRole(t, db, "hubadmin", store.RoleAdmin, &hub.ID)
	seedUserWithRole(t, db, "sa", store.RoleSuperAdmin, nil)

	operClient := loginAsClient(t, ts.URL, "oper")
	viewerClient := loginAsClient(t, ts.URL, "view")
	hubAdminClient := loginAsClient(t, ts.URL, "hubadmin")
	saClient := loginAsClient(t, ts.URL, "sa")

	// Viewer cannot write (hub-internal write): POST /api/models -> 403.
	resp := postAs(t, viewerClient, ts.URL+"/api/models", map[string]interface{}{
		"hub_id": hub.ID, "model_id": "viewer-model",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer model create: expected 403, got %d", resp.StatusCode)
	}

	// Operator can write within hub: POST /api/models -> 201.
	resp = postAs(t, operClient, ts.URL+"/api/models", map[string]interface{}{
		"hub_id": hub.ID, "model_id": "oper-model",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("operator model create: expected 201, got %d", resp.StatusCode)
	}
	var modelEnv envelope
	if err := json.NewDecoder(resp.Body).Decode(&modelEnv); err != nil {
		t.Fatalf("decode oper model: %v", err)
	}
	resp.Body.Close()
	var operModel map[string]interface{}
	if err := json.Unmarshal(modelEnv.Data, &operModel); err != nil {
		t.Fatalf("unmarshal oper model: %v", err)
	}
	operModelID := int64(operModel["id"].(float64))

	// Operator can start an eval (hub-scoped write, not global): POST
	// /api/evals must not be 403. It uses the global case library without
	// writing it, so it is a hub-scoped operation, not super_admin-only.
	instructionSuiteID := suiteIDByKey(t, ts.URL, "cap_instruction")
	evalResp := postAs(t, operClient, ts.URL+"/api/evals", map[string]interface{}{
		"suite_id":  instructionSuiteID,
		"model_ids": []int64{operModelID},
	})
	if evalResp.StatusCode == http.StatusForbidden {
		t.Errorf("operator eval create: must not be 403 (hub-scoped write), got %d", evalResp.StatusCode)
	}
	// Drain the async eval run before the test ends: a goroutine still
	// appending task logs at TempDir cleanup flakes RemoveAll with
	// "directory not empty".
	var evalEnv envelope
	_ = json.NewDecoder(evalResp.Body).Decode(&evalEnv)
	evalResp.Body.Close()
	var evalCampaign map[string]interface{}
	_ = json.Unmarshal(evalEnv.Data, &evalCampaign)
	if runs, ok := evalCampaign["runs"].([]interface{}); ok && len(runs) == 1 {
		runID := int64(runs[0].(map[string]interface{})["id"].(float64))
		waitEvalDone(t, ts.URL, runID)
	}

	// Operator cannot create hubs (super_admin-only): POST /api/hubs -> 403.
	resp = postAs(t, operClient, ts.URL+"/api/hubs", map[string]interface{}{
		"name": "x", "base_url": stubHub.URL, "token": "t",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("operator hub create: expected 403, got %d", resp.StatusCode)
	}

	// Operator cannot update global settings: PUT /api/settings -> 403.
	resp = putAs(t, operClient, ts.URL+"/api/settings", map[string]interface{}{"alert_enabled": true})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("operator settings update: expected 403, got %d", resp.StatusCode)
	}

	// Operator cannot create classification rules: POST /api/classification-rules -> 403.
	resp = postAs(t, operClient, ts.URL+"/api/classification-rules", map[string]interface{}{
		"dimension": "family", "keyword": "kw", "category": "cat",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("operator rule create: expected 403, got %d", resp.StatusCode)
	}

	// Admin (hub-scoped) can write within hub: POST /api/models -> 201.
	resp = postAs(t, hubAdminClient, ts.URL+"/api/models", map[string]interface{}{
		"hub_id": hub.ID, "model_id": "admin-model",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("admin model create: expected 201, got %d", resp.StatusCode)
	}

	// Admin cannot create hubs (super_admin-only): POST /api/hubs -> 403.
	resp = postAs(t, hubAdminClient, ts.URL+"/api/hubs", map[string]interface{}{
		"name": "x2", "base_url": stubHub.URL, "token": "t",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("admin hub create: expected 403, got %d", resp.StatusCode)
	}

	// Super_admin can create hubs: POST /api/hubs -> 201.
	resp = postAs(t, saClient, ts.URL+"/api/hubs", map[string]interface{}{
		"name": "sa-hub", "base_url": stubHub.URL, "token": "t",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("super_admin hub create: expected 201, got %d", resp.StatusCode)
	}

	// Unauthenticated write still returns 401 (not 403).
	resp = anonPost(t, ts.URL+"/api/hubs")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous hub create: expected 401, got %d", resp.StatusCode)
	}
}

// TestAuditActorIsLoginUsername verifies the audit actor is the logged-in
// username, not the legacy hardcoded "admin" constant. A super_admin "alice"
// performs a write; the resulting audit row must carry actor "alice".
func TestAuditActorIsLoginUsername(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stubHub := newStubHubServer()
	defer stubHub.Close()
	stubHub.SetMode("success")

	// Seed a super_admin with a distinctive username (not "admin").
	seedUserWithRole(t, db, "alice", store.RoleSuperAdmin, nil)
	aliceClient := loginAsClient(t, ts.URL, "alice")

	resp := postAs(t, aliceClient, ts.URL+"/api/hubs", map[string]interface{}{
		"name": "alice-hub", "base_url": stubHub.URL, "token": "alice-token",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("alice hub create: expected 201, got %d", resp.StatusCode)
	}

	// Fetch audit logs (any logged-in user may read) and find the hub.create.
	page := fetchAuditLogs(t, ts.URL, "?action=hub.create")
	items := auditItems(t, page)
	if len(items) == 0 {
		t.Fatal("expected a hub.create audit entry")
	}
	for _, it := range items {
		if it["actor"] != "alice" {
			t.Errorf("audit actor: expected alice, got %v", it["actor"])
		}
	}
}

// TestShareLinkCreatedByIsLoginUsername verifies that share_links.created_by
// records the logged-in username (not the legacy "admin" constant), since
// share-link creation passes the actor straight to the store without going
// through s.audit.
func TestShareLinkCreatedByIsLoginUsername(t *testing.T) {
	ts, stub, db := setupEvalEnv(t) // seeds super_admin "admin"
	seedUserWithRole(t, db, "bob", store.RoleSuperAdmin, nil)
	bobClient := loginAsClient(t, ts.URL, "bob")

	modelID := createEvalModel(t, ts.URL, stub.URL, "share-model")
	basicID := suiteIDByKey(t, ts.URL, "cap_instruction")
	runID := triggerEval(t, ts.URL, basicID, modelID)
	run := waitEvalDone(t, ts.URL, runID)
	campaignID := int64(run["campaign_id"].(float64))

	resp := postAs(t, bobClient, fmt.Sprintf("%s/api/campaigns/%d/share-links", ts.URL, campaignID), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create share link: expected 201, got %d", resp.StatusCode)
	}

	// List share-links with bob's session and assert created_by == "bob".
	listResp, err := bobClient.Get(ts.URL + "/api/share-links")
	if err != nil {
		t.Fatalf("GET share links: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list share links: expected 200, got %d", listResp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(listResp.Body).Decode(&env); err != nil {
		t.Fatalf("decode share links: %v", err)
	}
	var links []map[string]interface{}
	if err := json.Unmarshal(env.Data, &links); err != nil {
		t.Fatalf("unmarshal share links: %v", err)
	}
	if len(links) == 0 {
		t.Fatal("expected at least one share link")
	}
	var createdBy interface{}
	for _, l := range links {
		if int64(l["campaign_id"].(float64)) == campaignID {
			createdBy = l["created_by"]
		}
	}
	if createdBy == nil {
		t.Fatal("share link for the campaign not found in list")
	}
	if createdBy != "bob" {
		t.Errorf("share link created_by: expected bob, got %v", createdBy)
	}
}

// TestDisabledUserSessionRejected verifies that a valid session cookie for a
// user whose account has since been disabled is rejected at the auth gate
// (401, same message as an unauthenticated request to avoid probing).
func TestDisabledUserSessionRejected(t *testing.T) {
	db := openTempDB(t)
	ts := newTestAPIServer(t, db)
	stubHub := newStubHubServer()
	defer stubHub.Close()
	stubHub.SetMode("success")

	seedUserWithRole(t, db, "tempuser", store.RoleSuperAdmin, nil)
	client := loginAsClient(t, ts.URL, "tempuser")

	// Session works before disabling: a protected read returns 200.
	resp, err := client.Get(ts.URL + "/api/hubs")
	if err != nil {
		t.Fatalf("GET hubs: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pre-disable GET hubs: expected 200, got %d", resp.StatusCode)
	}

	// Disable the user, then reuse the same cookie: requireSession must 401.
	if err := db.SetUserEnabled("tempuser", false); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	resp, err = client.Get(ts.URL + "/api/hubs")
	if err != nil {
		t.Fatalf("GET hubs after disable: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("disabled user GET hubs: expected 401, got %d", resp.StatusCode)
	}
}
