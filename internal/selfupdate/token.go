package selfupdate

import "os"

func getToken() string {
	// Optional; increases GitHub API rate limit.
	if v := os.Getenv("GITHUB_TOKEN"); v != "" {
		return v
	}
	return os.Getenv("GH_TOKEN")
}
