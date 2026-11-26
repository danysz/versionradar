package core

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// ShortSHA returns up to 7 characters of sha, falling back to fallback if empty.
func ShortSHA(sha, fallback string) string {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return fallback
	}
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

type semver struct {
	major, minor, patch int
	prerelease          string
	valid               bool
}

var semverRe = regexp.MustCompile(`^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

func parseSemver(s string) semver {
	m := semverRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return semver{}
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	pat, _ := strconv.Atoi(m[3])
	return semver{major: maj, minor: min, patch: pat, prerelease: m[4], valid: true}
}

// CompareSemver compares two version strings. Valid semver ranks above non-semver;
// stable releases rank above prereleases of the same major.minor.patch.
func CompareSemver(a, b string) int {
	sa, sb := parseSemver(a), parseSemver(b)
	if sa.valid != sb.valid {
		if sa.valid {
			return 1
		}
		return -1
	}
	if !sa.valid {
		return strings.Compare(a, b)
	}
	if sa.major != sb.major {
		return cmpInt(sa.major, sb.major)
	}
	if sa.minor != sb.minor {
		return cmpInt(sa.minor, sb.minor)
	}
	if sa.patch != sb.patch {
		return cmpInt(sa.patch, sb.patch)
	}
	if sa.prerelease == sb.prerelease {
		return 0
	}
	if sa.prerelease == "" {
		return 1 // stable > prerelease
	}
	if sb.prerelease == "" {
		return -1
	}
	return strings.Compare(sa.prerelease, sb.prerelease)
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// NormalizeVersion trims whitespace and a leading "v"/"V" when followed by a digit.
func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == 'v' || v[0] == 'V') && v[1] >= '0' && v[1] <= '9' {
		return v[1:]
	}
	return v
}

func isHexSHA(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func shaPrefixMatch(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if !isHexSHA(a) || !isHexSHA(b) {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// VersionsMatch returns true when the deployed version/commit matches the latest version/commit from Git.
func VersionsMatch(deployedVersion, deployedCommit, latestVersion, latestCommit string) bool {
	if latestVersion == "" && latestCommit == "" {
		return false
	}
	if deployedCommit != "" && latestCommit != "" && shaPrefixMatch(deployedCommit, latestCommit) {
		return true
	}
	if deployedVersion != "" && latestCommit != "" && shaPrefixMatch(deployedVersion, latestCommit) {
		return true
	}
	if deployedCommit != "" && latestVersion != "" && shaPrefixMatch(deployedCommit, latestVersion) {
		return true
	}
	dv, lv := NormalizeVersion(deployedVersion), NormalizeVersion(latestVersion)
	if dv == "" || lv == "" {
		return false
	}
	if strings.EqualFold(dv, lv) {
		return true
	}
	// Match "1.2.3 (45)" or "1.2.3+45" against "1.2.3"
	dvBase := stripBuildMeta(dv)
	lvBase := stripBuildMeta(lv)
	return dvBase != "" && strings.EqualFold(dvBase, lvBase)
}

func stripBuildMeta(v string) string {
	if i := strings.IndexAny(v, " +("); i > 0 {
		return strings.TrimSpace(v[:i])
	}
	return v
}

var (
	yamlVersionRe = regexp.MustCompile(`(?m)^\s*version\s*:\s*['"]?([^\s'"#]+)['"]?`)
	tomlVersionRe = regexp.MustCompile(`(?m)^\s*version\s*=\s*['"]([^'"]+)['"]`)
)

// ExtractVersionFromFile parses a version string from common manifest files or a custom regex.
func ExtractVersionFromFile(filePath string, content []byte, customRegex string) (string, error) {
	text := string(content)
	if strings.TrimSpace(customRegex) != "" {
		re, err := regexp.Compile(customRegex)
		if err != nil {
			return "", err
		}
		m := re.FindStringSubmatch(text)
		if len(m) == 0 {
			return "", errors.New("regex did not match file content")
		}
		if len(m) >= 2 && strings.TrimSpace(m[1]) != "" {
			return strings.TrimSpace(m[1]), nil
		}
		return strings.TrimSpace(m[0]), nil
	}

	lower := strings.ToLower(strings.TrimSpace(filePath))
	switch {
	case strings.HasSuffix(lower, ".json"):
		var obj map[string]any
		if err := json.Unmarshal(content, &obj); err != nil {
			return "", err
		}
		if v, ok := obj["version"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), nil
		}
		return "", errors.New(`no string "version" field in JSON`)
	case strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml"):
		if m := yamlVersionRe.FindStringSubmatch(text); len(m) >= 2 {
			return strings.TrimSpace(m[1]), nil
		}
		return "", errors.New("no version: line found in YAML")
	case strings.HasSuffix(lower, ".toml"):
		if m := tomlVersionRe.FindStringSubmatch(text); len(m) >= 2 {
			return strings.TrimSpace(m[1]), nil
		}
		return "", errors.New("no version = line found in TOML")
	default:
		// Plain version file (e.g. VERSION): first non-empty, non-comment line.
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				return line, nil
			}
		}
		return "", errors.New("version file is empty")
	}
}
