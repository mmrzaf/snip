package selector

import (
	"strings"
	"testing"

	"github.com/mmrzaf/snip/internal/config"
	"github.com/mmrzaf/snip/internal/discovery"
)

func TestParseModifiers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		wantErr bool
		want    []Modifier
	}{
		{"empty", nil, false, nil},
		{"enable", []string{"+tests"}, false, []Modifier{{Name: "tests", Enable: true}}},
		{"disable", []string{"-docs"}, false, []Modifier{{Name: "docs", Enable: false}}},
		{"badprefix", []string{"tests"}, true, nil},
		{"badname", []string{"+a/b"}, true, nil},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mods, err := ParseModifiers(tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if len(mods) != len(tc.want) {
				t.Fatalf("len(mods)=%d want=%d", len(mods), len(tc.want))
			}
			for i := range mods {
				if mods[i] != tc.want[i] {
					t.Fatalf("mods[%d]=%+v want=%+v", i, mods[i], tc.want[i])
				}
			}
		})
	}
}

func TestEnabledSlices(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.DefaultProfile = "p"
	cfg.Slices = map[string]config.SliceConfig{
		"api":   {Include: []string{"**/*.go"}, Priority: 100},
		"tests": {Include: []string{"**/*_test.go"}, Priority: 10},
		"docs":  {Include: []string{"docs/**"}, Priority: 1},
	}
	cfg.Profiles = map[string]config.Profile{
		"p": {Enable: []string{"api", "docs"}},
	}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}

	mods, err := ParseModifiers([]string{"+tests", "-docs"})
	if err != nil {
		t.Fatalf("ParseModifiers: %v", err)
	}
	en, err := EnabledSlices(cfg, "p", mods)
	if err != nil {
		t.Fatalf("EnabledSlices: %v", err)
	}
	// Sorted.
	want := []string{"api", "tests"}
	if len(en) != len(want) {
		t.Fatalf("len=%d want=%d", len(en), len(want))
	}
	for i := range en {
		if en[i] != want[i] {
			t.Fatalf("en[%d]=%s want=%s", i, en[i], want[i])
		}
	}
}

func TestEnabledSlicesUnknownProfileListsAvailable(t *testing.T) {
	t.Parallel()

	cfg := selectorErrorFixture()
	_, err := EnabledSlices(cfg, "foo", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	msg := err.Error()
	for _, want := range []string{`unknown profile "foo"`, "available: api, debug, full, minimal"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

func TestEnabledSlicesUnknownModifierSliceListsAvailable(t *testing.T) {
	t.Parallel()

	cfg := selectorErrorFixture()
	_, err := EnabledSlices(cfg, "api", []Modifier{{Name: "bar", Enable: true}})
	if err == nil {
		t.Fatalf("expected error")
	}
	msg := err.Error()
	for _, want := range []string{`unknown slice "bar"`, "available: code, configs, docs, infra, tests"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

func TestEnabledSlicesProfileUnknownSliceStaysClear(t *testing.T) {
	t.Parallel()

	cfg := selectorErrorFixture()
	cfg.Profiles["api"] = config.Profile{Enable: []string{"missing"}}
	_, err := EnabledSlices(cfg, "api", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	msg := err.Error()
	for _, want := range []string{`profile "api" enables unknown slice "missing"`, "available: code, configs, docs, infra, tests"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

func selectorErrorFixture() config.Config {
	cfg := config.Default()
	cfg.DefaultProfile = "api"
	cfg.Slices = map[string]config.SliceConfig{
		"code":    {Include: []string{"**/*.go"}, Priority: 100},
		"configs": {Include: []string{"**/*.yaml"}, Priority: 15},
		"docs":    {Include: []string{"README.md"}, Priority: 20},
		"infra":   {Include: []string{".github/**"}, Priority: 10},
		"tests":   {Include: []string{"**/*_test.go"}, Priority: 40},
	}
	cfg.Profiles = map[string]config.Profile{
		"api":     {Enable: []string{"code", "docs"}},
		"debug":   {Enable: []string{"code", "docs", "tests"}},
		"full":    {Enable: []string{"code", "configs", "docs", "infra", "tests"}},
		"minimal": {Enable: []string{"code"}},
	}
	return cfg
}

func TestSelectPrimaryAndHiddenRules(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.DefaultProfile = "p"
	cfg.Slices = map[string]config.SliceConfig{
		"all": {Include: []string{"**/*"}, Priority: 1},
		"dot": {Include: []string{".github/**"}, Priority: 10},
	}
	cfg.Profiles = map[string]config.Profile{"p": {Enable: []string{"all", "dot"}}}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}

	discovered := []discovery.PathInfo{
		{RelPath: "README.md", AbsPath: "/tmp/README.md", IsHidden: false},
		{RelPath: ".github/workflows/ci.yml", AbsPath: "/tmp/.github/workflows/ci.yml", IsHidden: true},
	}

	// Without --include-hidden: only explicit dot-pattern should match hidden file.
	selected, err := Select(cfg, []string{"all", "dot"}, discovered, false)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(selected.Included) != 2 {
		t.Fatalf("included=%d want=2", len(selected.Included))
	}
	var hidden File
	for _, f := range selected.Included {
		if f.IsHidden {
			hidden = f
		}
	}
	if hidden.RelPath == "" {
		t.Fatalf("expected hidden file")
	}
	if hidden.PrimarySlice != "dot" {
		t.Fatalf("primary=%s want dot", hidden.PrimarySlice)
	}
	if len(hidden.Slices) != 1 || hidden.Slices[0] != "dot" {
		t.Fatalf("slices=%v want [dot]", hidden.Slices)
	}

	// With --include-hidden: broad patterns are allowed to match hidden.
	selected2, err := Select(cfg, []string{"all", "dot"}, discovered, true)
	if err != nil {
		t.Fatalf("Select(includeHidden): %v", err)
	}
	var hidden2 File
	for _, f := range selected2.Included {
		if f.IsHidden {
			hidden2 = f
		}
	}
	if len(hidden2.Slices) != 2 {
		t.Fatalf("slices=%v want both", hidden2.Slices)
	}
}
