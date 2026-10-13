package main

import "testing"

func TestPickEngine(t *testing.T) {
	cases := []struct {
		name    string
		cfg     trConfig
		want    string
		wantErr bool
	}{
		{"auto without key", trConfig{Engine: "auto"}, "mymemory", false},
		{"empty engine without key", trConfig{}, "mymemory", false},
		{"auto with key", trConfig{Engine: "auto", DeepLKey: "k:fx"}, "deepl", false},
		{"deepl with key", trConfig{Engine: "deepl", DeepLKey: "k"}, "deepl", false},
		{"deepl without key", trConfig{Engine: "deepl"}, "", true},
		{"mymemory ignores key", trConfig{Engine: "mymemory", DeepLKey: "k"}, "mymemory", false},
		{"unknown engine", trConfig{Engine: "bogus"}, "", true},
	}
	for _, c := range cases {
		got, err := pickEngine(c.cfg)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("%s: got (%q, %v), want (%q, err=%v)", c.name, got, err, c.want, c.wantErr)
		}
	}
}

func TestResolveFallsBackToEnv(t *testing.T) {
	t.Setenv("GITCORD_DEEPL_KEY", "envkey:fx")
	t.Setenv("GITCORD_MYMEMORY_EMAIL", "env@example.com")
	got := trConfig{DeepLKey: "  "}.resolve()
	if got.DeepLKey != "envkey:fx" || got.MyMemoryEmail != "env@example.com" {
		t.Errorf("env fallback failed: %+v", got)
	}
	// Values from the settings dialog win over the environment
	got = trConfig{DeepLKey: " ui:fx ", MyMemoryEmail: " ui@example.com "}.resolve()
	if got.DeepLKey != "ui:fx" || got.MyMemoryEmail != "ui@example.com" {
		t.Errorf("settings should override env: %+v", got)
	}
}

func TestDeeplEndpoint(t *testing.T) {
	if got := deeplEndpoint("abc:fx"); got != "https://api-free.deepl.com/v2/translate" {
		t.Errorf("free key endpoint = %s", got)
	}
	if got := deeplEndpoint("abc"); got != "https://api.deepl.com/v2/translate" {
		t.Errorf("pro key endpoint = %s", got)
	}
}
