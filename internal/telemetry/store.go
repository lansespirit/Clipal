package telemetry

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	storeFilename          = "usage.json"
	storeVersion           = 1
	defaultPersistInterval = 3 * time.Second
)

type UsageDelta struct {
	InputTokens  int64 `json:"input_tokens,omitempty"`
	OutputTokens int64 `json:"output_tokens,omitempty"`
	TotalTokens  int64 `json:"total_tokens,omitempty"`
}

type UsageSnapshot struct {
	UsageDelta
	Usage map[string]any `json:"usage,omitempty"`
}

type RecordOptions struct {
	CountRequest bool
	CountSuccess bool
}

func (u UsageDelta) normalized() UsageDelta {
	if u.TotalTokens <= 0 {
		total := u.InputTokens + u.OutputTokens
		if total > 0 {
			u.TotalTokens = total
		}
	}
	return u
}

type ProviderUsage struct {
	RequestCount int64          `json:"request_count,omitempty"`
	SuccessCount int64          `json:"success_count,omitempty"`
	InputTokens  int64          `json:"input_tokens,omitempty"`
	OutputTokens int64          `json:"output_tokens,omitempty"`
	TotalTokens  int64          `json:"total_tokens,omitempty"`
	LastUsedAt   time.Time      `json:"last_used_at,omitempty"`
	Usage        map[string]any `json:"usage,omitempty"`
}

type clientUsage struct {
	Providers map[string]ProviderUsage `json:"providers,omitempty"`
}

type storeState struct {
	Version   int                    `json:"version"`
	UpdatedAt time.Time              `json:"updated_at,omitempty"`
	Clients   map[string]clientUsage `json:"clients,omitempty"`
}

type Store struct {
	path            string
	persistInterval time.Duration

	mu          sync.RWMutex
	state       storeState
	dirty       bool
	lastPersist time.Time
	revision    uint64
}

var afterCloneForFlush func()

func NewStore(configDir string) (*Store, error) {
	configDir = strings.TrimSpace(configDir)
	s := &Store{
		persistInterval: defaultPersistInterval,
		state: storeState{
			Version: storeVersion,
			Clients: map[string]clientUsage{},
		},
	}
	if configDir == "" {
		return s, nil
	}
	s.path = filepath.Join(configDir, storeFilename)
	if err := s.load(); err != nil {
		return s, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var state storeState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Version == 0 {
		state.Version = storeVersion
	}
	if state.Clients == nil {
		state.Clients = map[string]clientUsage{}
	}
	for client, usage := range state.Clients {
		if usage.Providers == nil {
			usage.Providers = map[string]ProviderUsage{}
			state.Clients[client] = usage
		}
	}
	s.state = state
	return nil
}

func (s *Store) RecordUsage(clientType string, provider string, snapshot UsageSnapshot, when time.Time) error {
	return s.Record(clientType, provider, snapshot, when, RecordOptions{
		CountRequest: true,
		CountSuccess: true,
	})
}

func (s *Store) Record(clientType string, provider string, snapshot UsageSnapshot, when time.Time, options RecordOptions) error {
	clientType = strings.TrimSpace(clientType)
	provider = strings.TrimSpace(provider)
	if s == nil || clientType == "" || provider == "" || (!options.CountRequest && !options.CountSuccess) {
		return nil
	}

	delta := snapshot.normalized()
	if when.IsZero() {
		when = time.Now()
	}

	s.mu.Lock()
	client := s.state.Clients[clientType]
	if client.Providers == nil {
		client.Providers = map[string]ProviderUsage{}
	}
	entry := client.Providers[provider]
	if options.CountRequest {
		entry.RequestCount++
	}
	if options.CountSuccess {
		entry.SuccessCount++
	}
	entry.InputTokens += delta.InputTokens
	entry.OutputTokens += delta.OutputTokens
	entry.TotalTokens += delta.TotalTokens
	entry.LastUsedAt = when
	if snapshot.Usage != nil {
		entry.Usage = cloneMap(snapshot.Usage)
	}
	client.Providers[provider] = entry
	s.state.Clients[clientType] = client
	s.state.Version = storeVersion
	s.state.UpdatedAt = when
	s.dirty = true
	s.revision++
	shouldPersist := s.lastPersist.IsZero() || when.Sub(s.lastPersist) >= s.persistInterval
	s.mu.Unlock()

	if shouldPersist {
		return s.Flush()
	}
	return nil
}

func (s *Store) ProviderSnapshot(clientType string, provider string) (ProviderUsage, bool) {
	clientType = strings.TrimSpace(clientType)
	provider = strings.TrimSpace(provider)
	if s == nil || clientType == "" || provider == "" {
		return ProviderUsage{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	client, ok := s.state.Clients[clientType]
	if !ok || client.Providers == nil {
		return ProviderUsage{}, false
	}
	usage, ok := client.Providers[provider]
	if usage.Usage != nil {
		usage.Usage = cloneMap(usage.Usage)
	}
	return usage, ok
}

func (s *Store) ProviderSnapshots(clientType string) map[string]ProviderUsage {
	clientType = strings.TrimSpace(clientType)
	if s == nil || clientType == "" {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	client, ok := s.state.Clients[clientType]
	if !ok || len(client.Providers) == 0 {
		return nil
	}
	out := make(map[string]ProviderUsage, len(client.Providers))
	for name, usage := range client.Providers {
		if usage.Usage != nil {
			usage.Usage = cloneMap(usage.Usage)
		}
		out[name] = usage
	}
	return out
}

func (s *Store) RenameProvider(clientType string, from string, to string) error {
	clientType = strings.TrimSpace(clientType)
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if s == nil || clientType == "" || from == "" || to == "" || from == to {
		return nil
	}

	s.mu.Lock()
	client, ok := s.state.Clients[clientType]
	if !ok || client.Providers == nil {
		s.mu.Unlock()
		return nil
	}
	entry, ok := client.Providers[from]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	if existing, exists := client.Providers[to]; exists {
		entry.RequestCount += existing.RequestCount
		entry.SuccessCount += existing.SuccessCount
		entry.InputTokens += existing.InputTokens
		entry.OutputTokens += existing.OutputTokens
		entry.TotalTokens += existing.TotalTokens
		if existing.LastUsedAt.After(entry.LastUsedAt) {
			entry.LastUsedAt = existing.LastUsedAt
			if existing.Usage != nil {
				entry.Usage = cloneMap(existing.Usage)
			}
		}
		if entry.Usage == nil && existing.Usage != nil {
			entry.Usage = cloneMap(existing.Usage)
		}
	}
	delete(client.Providers, from)
	client.Providers[to] = entry
	s.state.Clients[clientType] = client
	s.state.Version = storeVersion
	s.state.UpdatedAt = time.Now()
	s.dirty = true
	s.revision++
	s.mu.Unlock()
	return s.Flush()
}

func (s *Store) DeleteProvider(clientType string, provider string) error {
	clientType = strings.TrimSpace(clientType)
	provider = strings.TrimSpace(provider)
	if s == nil || clientType == "" || provider == "" {
		return nil
	}

	s.mu.Lock()
	client, ok := s.state.Clients[clientType]
	if !ok || client.Providers == nil {
		s.mu.Unlock()
		return nil
	}
	if _, ok := client.Providers[provider]; !ok {
		s.mu.Unlock()
		return nil
	}
	delete(client.Providers, provider)
	if len(client.Providers) == 0 {
		delete(s.state.Clients, clientType)
	} else {
		s.state.Clients[clientType] = client
	}
	s.state.Version = storeVersion
	s.state.UpdatedAt = time.Now()
	s.dirty = true
	s.revision++
	s.mu.Unlock()
	return s.Flush()
}

func (s *Store) Flush() error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}

	for {
		s.mu.Lock()
		if !s.dirty {
			s.mu.Unlock()
			return nil
		}
		state := cloneState(s.state)
		revision := s.revision
		s.mu.Unlock()

		if afterCloneForFlush != nil {
			afterCloneForFlush()
		}

		data, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if err := atomicWriteFile(s.path, data, 0o600); err != nil {
			return err
		}

		now := time.Now()
		s.mu.Lock()
		s.lastPersist = now
		if s.revision == revision {
			s.dirty = false
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()
	}
}

func cloneState(state storeState) storeState {
	out := storeState{
		Version:   state.Version,
		UpdatedAt: state.UpdatedAt,
		Clients:   make(map[string]clientUsage, len(state.Clients)),
	}
	for clientName, client := range state.Clients {
		nextClient := clientUsage{
			Providers: make(map[string]ProviderUsage, len(client.Providers)),
		}
		for providerName, usage := range client.Providers {
			if usage.Usage != nil {
				usage.Usage = cloneMap(usage.Usage)
			}
			nextClient.Providers[providerName] = usage
		}
		out.Clients[clientName] = nextClient
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	data, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func atomicWriteFile(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, ".clipal-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	success := false
	defer func() {
		_ = f.Close()
		if !success {
			_ = os.Remove(tmp)
		}
	}()

	if err := f.Chmod(perm); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	success = true
	return nil
}
