package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Plan struct {
	CurrentVersion string
	LatestVersion  string
	BinaryAsset    Asset
	ChecksumsAsset Asset
	ExecutablePath string
}

func BuildPlan(ctx context.Context, client *http.Client, currentVersion string) (*Plan, error) {
	rel, err := fetchLatestRelease(ctx, client)
	if err != nil {
		return nil, err
	}

	binName, err := expectedBinaryAssetName()
	if err != nil {
		return nil, err
	}
	bin, ok := findAssetByName(rel.Assets, binName)
	if !ok {
		return nil, fmt.Errorf("release %s missing asset %q", rel.TagName, binName)
	}
	chk, ok := findAssetByName(rel.Assets, ChecksumsAssetName)
	if !ok {
		return nil, fmt.Errorf("release %s missing asset %q", rel.TagName, ChecksumsAssetName)
	}

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return nil, err
	}

	return &Plan{
		CurrentVersion: strings.TrimSpace(currentVersion),
		LatestVersion:  strings.TrimSpace(rel.TagName),
		BinaryAsset:    *bin,
		ChecksumsAsset: *chk,
		ExecutablePath: exe,
	}, nil
}

func NeedsUpdate(currentVersion string, latestTag string) (needs bool, comparable bool) {
	cur := strings.TrimSpace(currentVersion)
	lat := strings.TrimSpace(latestTag)
	if cur == "" {
		return true, false
	}
	if cur == "dev" {
		return true, false
	}
	cmp, ok := compareVersions(cur, lat)
	if !ok {
		return true, false
	}
	return cmp < 0, true
}

func Update(ctx context.Context, currentVersion string, opts Options) (*Plan, bool, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	client := &http.Client{Timeout: opts.Timeout}

	plan, err := BuildPlan(ctx, client, currentVersion)
	if err != nil {
		return nil, false, err
	}

	needs, comparable := NeedsUpdate(plan.CurrentVersion, plan.LatestVersion)
	if !needs && !opts.Force {
		return plan, false, nil
	}
	if comparable {
		if cmp, ok := compareVersions(plan.CurrentVersion, plan.LatestVersion); ok && cmp > 0 && !opts.Force {
			return plan, false, fmt.Errorf("current version %s is newer than latest %s (use --force to downgrade)", plan.CurrentVersion, plan.LatestVersion)
		}
	}

	if opts.Check || opts.DryRun {
		return plan, needs, nil
	}

	checksumsPath, err := downloadToTempFile(ctx, client, plan.ChecksumsAsset.BrowserDownloadURL, "clipal-checksums-")
	if err != nil {
		return plan, false, err
	}
	defer os.Remove(checksumsPath)

	checksumsBytes, err := os.ReadFile(checksumsPath)
	if err != nil {
		return plan, false, err
	}
	sumMap, err := parseChecksums(checksumsBytes)
	if err != nil {
		return plan, false, err
	}

	wantSum, ok := sumMap[plan.BinaryAsset.Name]
	if !ok {
		return plan, false, fmt.Errorf("checksums.txt missing entry for %q", plan.BinaryAsset.Name)
	}

	newBinPath, err := downloadToTempFile(ctx, client, plan.BinaryAsset.BrowserDownloadURL, "clipal-bin-")
	if err != nil {
		return plan, false, err
	}
	cleanupNew := runtime.GOOS != "windows"
	defer func() {
		if cleanupNew {
			_ = os.Remove(newBinPath)
		}
	}()

	gotSum, err := sha256File(newBinPath)
	if err != nil {
		return plan, false, err
	}
	if !strings.EqualFold(gotSum, wantSum) {
		return plan, false, fmt.Errorf("sha256 mismatch for %s: got %s want %s", plan.BinaryAsset.Name, gotSum, wantSum)
	}

	switch runtime.GOOS {
	case "windows":
		if err := applyWindows(plan.ExecutablePath, newBinPath, opts.Relaunch); err != nil {
			return plan, false, err
		}
	default:
		if err := applyUnix(plan.ExecutablePath, newBinPath); err != nil {
			return plan, false, err
		}
	}

	return plan, true, nil
}

func copyFile(dst string, src string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return out.Close()
}
