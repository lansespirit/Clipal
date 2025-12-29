package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

func fetchLatestRelease(ctx context.Context, client *http.Client) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", Owner, Repo), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := strings.TrimSpace(getToken()); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("github releases latest: %s", resp.Status)
	}

	var gr githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, err
	}

	r := &Release{TagName: strings.TrimSpace(gr.TagName)}
	for _, a := range gr.Assets {
		r.Assets = append(r.Assets, Asset{
			Name:               a.Name,
			BrowserDownloadURL: a.BrowserDownloadURL,
			Size:               a.Size,
		})
	}
	if r.TagName == "" {
		return nil, fmt.Errorf("github release has empty tag_name")
	}
	return r, nil
}
