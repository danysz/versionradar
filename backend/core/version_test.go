package core

import "testing"

func TestCompareSemver(t *testing.T) {
	if CompareSemver("v1.10.0", "v1.9.5") <= 0 {
		t.Fatal("expected v1.10.0 > v1.9.5")
	}
	if CompareSemver("1.2.0", "1.2.0-rc.1") <= 0 {
		t.Fatal("expected stable > prerelease")
	}
}

func TestVersionsMatch(t *testing.T) {
	cases := []struct {
		dv, dc, lv, lc string
		want           bool
	}{
		{"v1.2.3", "", "1.2.3", "", true},
		{"1.2.3 (45)", "", "v1.2.3", "", true},
		{"1.2.3+45", "", "1.2.3", "", true},
		{"abcdef1", "", "v1.4.0", "abcdef1234567890abcdef1234567890abcdef12", true},
		{"v42", "abcdef123456", "abcdef1", "abcdef1234567890abcdef1234567890abcdef12", true},
		{"v1.2.0", "", "v1.3.0", "", false},
	}
	for _, tc := range cases {
		if got := VersionsMatch(tc.dv, tc.dc, tc.lv, tc.lc); got != tc.want {
			t.Errorf("VersionsMatch(%q,%q,%q,%q) = %v, want %v", tc.dv, tc.dc, tc.lv, tc.lc, got, tc.want)
		}
	}
}

func TestExtractVersionFromFile(t *testing.T) {
	if v, err := ExtractVersionFromFile("package.json", []byte(`{"name":"x","version":"3.4.5"}`), ""); err != nil || v != "3.4.5" {
		t.Fatalf("package.json: %q %v", v, err)
	}
	if v, err := ExtractVersionFromFile("pubspec.yaml", []byte("name: vr\nversion: 1.2.0+7\n"), ""); err != nil || v != "1.2.0+7" {
		t.Fatalf("pubspec.yaml: %q %v", v, err)
	}
	if v, err := ExtractVersionFromFile("Cargo.toml", []byte("[package]\nversion = \"0.9.1\"\n"), ""); err != nil || v != "0.9.1" {
		t.Fatalf("Cargo.toml: %q %v", v, err)
	}
	if v, err := ExtractVersionFromFile("VERSION", []byte("# comment\n2.0.0\n"), ""); err != nil || v != "2.0.0" {
		t.Fatalf("VERSION: %q %v", v, err)
	}
	if v, err := ExtractVersionFromFile("build.gradle", []byte(`versionName "4.5.6"`), `versionName\s+"([^"]+)"`); err != nil || v != "4.5.6" {
		t.Fatalf("custom regex: %q %v", v, err)
	}
}
