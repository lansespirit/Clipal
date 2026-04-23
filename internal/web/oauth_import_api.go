package web

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/lansespirit/Clipal/internal/config"
	"github.com/lansespirit/Clipal/internal/logger"
	oauthpkg "github.com/lansespirit/Clipal/internal/oauth"
)

const (
	maxOAuthImportFiles           = 512
	maxOAuthImportFileBytes       = 1 << 20 // 1 MiB per uploaded credential file
	oauthImportMultipartMaxMemory = 8 << 20 // spill larger payloads to disk
)

type oauthImportCandidate struct {
	cred   *oauthpkg.Credential
	result OAuthImportFileResultResponse
}

func (a *API) HandleImportCLIProxyAPICredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(oauthImportMultipartMaxMemory); err != nil {
		writeError(w, fmt.Sprintf("invalid multipart form: %v", err), http.StatusBadRequest)
		return
	}

	clientType, ok := config.CanonicalClientType(strings.TrimSpace(r.FormValue("client_type")))
	if !ok {
		writeError(w, "invalid client type", http.StatusBadRequest)
		return
	}
	requestedProvider := config.OAuthProvider(strings.ToLower(strings.TrimSpace(r.FormValue("provider"))))
	if requestedProvider == "" {
		writeError(w, "provider is required", http.StatusBadRequest)
		return
	}
	if err := validateOAuthProviderForClient(clientType, requestedProvider); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		writeError(w, "no credential files uploaded", http.StatusBadRequest)
		return
	}
	if len(headers) > maxOAuthImportFiles {
		writeError(w, fmt.Sprintf("too many credential files: max %d", maxOAuthImportFiles), http.StatusBadRequest)
		return
	}

	resp := OAuthImportResponse{
		ClientType: clientType,
		Provider:   requestedProvider,
		Results:    make([]OAuthImportFileResultResponse, 0, len(headers)),
	}
	candidates := make([]oauthImportCandidate, 0, len(headers))
	for _, header := range headers {
		candidate := a.parseCLIProxyAPIImportCandidate(header, requestedProvider)
		resp.addResult(candidate.result)
		candidates = append(candidates, candidate)
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}
	cc, err := getClientConfigRef(cfg, clientType)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	restoreOAuthDir, finalizeOAuthDir, err := snapshotOAuthImportProviderDir(filepath.Join(a.configDir, "oauth", string(requestedProvider)))
	if err != nil {
		writeAPIError(w, newAPIError(http.StatusInternalServerError, fmt.Sprintf("failed to prepare oauth import rollback: %v", err), err))
		return
	}
	rolledBackOAuthDir := false
	defer func() {
		if rolledBackOAuthDir || finalizeOAuthDir == nil {
			return
		}
		if err := finalizeOAuthDir(); err != nil {
			logger.Warn("failed to finalize oauth import snapshot for %s: %v", requestedProvider, err)
		}
	}()
	rollbackOAuthDir := func(baseErr error) error {
		if restoreOAuthDir == nil {
			return baseErr
		}
		rolledBackOAuthDir = true
		if restoreErr := restoreOAuthDir(); restoreErr != nil {
			return fmt.Errorf("%w (oauth rollback failed: %v)", baseErr, restoreErr)
		}
		return baseErr
	}

	seen := make([]*oauthpkg.Credential, 0, len(candidates))
	changed := false
	for i := range candidates {
		candidate := &candidates[i]
		if candidate.cred == nil {
			continue
		}

		if oauthImportCandidateSeen(seen, candidate.cred) {
			candidate.result.Status = "skipped"
			candidate.result.Message = "duplicate account in selected files"
			candidate.cred = nil
			resp.recountResult(i, candidate.result)
			continue
		}

		if err := a.oauth.Store().Save(candidate.cred); err != nil {
			candidate.result.Status = "failed"
			candidate.result.Message = fmt.Sprintf("save imported credential: %v", err)
			candidate.cred = nil
			resp.recountResult(i, candidate.result)
			continue
		}
		seen = append(seen, candidate.cred.Clone())
		candidate.result.Ref = candidate.cred.Ref
		candidate.result.Email = candidate.cred.Email

		link := ensureOAuthProviderLinked(cc, candidate.cred, a.oauth.Load)
		candidate.result.Status = "imported"
		candidate.result.ProviderName = link.Provider.Name
		candidate.result.ProviderAction = string(link.Action)
		switch link.Action {
		case oauthProviderActionCreated:
			candidate.result.Message = fmt.Sprintf("imported account and created provider %s", link.Provider.Name)
			resp.LinkedCount++
		case oauthProviderActionRelinked:
			candidate.result.Message = fmt.Sprintf("imported account and relinked provider %s", link.Provider.Name)
			resp.LinkedCount++
		default:
			candidate.result.Message = fmt.Sprintf("imported account and reused provider %s", link.Provider.Name)
		}
		changed = changed || link.Changed
		resp.recountResult(i, candidate.result)
	}

	if changed {
		if err := cfg.Validate(); err != nil {
			err = rollbackOAuthDir(fmt.Errorf("invalid configuration: %w", err))
			writeAPIError(w, newAPIError(http.StatusBadRequest, err.Error(), err))
			return
		}
		restoreConfig, err := a.saveClientConfigWithRollback(clientType, *cc)
		if err != nil {
			err = rollbackOAuthDir(fmt.Errorf("failed to save config: %w", err))
			writeAPIError(w, newAPIError(http.StatusInternalServerError, err.Error(), err))
			return
		}
		if err := a.reloadRuntimeProviderConfigs(); err != nil {
			if restoreErr := restoreConfig(); restoreErr != nil {
				err = fmt.Errorf("failed to apply saved config: %w (config rollback failed: %v)", err, restoreErr)
			} else {
				err = fmt.Errorf("failed to apply saved config: %w", err)
			}
			err = rollbackOAuthDir(err)
			writeAPIError(w, newAPIError(http.StatusInternalServerError, err.Error(), err))
			return
		}
	}

	resp.Message = summarizeOAuthImport(resp)
	writeJSON(w, resp)
}

func (a *API) parseCLIProxyAPIImportCandidate(header *multipart.FileHeader, requestedProvider config.OAuthProvider) oauthImportCandidate {
	result := OAuthImportFileResultResponse{
		File:   strings.TrimSpace(header.Filename),
		Status: "skipped",
	}
	if result.File == "" {
		result.File = "credential.json"
	}
	if ext := strings.ToLower(filepath.Ext(result.File)); ext != ".json" {
		result.Message = "skipped non-JSON file"
		return oauthImportCandidate{result: result}
	}

	data, err := readOAuthImportFile(header)
	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
		return oauthImportCandidate{result: result}
	}
	cred, err := oauthpkg.ParseCLIProxyAPICredential(data)
	if err != nil {
		switch {
		case errors.Is(err, oauthpkg.ErrCLIProxyAPINotCredential):
			result.Message = "skipped file without supported OAuth credential data"
		case errors.Is(err, oauthpkg.ErrCLIProxyAPIUnsupportedType):
			result.Message = err.Error()
		case errors.Is(err, oauthpkg.ErrCLIProxyAPIDisabledCredential):
			result.Message = "skipped disabled OAuth credential"
		default:
			result.Status = "failed"
			result.Message = err.Error()
		}
		return oauthImportCandidate{result: result}
	}
	if cred.Provider != requestedProvider {
		result.Message = fmt.Sprintf("skipped %s credential while importing %s accounts", cred.Provider, requestedProvider)
		return oauthImportCandidate{result: result}
	}

	result.Provider = cred.Provider
	result.Ref = cred.Ref
	result.Email = cred.Email
	return oauthImportCandidate{cred: cred, result: result}
}

func readOAuthImportFile(header *multipart.FileHeader) ([]byte, error) {
	f, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("open uploaded file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(f, maxOAuthImportFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read uploaded file: %w", err)
	}
	if len(data) > maxOAuthImportFileBytes {
		return nil, fmt.Errorf("uploaded file exceeds %d bytes", maxOAuthImportFileBytes)
	}
	return data, nil
}

func snapshotOAuthImportProviderDir(path string) (func() error, func() error, error) {
	path = filepath.Clean(path)
	// #nosec G703 -- path is derived from Clipal's configDir/oauth/<validated-provider>.
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return func() error {
				// #nosec G703 -- path is derived from Clipal's configDir/oauth/<validated-provider>.
				return os.RemoveAll(path)
			}, func() error { return nil }, nil
		}
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("oauth provider path is not a directory: %s", path)
	}
	backupPath, err := prepareOAuthImportBackupPath(path)
	if err != nil {
		return nil, nil, err
	}
	// #nosec G703 -- path is derived from Clipal's configDir/oauth/<validated-provider>.
	if err := os.MkdirAll(backupPath, info.Mode().Perm()); err != nil {
		return nil, nil, err
	}
	// #nosec G703 -- path is derived from Clipal's configDir/oauth/<validated-provider>.
	protectedPaths, err := copyOAuthImportSnapshot(path, backupPath)
	if err != nil {
		// #nosec G703 -- backupPath is generated under Clipal's validated oauth provider directory.
		_ = os.RemoveAll(backupPath)
		return nil, nil, err
	}

	return func() error {
			stashDir := ""
			if len(protectedPaths) > 0 {
				// #nosec G703 -- path is derived from Clipal's configDir/oauth/<validated-provider>.
				stashDir, err = os.MkdirTemp(filepath.Dir(path), "."+filepath.Base(path)+"-restore-protected-*")
				if err != nil {
					return err
				}
				defer func() {
					if stashDir != "" {
						// #nosec G703 -- stashDir is created under a validated temp parent above.
						_ = os.RemoveAll(stashDir)
					}
				}()
				for rel := range protectedPaths {
					currentPath := filepath.Join(path, rel)
					// #nosec G703 -- currentPath stays under Clipal's validated oauth provider directory.
					if _, err := os.Lstat(currentPath); err != nil {
						continue
					}
					stashPath := filepath.Join(stashDir, rel)
					// #nosec G703 -- stashPath stays under the temp restore directory created above.
					if err := os.MkdirAll(filepath.Dir(stashPath), 0o700); err != nil {
						return err
					}
					// #nosec G703 -- both currentPath and stashPath are derived from validated directories above.
					if err := os.Rename(currentPath, stashPath); err != nil {
						return err
					}
				}
			}
			// #nosec G703 -- path is derived from Clipal's configDir/oauth/<validated-provider>.
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			// #nosec G703 -- path is derived from Clipal's configDir/oauth/<validated-provider>.
			if err := os.Rename(backupPath, path); err != nil {
				return err
			}
			if stashDir != "" {
				for rel := range protectedPaths {
					stashPath := filepath.Join(stashDir, rel)
					// #nosec G703 -- stashPath stays under the temp restore directory created above.
					if _, err := os.Lstat(stashPath); err != nil {
						continue
					}
					targetPath := filepath.Join(path, rel)
					// #nosec G703 -- targetPath stays under Clipal's validated oauth provider directory.
					if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
						return err
					}
					// #nosec G703 -- stashPath and targetPath are derived from validated directories above.
					if err := os.Rename(stashPath, targetPath); err != nil {
						return err
					}
				}
			}
			return nil
		}, func() error {
			// #nosec G703 -- path is derived from Clipal's configDir/oauth/<validated-provider>.
			return os.RemoveAll(backupPath)
		}, nil
}

func prepareOAuthImportBackupPath(path string) (string, error) {
	backupPath, err := os.MkdirTemp(filepath.Dir(path), "."+filepath.Base(path)+"-import-backup-*")
	if err != nil {
		return "", err
	}
	// #nosec G703 -- backupPath is generated under Clipal's validated oauth provider directory.
	return backupPath, os.Remove(backupPath)
}

func copyOAuthImportSnapshot(srcRoot string, dstRoot string) (map[string]struct{}, error) {
	protectedPaths := make(map[string]struct{})
	// #nosec G703 -- path is derived from Clipal's configDir/oauth/<validated-provider>.
	err := filepath.WalkDir(srcRoot, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if current == srcRoot {
				return walkErr
			}
			if rel, err := filepath.Rel(srcRoot, current); err == nil && rel != "." {
				protectedPaths[rel] = struct{}{}
			}
			return nil
		}
		if current == srcRoot {
			return nil
		}

		rel, err := filepath.Rel(srcRoot, current)
		if err != nil {
			return nil
		}
		dstPath := filepath.Join(dstRoot, rel)

		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				protectedPaths[rel] = struct{}{}
				return nil
			}
			return os.MkdirAll(dstPath, info.Mode().Perm())
		}
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(current)
			if err != nil {
				protectedPaths[rel] = struct{}{}
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
				return err
			}
			return os.Symlink(target, dstPath)
		}
		if !d.Type().IsRegular() {
			protectedPaths[rel] = struct{}{}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			protectedPaths[rel] = struct{}{}
			return nil
		}
		data, err := os.ReadFile(current)
		if err != nil {
			protectedPaths[rel] = struct{}{}
			return nil
		}
		return atomicWriteFile(dstPath, data, info.Mode().Perm())
	})
	return protectedPaths, err
}

func (resp *OAuthImportResponse) addResult(result OAuthImportFileResultResponse) {
	resp.Results = append(resp.Results, result)
	switch result.Status {
	case "imported":
		resp.ImportedCount++
	case "failed":
		resp.FailedCount++
	default:
		resp.SkippedCount++
	}
}

func (resp *OAuthImportResponse) recountResult(index int, next OAuthImportFileResultResponse) {
	if resp == nil || index < 0 || index >= len(resp.Results) {
		return
	}
	prev := resp.Results[index]
	resp.adjustResultCount(prev.Status, -1)
	resp.adjustResultCount(next.Status, 1)
	resp.Results[index] = next
}

func (resp *OAuthImportResponse) adjustResultCount(status string, delta int) {
	switch status {
	case "imported":
		resp.ImportedCount += delta
	case "failed":
		resp.FailedCount += delta
	default:
		resp.SkippedCount += delta
	}
}

func summarizeOAuthImport(resp OAuthImportResponse) string {
	parts := []string{
		fmt.Sprintf("imported %d account(s)", resp.ImportedCount),
		fmt.Sprintf("linked %d provider(s)", resp.LinkedCount),
	}
	if resp.SkippedCount > 0 {
		parts = append(parts, fmt.Sprintf("skipped %d file(s)", resp.SkippedCount))
	}
	if resp.FailedCount > 0 {
		parts = append(parts, fmt.Sprintf("failed %d file(s)", resp.FailedCount))
	}
	return strings.Join(parts, ", ")
}

func oauthImportCandidateSeen(seen []*oauthpkg.Credential, cred *oauthpkg.Credential) bool {
	for _, existing := range seen {
		if sameOAuthImportCandidate(existing, cred) {
			return true
		}
	}
	return false
}

func sameOAuthImportCandidate(a *oauthpkg.Credential, b *oauthpkg.Credential) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Provider != b.Provider {
		return false
	}
	if ref := strings.TrimSpace(a.Ref); ref != "" && ref == strings.TrimSpace(b.Ref) {
		return true
	}
	return oauthpkg.SameAccountIdentity(a, b)
}
