package selfupdate

import (
	"fmt"
	"runtime"
)

const ChecksumsAssetName = "checksums.txt"

func expectedBinaryAssetName() (string, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	switch goos {
	case "darwin":
		switch goarch {
		case "amd64":
			return "clipal-darwin-amd64", nil
		case "arm64":
			return "clipal-darwin-arm64", nil
		}
	case "linux":
		switch goarch {
		case "amd64":
			return "clipal-linux-amd64", nil
		case "arm64":
			return "clipal-linux-arm64", nil
		}
	case "windows":
		switch goarch {
		case "amd64":
			return "clipal-windows-amd64.exe", nil
		case "arm64":
			return "clipal-windows-arm64.exe", nil
		}
	}

	return "", fmt.Errorf("unsupported platform %s/%s", goos, goarch)
}

func findAssetByName(assets []Asset, want string) (*Asset, bool) {
	for i := range assets {
		if assets[i].Name == want {
			return &assets[i], true
		}
	}
	return nil, false
}
