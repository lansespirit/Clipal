package selfupdate

import "time"

const (
	Owner = "lansespirit"
	Repo  = "Clipal"
)

type Options struct {
	Check    bool
	Force    bool
	DryRun   bool
	Timeout  time.Duration
	Relaunch bool // Windows: relaunch the updated binary after replacing.
}

type Release struct {
	TagName string
	Assets  []Asset
}

type Asset struct {
	Name               string
	BrowserDownloadURL string
	Size               int64
}
