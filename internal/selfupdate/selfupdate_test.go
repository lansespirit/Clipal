package selfupdate

import "testing"

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"v0.1.0", "v0.1.0", 0, true},
		{"0.1.0", "v0.1.1", -1, true},
		{"v1.2.3", "v1.2.2", 1, true},
		{"v1.2", "v1.2.0", 0, true},
		{"dev", "v1.0.0", 0, false},
		{"v1", "v2", -1, true},
	}
	for _, tc := range tests {
		t.Run(tc.a+"_"+tc.b, func(t *testing.T) {
			t.Parallel()
			got, ok := compareVersions(tc.a, tc.b)
			if ok != tc.ok {
				t.Fatalf("ok: got %v want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("cmp: got %d want %d", got, tc.want)
			}
		})
	}
}

func TestParseChecksums(t *testing.T) {
	t.Parallel()

	data := []byte(`
# comment
0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  clipal-linux-amd64
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa *checksums.txt
`)
	m, err := parseChecksums(data)
	if err != nil {
		t.Fatalf("parseChecksums: %v", err)
	}
	if got := m["clipal-linux-amd64"]; got == "" {
		t.Fatalf("missing linux amd64 checksum")
	}
	if got := m["checksums.txt"]; got == "" {
		t.Fatalf("missing checksums.txt checksum")
	}
}
