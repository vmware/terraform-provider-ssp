// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package compat

import "testing"

func TestLoad(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if c.Platform.MinSupported == "" {
		t.Error("Platform.MinSupported should not be empty")
	}
	if c.ProviderVersion == "" {
		t.Error("ProviderVersion should not be empty")
	}
}

func TestForResource_NoConstraintsToday(t *testing.T) {
	// compatibility.yaml ships with an empty attributes list until a real
	// version-sensitive attribute is confirmed and documented there.
	if got := ForResource("ssp_site"); got != nil {
		t.Errorf("ForResource(%q) = %v, want nil (no constraints registered yet)", "ssp_site", got)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
		ok   bool
	}{
		{"equal", "5.2.0", "5.2.0", 0, true},
		{"a older", "5.1.0", "5.2.0", -1, true},
		{"a newer", "5.3.0", "5.2.0", 1, true},
		{"a shorter", "5.1.0", "5.1.0.2", -1, true},
		{"non-numeric segment", "5.x.0", "5.2.0", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := compareVersions(tt.a, tt.b)
			if ok != tt.ok {
				t.Fatalf("compareVersions(%q, %q) ok = %v, want %v", tt.a, tt.b, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	c := AttributeConstraint{Resource: "ssp_site", Attribute: "example", IntroducedIn: "5.3.0"}

	if msg := Evaluate(c, "5.3.0"); msg != "" {
		t.Errorf("Evaluate() at exact version = %q, want empty", msg)
	}
	if msg := Evaluate(c, "5.4.0"); msg != "" {
		t.Errorf("Evaluate() at newer version = %q, want empty", msg)
	}
	if msg := Evaluate(c, "5.2.0"); msg == "" {
		t.Error("Evaluate() at older version should return a warning, got empty")
	}
	if msg := Evaluate(c, "5.3.0.0.0.29219505"); msg != "" {
		t.Errorf("Evaluate() with build-qualified live version = %q, want empty (satisfies 5.3.0 baseline)", msg)
	}
	if msg := Evaluate(c, "not-a-version"); msg != "" {
		t.Errorf("Evaluate() with unparseable version = %q, want empty (advisory only, never hard-fails)", msg)
	}
}
