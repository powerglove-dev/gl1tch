package pluginmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const githubRawBase = "https://raw.githubusercontent.com"
const githubAPIBase = "https://api.github.com"

// FetchManifest downloads glitch-plugin.yaml from a GitHub repo at the given ref.
// If ref.Version is empty it tries the default branch (main, then master).
func FetchManifest(ctx context.Context, ref PluginRef) (*PluginManifest, error) {
	branches := []string{ref.Version}
	if ref.Version == "" {
		branches = []string{"main", "master"}
	}

	var lastErr error
	for _, branch := range branches {
		u := fmt.Sprintf("%s/%s/%s/%s/%s",
			githubRawBase, ref.Owner, ref.Repo, branch, ManifestFileName)
		data, err := httpGet(ctx, u)
		if err != nil {
			lastErr = err
			continue
		}
		m, err := ParseManifest(data)
		if err != nil {
			return nil, err
		}
		m.source = ref.Owner + "/" + ref.Repo
		if ref.Version != "" && m.Version == "" {
			m.Version = ref.Version
		}
		return m, nil
	}
	return nil, fmt.Errorf("fetch manifest for %s: %w", ref, lastErr)
}

// InstallBinary installs the plugin binary according to the manifest's install block.
// For InstallGo it runs `go install`; for InstallRelease it downloads from GitHub Releases.
// Returns the path to the installed binary.
func InstallBinary(ctx context.Context, ref PluginRef, m *PluginManifest) (string, error) {
	switch m.Install.method() {
	case InstallGo:
		return installViaGo(ctx, m)
	case InstallRelease:
		return installViaRelease(ctx, ref, m)
	default:
		return "", fmt.Errorf("install binary: unknown method for plugin %q", m.Name)
	}
}

func installViaGo(ctx context.Context, m *PluginManifest) (string, error) {
	module := m.Install.Go
	if m.Version != "" && !strings.Contains(module, "@") {
		module = module + "@" + m.Version
	} else if !strings.Contains(module, "@") {
		module = module + "@latest"
	}

	// Derive a GOPRIVATE/GONOSUMDB pattern from the module host so that
	// private GitHub repos (and other private hosts) can be installed without
	// hitting the public checksum database.
	host := moduleHost(module)
	out, err := runCommandEnv(ctx, []string{
		"GOPRIVATE=" + host,
		"GONOSUMDB=" + host,
		"GONOSUMCHECK=" + host,
	}, "go", "install", module)
	if err != nil {
		return "", fmt.Errorf("go install %s: %w\n%s", module, err, out)
	}

	// Resolve the binary path from GOPATH/bin or GOBIN.
	binPath, err := resolveBinaryPath(m.Binary)
	if err != nil {
		return "", fmt.Errorf("go install %s succeeded but binary %q not found: %w", module, m.Binary, err)
	}
	return binPath, nil
}

// githubRelease is a minimal struct for decoding a GitHub releases API response.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func installViaRelease(ctx context.Context, ref PluginRef, m *PluginManifest) (string, error) {
	tag := m.Version
	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases", githubAPIBase, ref.Owner, ref.Repo)
	if tag != "" {
		apiURL = fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", githubAPIBase, ref.Owner, ref.Repo, tag)
	} else {
		apiURL = fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPIBase, ref.Owner, ref.Repo)
	}

	data, err := httpGet(ctx, apiURL)
	if err != nil {
		return "", fmt.Errorf("fetch release for %s: %w", ref, err)
	}

	var release githubRelease
	if err := json.Unmarshal(data, &release); err != nil {
		return "", fmt.Errorf("parse release JSON for %s: %w", ref, err)
	}

	// Find an asset matching <binary>_<GOOS>_<GOARCH> (case-insensitive prefix match).
	wantPrefix := strings.ToLower(fmt.Sprintf("%s_%s_%s", m.Binary, runtime.GOOS, runtime.GOARCH))
	var downloadURL string
	for _, asset := range release.Assets {
		if strings.HasPrefix(strings.ToLower(asset.Name), wantPrefix) {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return "", fmt.Errorf("no release asset matching %q for %s/%s", wantPrefix, runtime.GOOS, runtime.GOARCH)
	}

	// Download to a temp file, then move to ~/.local/bin.
	binDir, err := localBinDir()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(binDir, m.Binary)

	if err := downloadFile(ctx, downloadURL, dest); err != nil {
		return "", fmt.Errorf("download release asset: %w", err)
	}
	if err := os.Chmod(dest, 0o755); err != nil {
		return "", fmt.Errorf("chmod release binary: %w", err)
	}
	return dest, nil
}

// httpGet performs a GET request and returns the response body.
// Attaches a GitHub token if GH_TOKEN or GITHUB_TOKEN is set, or if `gh auth token` succeeds.
func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := githubToken(ctx); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// githubToken returns an auth token for GitHub API requests, checking env vars
// first then falling back to `gh auth token`.
func githubToken(ctx context.Context) string {
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if t := os.Getenv(k); t != "" {
			return t
		}
	}
	out, err := runCommand(ctx, "gh", "auth", "token")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// downloadFile streams a URL to dest atomically via a temp file.
func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, url)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".dl-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // clean up on error; no-op after rename

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}
