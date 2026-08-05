package registry

import "testing"

func TestLookupBuiltinExactAndPrefix(t *testing.T) {
	// Exact built-in pattern (none today) would beat prefix; verify the
	// prefix semantics on the shipped table.
	info := Lookup("gpt-4o-mini-2024-07-18", nil)
	if info.IQ == nil || *info.IQ != 7 {
		t.Fatalf("gpt-4o-mini* prefix should give IQ 7, got %+v", info.IQ)
	}
	// Longest prefix wins: gpt-4o-mini* (11 chars) beats gpt-4o* (6).
	if info.PriceOut == nil || *info.PriceOut != 0.6 {
		t.Fatalf("longest prefix should win, got price_out %+v", info.PriceOut)
	}
}

func TestLookupUnknownModelAllNil(t *testing.T) {
	info := Lookup("some-random-finetune-v9", nil)
	if info.IQ != nil || info.PriceIn != nil || info.PriceOut != nil {
		t.Fatalf("unknown model must report all-nil Info, got %+v", info)
	}
}

func TestLookupOverrideFieldMerge(t *testing.T) {
	overrides := []Override{
		{Match: "deepseek-v3*", PriceIn: new(0.5)}, // price correction only
	}
	info := Lookup("deepseek-v3-0324", overrides)
	if info.PriceIn == nil || *info.PriceIn != 0.5 {
		t.Fatalf("override price_in should win, got %+v", info.PriceIn)
	}
	// Fields the override leaves nil keep the built-in values.
	if info.IQ == nil || *info.IQ != 8.5 {
		t.Fatalf("built-in IQ should survive a partial override, got %+v", info.IQ)
	}
	if info.PriceOut == nil || *info.PriceOut != 1.1 {
		t.Fatalf("built-in price_out should survive a partial override, got %+v", info.PriceOut)
	}
}

func TestLookupOverrideRegistersUnknown(t *testing.T) {
	overrides := []Override{
		{Match: "my-local-model", IQ: new(6.0), PriceIn: new(0.0), PriceOut: new(0.0)},
	}
	info := Lookup("my-local-model", overrides)
	if info.IQ == nil || *info.IQ != 6 || info.PriceIn == nil || *info.PriceIn != 0 {
		t.Fatalf("override should register an unknown model, got %+v", info)
	}
}

func TestParseOverrides(t *testing.T) {
	if ovs, err := ParseOverrides(""); err != nil || ovs != nil {
		t.Fatalf("empty raw should yield nil overrides, got %v, %v", ovs, err)
	}
	if _, err := ParseOverrides("{not json"); err == nil {
		t.Fatal("malformed JSON must be an error")
	}
	ovs, err := ParseOverrides(`[{"match":"a*","iq_tier":5}]`)
	if err != nil || len(ovs) != 1 || ovs[0].Match != "a*" || *ovs[0].IQ != 5 {
		t.Fatalf("valid JSON should parse, got %+v, %v", ovs, err)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		ovs  []Override
		ok   bool
	}{
		{"valid", []Override{{Match: "a*", IQ: new(5.0)}}, true},
		{"empty match", []Override{{Match: " ", IQ: new(5.0)}}, false},
		{"iq too high", []Override{{Match: "a", IQ: new(11.0)}}, false},
		{"iq too low", []Override{{Match: "a", IQ: new(0.5)}}, false},
		{"negative price", []Override{{Match: "a", PriceIn: new(-1.0)}}, false},
		{"all fields nil", []Override{{Match: "a"}}, false},
	}
	for _, c := range cases {
		if err := Validate(c.ovs); (err == nil) != c.ok {
			t.Errorf("%s: Validate() err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
