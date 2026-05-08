package kb

import "testing"

func TestIsSSLVersionOk_OpenSSL(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"OpenSSL 1.1.1k  25 Mar 2021", true},
		{"OpenSSL 3.0.2 15 Mar 2022", true},
		{"OpenSSL 1.0.2u 20 Dec 2019", false},
		{"LibreSSL 2.8.3", false},
		{"LibreSSL 3.3.6", false},
		{"BoringSSL", false},
	}
	for _, tc := range cases {
		if got := isSSLVersionOk(tc.version); got != tc.want {
			t.Fatalf("isSSLVersionOk(%q) = %t, want %t", tc.version, got, tc.want)
		}
	}
}
