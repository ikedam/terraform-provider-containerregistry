package buildx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	// See: https://github.com/docker/buildx/releases
	buildxRepo              = "docker/buildx"
	githubReleasesBase      = "https://github.com/" + buildxRepo + "/releases"
	githubReleasesLatestURL = githubReleasesBase + "/latest"
	pluginName              = "docker-buildx"
	envDockerConfig         = "DOCKER_CONFIG"
	checksumsFilename       = "checksums.txt"
)

// installMu serializes EnsureInstalled so that concurrent resources do not run install in parallel.
var installMu sync.Mutex

// PluginDir returns the directory where the Docker CLI looks for plugins (cli-plugins under config dir).
// It uses DOCKER_CONFIG if set, otherwise $HOME/.docker.
func PluginDir() string {
	configDir := os.Getenv(envDockerConfig)
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".docker")
	}
	return filepath.Join(configDir, "cli-plugins")
}

// InstalledPath returns the path to the buildx plugin binary if it were in the plugin dir.
func InstalledPath() string {
	name := pluginName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(PluginDir(), name)
}

// IsInstalled returns true if the buildx plugin binary exists and is executable in the plugin dir.
func IsInstalled() bool {
	path := InstalledPath()
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && (info.Mode()&0111) != 0
}

// EnsureInstalled installs the buildx plugin when not present. version is the release tag (e.g. "v0.12.0"); empty means latest.
// Uses direct asset downloads only (no GitHub API). For latest, resolves version from the releases/latest redirect, then downloads checksums.txt to verify the binary.
// Safe to call from multiple goroutines; install is serialized with a mutex so only one install runs at a time.
// If client is non-nil, it is used for all HTTP requests (e.g. a logging client for traceability); otherwise http.DefaultClient is used.
func EnsureInstalled(ctx context.Context, version string, client *http.Client) error {
	if IsInstalled() {
		return nil
	}

	installMu.Lock()
	defer installMu.Unlock()

	// Double-check after acquiring lock (another goroutine may have just installed)
	if IsInstalled() {
		return nil
	}

	tflog.Info(ctx, "Installing buildx plugin", map[string]interface{}{
		"version": version,
	})

	if version == "" || version == "latest" {
		tflog.Info(ctx, "Resolving latest buildx version")
		var err error
		version, err = resolveLatestVersion(ctx, client)
		if err != nil {
			return fmt.Errorf("failed to resolve latest version: %w", err)
		}
		tflog.Info(ctx, "Resolved latest buildx version", map[string]interface{}{
			"version": version,
		})
	} else {
		if !strings.HasPrefix(version, "v") {
			version = "v" + version
		}
	}

	assetName := assetNameForCurrentPlatform(version)
	if assetName == "" {
		return fmt.Errorf("buildx does not provide a binary for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	baseURL := githubReleasesBase + "/download/" + version + "/"

	// Download checksums.txt and parse expected hash for our asset
	tflog.Info(ctx, "Downloading checksums.txt", map[string]interface{}{
		"url": baseURL + checksumsFilename,
	})
	checksums, err := downloadChecksums(ctx, baseURL+checksumsFilename, client)
	if err != nil {
		return fmt.Errorf("failed to download checksums: %w", err)
	}
	expectedHash, ok := checksums[assetName]
	if !ok {
		return fmt.Errorf("asset %q not found in checksums.txt", assetName)
	}

	pluginDir := PluginDir()
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugin dir: %w", err)
	}
	destPath := InstalledPath()

	if err := downloadAndVerifyFile(ctx, baseURL+assetName, destPath, expectedHash, client); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(destPath, 0755); err != nil {
			_ = os.Remove(destPath)
			return fmt.Errorf("failed to chmod plugin: %w", err)
		}
	}

	return nil
}

// httpClient returns the client to use for requests; if c is nil, returns http.DefaultClient.
func httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return http.DefaultClient
}

// resolveLatestVersion performs a GET to .../releases/latest and returns the tag from the redirect Location (e.g. "v0.32.1"). No API used.
func resolveLatestVersion(ctx context.Context, client *http.Client) (string, error) {
	transport := http.DefaultTransport
	if client != nil && client.Transport != nil {
		transport = client.Transport
	}
	c := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesLatestURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") == "" {
		return "", fmt.Errorf("expected redirect from %s, got %s", githubReleasesLatestURL, resp.Status)
	}
	location := resp.Header.Get("Location")
	// Location is like https://github.com/docker/buildx/releases/tag/v0.32.1
	re := regexp.MustCompile(`/releases/tag/(v[^/]+)$`)
	matches := re.FindStringSubmatch(location)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not parse version from redirect %q", location)
	}
	return matches[1], nil
}

// downloadChecksums fetches checksums.txt and returns a map of filename -> hex-encoded SHA256.
// Lines are expected in the form "hash  filename" or "hash *filename" (checksum format).
func downloadChecksums(ctx context.Context, url string, client *http.Client) (map[string]string, error) {
	body, err := downloadBytes(ctx, url, client)
	if err != nil {
		return nil, err
	}
	return parseChecksums(body)
}

// parseChecksums parses checksums.txt content. Supports "SHA256(filename)= hash" and "hash  filename" / "hash *filename".
var (
	reChecksumSpace = regexp.MustCompile(`^([a-fA-F0-9]{64})\s+[\* ]\s*(.+)$`)
	reChecksumEq    = regexp.MustCompile(`^SHA256\s*\(\s*(.+?)\s*\)\s*=\s*([a-fA-F0-9]{64})\s*$`)
)

func parseChecksums(data []byte) (map[string]string, error) {
	out := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := reChecksumSpace.FindStringSubmatch(line); len(m) == 3 {
			out[strings.TrimSpace(m[2])] = strings.ToLower(m[1])
			continue
		}
		if m := reChecksumEq.FindStringSubmatch(line); len(m) == 3 {
			out[strings.TrimSpace(m[1])] = strings.ToLower(m[2])
			continue
		}
	}
	return out, nil
}

func downloadBytes(ctx context.Context, url string, client *http.Client) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient(client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func downloadAndVerifyFile(ctx context.Context, url, destPath, expectedHashHex string, client *http.Client) error {
	tflog.Info(ctx, "Downloading file", map[string]interface{}{
		"url":               url,
		"dest_path":         destPath,
		"expected_hash_hex": expectedHashHex,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient(client).Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: %s", resp.Status)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("write file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(destPath)
		return err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(expectedHashHex) {
		_ = os.Remove(destPath)
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", filepath.Base(destPath), expectedHashHex, got)
	}
	return nil
}

// assetNameForCurrentPlatform returns the buildx asset filename for the current GOOS/GOARCH.
// Buildx uses names like buildx-v0.32.1.linux-amd64, buildx-v0.32.1.windows-amd64.exe.
func assetNameForCurrentPlatform(version string) string {
	version = strings.TrimPrefix(version, "v")
	base := "buildx-v" + version + "."
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}

	arch := runtime.GOARCH
	switch runtime.GOOS {
	case "darwin":
		switch arch {
		case "amd64", "arm64":
			return base + "darwin-" + arch
		}
	case "linux":
		switch arch {
		case "amd64", "arm64", "ppc64le", "riscv64", "s390x":
			return base + "linux-" + arch
		case "arm":
			return base + "linux-arm-v7"
		}
	case "windows":
		switch arch {
		case "amd64", "arm64":
			return base + "windows-" + arch + suffix
		}
	case "freebsd", "netbsd", "openbsd":
		switch arch {
		case "amd64", "arm64":
			return base + runtime.GOOS + "-" + arch
		}
	}
	return ""
}
