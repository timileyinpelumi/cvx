package auth

import (
	"strings"
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	cookie := Sign(42, time.Hour, "secret")
	if cookie == "" || !strings.Contains(cookie, "|") {
		t.Fatalf("want well-formed cookie value, got %q", cookie)
	}

	userID, ok := Verify(cookie, "secret")
	if !ok || userID != 42 {
		t.Fatalf("want (42, true), got (%d, %v)", userID, ok)
	}
}

func TestVerifyWrongSecretFails(t *testing.T) {
	cookie := Sign(42, time.Hour, "secret")
	if _, ok := Verify(cookie, "wrong-secret"); ok {
		t.Fatal("want verify to fail with wrong secret")
	}
}

func TestVerifyTamperedPayloadFails(t *testing.T) {
	cookie := Sign(42, time.Hour, "secret")
	parts := strings.SplitN(cookie, "|", 2)
	if len(parts) != 2 {
		t.Fatalf("bad cookie shape: %q", cookie)
	}
	// Flip the payload but keep the original signature: must not verify.
	tampered := Sign(999, time.Hour, "secret")
	tamperedParts := strings.SplitN(tampered, "|", 2)
	forged := tamperedParts[0] + "|" + parts[1]
	if _, ok := Verify(forged, "secret"); ok {
		t.Fatal("want verify to fail for tampered payload with mismatched signature")
	}
}

func TestVerifyExpiredFails(t *testing.T) {
	cookie := Sign(42, -time.Hour, "secret") // already expired
	if _, ok := Verify(cookie, "secret"); ok {
		t.Fatal("want verify to fail for expired cookie")
	}
}

func TestVerifyMalformedFails(t *testing.T) {
	cases := []string{"", "no-pipe-here", "abc|", "|abc", "abc|not-hex"}
	for _, c := range cases {
		if _, ok := Verify(c, "secret"); ok {
			t.Fatalf("want verify to fail for malformed value %q", c)
		}
	}
}
