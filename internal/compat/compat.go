// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

// Package compat is the single source of truth for which resource
// attributes require a minimum SSP platform version, read from the
// embedded compatibility.yaml. docs/guides/version-compatibility.md and
// the README's compatibility section are generated from the same file via
// scripts/gen-compat-docs.py -- edit compatibility.yaml, not the generated
// docs, and regenerate with `make docs-compat`.
package compat

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed compatibility.yaml
var compatibilityYAML []byte

// Platform describes the platform-version baseline this provider release
// was built and tested against.
type Platform struct {
	MinSupported  string `yaml:"min_supported"`
	TestedAgainst string `yaml:"tested_against"`
}

// AttributeConstraint records that a specific resource attribute requires
// at least the given platform version.
type AttributeConstraint struct {
	Resource     string `yaml:"resource"`
	Attribute    string `yaml:"attribute"`
	IntroducedIn string `yaml:"introduced_in"`
	Description  string `yaml:"description,omitempty"`
}

// Compatibility is the parsed contents of compatibility.yaml.
type Compatibility struct {
	ProviderVersion string                `yaml:"provider_version"`
	Platform        Platform              `yaml:"platform"`
	Attributes      []AttributeConstraint `yaml:"attributes"`
}

var (
	once    sync.Once
	loaded  *Compatibility
	loadErr error
)

// Load parses the embedded compatibility.yaml, caching the result.
func Load() (*Compatibility, error) {
	once.Do(func() {
		var c Compatibility
		if err := yaml.Unmarshal(compatibilityYAML, &c); err != nil {
			loadErr = fmt.Errorf("parsing embedded compatibility.yaml: %w", err)
			return
		}
		loaded = &c
	})
	return loaded, loadErr
}

// ForResource returns the version constraints registered for the given
// resource type name (e.g. "ssp_site"), or nil if none are registered.
// Callers should treat a nil/empty result as "nothing to check" and skip
// fetching the live platform version entirely -- compatibility.yaml ships
// with an empty attribute list today, so this is a no-op everywhere until
// a real version-sensitive attribute is documented there.
func ForResource(resourceType string) []AttributeConstraint {
	c, err := Load()
	if err != nil || c == nil {
		return nil
	}
	var out []AttributeConstraint
	for _, a := range c.Attributes {
		if a.Resource == resourceType {
			out = append(out, a)
		}
	}
	return out
}

// Evaluate returns a human-readable warning if livePlatformVersion is
// older than the constraint's IntroducedIn version, or "" if the
// constraint is satisfied or the versions can't be compared (e.g. a
// non-numeric build string). This check is advisory only, never a hard
// failure.
func Evaluate(c AttributeConstraint, livePlatformVersion string) string {
	cmp, ok := compareVersions(livePlatformVersion, c.IntroducedIn)
	if !ok || cmp >= 0 {
		return ""
	}
	return fmt.Sprintf(
		"%s.%s requires platform version %s or newer, but the configured platform reports %s.",
		c.Resource, c.Attribute, c.IntroducedIn, livePlatformVersion,
	)
}

// compareVersions compares dotted numeric version strings (e.g. "5.2.0" or
// the longer build-qualified form "5.2.0.0.0.29219505"), returning
// -1/0/1 like strings.Compare, and ok=false if either string has a
// non-numeric segment.
func compareVersions(a, b string) (int, bool) {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		var err error
		if i < len(as) {
			if av, err = strconv.Atoi(as[i]); err != nil {
				return 0, false
			}
		}
		if i < len(bs) {
			if bv, err = strconv.Atoi(bs[i]); err != nil {
				return 0, false
			}
		}
		if av != bv {
			if av < bv {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}
