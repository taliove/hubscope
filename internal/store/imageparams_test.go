package store

import (
	"strings"
	"testing"
)

// TestImageParamsForCorruptRow pins the error path behind the GH #44
// degradation branches: a rule row whose params JSON is unreadable (e.g. a
// hand-edited database) must surface as an error from the single resolution
// entry — never as a silently half-applied rule set — so the callers
// (prober, discovery, server) can degrade to the minimal request body. This
// in-package leaf test mirrors the users_test.go pattern; the black-box
// behavior test lives in internal/server/image_params_degrade_test.go.
func TestImageParamsForCorruptRow(t *testing.T) {
	db := openTestDB(t)

	// A fresh database carries the seeded gpt-image rule and resolves fine.
	params, err := db.ImageParamsFor("gpt-image-1")
	if err != nil {
		t.Fatalf("fresh db: unexpected error: %v", err)
	}
	if params["quality"] != "low" {
		t.Fatalf("fresh db: expected seeded quality=low, got %v", params)
	}

	// Corrupt the seeded row directly (in-package access; the external
	// black-box tests use ExecRawForTest for the same setup).
	if _, err := db.conn.Exec(
		"UPDATE image_param_rules SET params = ? WHERE keyword = ?",
		"{corrupt-json", "gpt-image",
	); err != nil {
		t.Fatalf("corrupt rule row: %v", err)
	}

	checks := map[string]func() error{
		"ListImageParamRules": func() error { _, err := db.ListImageParamRules(); return err },
		"ImageParamsFor":      func() error { _, err := db.ImageParamsFor("gpt-image-1"); return err },
	}
	for name, fn := range checks {
		err := fn()
		if err == nil {
			t.Errorf("%s: expected a corrupt params JSON error, got nil", name)
			continue
		}
		if !strings.Contains(err.Error(), "corrupt params JSON") {
			t.Errorf("%s: expected 'corrupt params JSON' in error, got %v", name, err)
		}
	}
}
