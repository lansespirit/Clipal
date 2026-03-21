package proxy

import (
	"bufio"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

func sanitizeLogString(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

func readResponseBodyBytes(resp *http.Response, maxBytes int64) (data []byte, truncated bool) {
	if resp == nil || resp.Body == nil {
		return nil, false
	}
	if maxBytes <= 0 {
		_ = resp.Body.Close()
		return nil, false
	}

	br := bufio.NewReader(resp.Body)
	peek, _ := br.Peek(2)
	enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	isGzip := strings.Contains(enc, "gzip") || (len(peek) == 2 && peek[0] == 0x1f && peek[1] == 0x8b)

	if isGzip {
		if gz, err := gzip.NewReader(br); err == nil {
			defer func() { _ = gz.Close() }()
			data, _ = io.ReadAll(io.LimitReader(gz, maxBytes+1))
			truncated = int64(len(data)) > maxBytes
			if truncated {
				data = data[:maxBytes]
			}
			// Best-effort drain to encourage connection reuse, but don't hang on huge bodies.
			_, _ = io.Copy(io.Discard, io.LimitReader(gz, 512*1024))
			return data, truncated
		}
	}

	data, _ = io.ReadAll(io.LimitReader(br, maxBytes+1))
	truncated = int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(br, 512*1024))
	return data, truncated
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
