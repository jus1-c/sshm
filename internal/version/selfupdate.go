package version

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// maxDownloadBytes caps a single downloaded asset to guard against a runaway response.
const maxDownloadBytes = 100 << 20

var downloadClient = &http.Client{Timeout: 5 * time.Minute}

// UpdateResult reports the outcome of a self-update.
type UpdateResult struct {
	Updated     bool
	FromVersion string
	ToVersion   string
	AssetName   string
	Path        string
}

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type releaseInfo struct {
	TagName    string         `json:"tag_name"`
	Prerelease bool           `json:"prerelease"`
	Draft      bool           `json:"draft"`
	Assets     []releaseAsset `json:"assets"`
}

// SelfUpdate downloads the release asset for the current platform, verifies its
// checksum and replaces the running sshm binary in place.
func SelfUpdate(ctx context.Context, currentVersion, targetVersion string, force bool) (*UpdateResult, error) {
	release, err := fetchRelease(ctx, targetVersion)
	if err != nil {
		return nil, err
	}
	if release.Prerelease || release.Draft {
		return nil, fmt.Errorf("release %s is a pre-release or draft; target a stable tag with --version", release.TagName)
	}

	toVersion := release.TagName
	if !force && targetVersion == "" && currentVersion != "" && currentVersion != "dev" &&
		compareVersions(currentVersion, toVersion) >= 0 {
		return &UpdateResult{FromVersion: currentVersion, ToVersion: toVersion}, nil
	}

	assetName, err := assetName(runtime.GOOS, runtime.GOARCH, buildGoarm())
	if err != nil {
		return nil, fmt.Errorf("%w; use the install script instead", err)
	}

	assetURL := ""
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			assetURL = asset.URL
			break
		}
	}
	if assetURL == "" {
		return nil, fmt.Errorf("release %s has no asset %q for %s/%s", toVersion, assetName, runtime.GOOS, runtime.GOARCH)
	}

	data, err := download(ctx, assetURL)
	if err != nil {
		return nil, err
	}
	if err := verifyChecksum(ctx, release, assetName, data); err != nil {
		return nil, err
	}

	binary, err := extractBinary(assetName, data)
	if err != nil {
		return nil, err
	}

	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if err := replaceExecutable(path, binary); err != nil {
		return nil, err
	}

	return &UpdateResult{
		Updated:     true,
		FromVersion: currentVersion,
		ToVersion:   toVersion,
		AssetName:   assetName,
		Path:        path,
	}, nil
}

// assetName mirrors the GoReleaser archive name template for the given platform.
func assetName(goos, goarch, goarm string) (string, error) {
	osName, ok := map[string]string{"linux": "Linux", "darwin": "Darwin", "windows": "Windows"}[goos]
	if !ok {
		return "", fmt.Errorf("unsupported OS %q", goos)
	}

	var archName string
	switch goarch {
	case "amd64":
		archName = "x86_64"
	case "386":
		archName = "i386"
	case "arm64":
		archName = "arm64"
	case "arm":
		if goarm == "" {
			goarm = "7"
		}
		archName = "armv" + goarm
	default:
		return "", fmt.Errorf("unsupported architecture %q", goarch)
	}

	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("sshm_%s_%s%s", osName, archName, ext), nil
}

// buildGoarm reports the GOARM value the binary was built with (empty off ARM).
func buildGoarm() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "GOARM" {
			return setting.Value
		}
	}
	return ""
}

func fetchRelease(ctx context.Context, tag string) (*releaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	if tag != "" {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", repoOwner, repoName, tag)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sshm-updater")

	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound && tag != "" {
		return nil, fmt.Errorf("release %q was not found", tag)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}
	return &release, nil
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sshm-updater")

	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes))
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	return data, nil
}

func verifyChecksum(ctx context.Context, release *releaseInfo, assetName string, data []byte) error {
	checksumURL := ""
	for _, asset := range release.Assets {
		if asset.Name == "checksums.txt" {
			checksumURL = asset.URL
			break
		}
	}
	if checksumURL == "" {
		return errors.New("release has no checksums.txt; refusing to update")
	}

	raw, err := download(ctx, checksumURL)
	if err != nil {
		return err
	}

	expected := ""
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == assetName {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("no checksum found for %q", assetName)
	}

	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
		return fmt.Errorf("checksum mismatch for %s; download may be corrupted", assetName)
	}
	return nil
}

func extractBinary(assetName string, data []byte) ([]byte, error) {
	if strings.HasSuffix(assetName, ".zip") {
		return extractZip(data)
	}
	return extractTarGz(data)
}

func extractTarGz(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid gzip archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid tar archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || !isBinaryName(filepath.Base(header.Name)) {
			continue
		}
		return io.ReadAll(tr)
	}
	return nil, errors.New("sshm binary not found in archive")
}

func extractZip(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip archive: %w", err)
	}
	for _, file := range zr.File {
		if file.FileInfo().IsDir() || !isBinaryName(filepath.Base(file.Name)) {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, errors.New("sshm binary not found in archive")
}

func isBinaryName(name string) bool {
	name = strings.ToLower(name)
	return name == "sshm" || name == "sshm.exe"
}

// replaceExecutable writes binary over path. On Windows the running executable
// cannot be overwritten, so the old file is moved aside first.
func replaceExecutable(path string, binary []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".sshm-update-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s (run with sudo or use the install script): %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0755); err != nil {
		return err
	}

	if err := os.Rename(tmpName, path); err == nil {
		return nil
	}

	backup := path + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Rename(backup, path)
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}
	_ = os.Remove(backup)
	return nil
}
