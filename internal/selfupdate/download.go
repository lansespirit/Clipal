package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func downloadToTempFile(ctx context.Context, client *http.Client, url string, prefix string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if tok := strings.TrimSpace(getToken()); tok != "" {
		// GitHub release asset URLs are public, but keep the token for higher rate limits if used.
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("download %s: %s", url, resp.Status)
	}

	f, err := os.CreateTemp("", prefix)
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer func() {
		_ = f.Close()
	}()

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return tmp, nil
}

func tempSiblingPath(dst string, suffix string) string {
	dir := filepath.Dir(dst)
	base := filepath.Base(dst)
	return filepath.Join(dir, base+suffix)
}
