// Package auth implements cvx's authentication: session cookies signed with
// HMAC-SHA256, Google/GitHub OAuth2 sign-in, and the Echo middleware that
// resolves a request's userID (from a dev-mode env var or a verified
// session cookie) into request context.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// SessionCookieName is the cookie carrying the signed session.
const SessionCookieName = "cvx_auth"

// SessionTTL is how long a session cookie stays valid after sign-in.
const SessionTTL = 30 * 24 * time.Hour

// Sign produces a session cookie value for userID, valid for ttl from now:
// base64url(payload) + "|" + hex(hmac-sha256(payload)), where payload is
// "userID:expiryUnix". The payload is authenticated (not encrypted) — it is
// not secret, just tamper-proof.
func Sign(userID int64, ttl time.Duration, secret string) string {
	expiry := time.Now().Add(ttl).Unix()
	payload := strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(expiry, 10)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return encoded + "|" + hmacHex(encoded, secret)
}

// Verify checks a session cookie value's signature (constant-time) and
// expiry, returning the userID it carries when valid.
func Verify(value, secret string) (int64, bool) {
	encoded, sig, ok := strings.Cut(value, "|")
	if !ok || encoded == "" || sig == "" {
		return 0, false
	}
	want := hmacHex(encoded, secret)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(want)) != 1 {
		return 0, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return 0, false
	}
	userIDStr, expiryStr, ok := strings.Cut(string(payload), ":")
	if !ok {
		return 0, false
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return 0, false
	}
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return 0, false
	}
	if time.Now().Unix() > expiry {
		return 0, false
	}
	return userID, true
}

func hmacHex(encoded, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encoded))
	return hex.EncodeToString(mac.Sum(nil))
}
