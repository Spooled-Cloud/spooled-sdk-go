package spooled

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Headers Spooled sets on a signed outgoing webhook or completion callback.
const (
	// WebhookSignatureHeader carries "sha256=<hex HMAC-SHA256>" of
	// "{timestamp}.{body}", keyed with the webhook secret.
	WebhookSignatureHeader = "X-Spooled-Signature"
	// WebhookTimestampHeader carries the Unix time (seconds) the delivery was
	// signed at.
	WebhookTimestampHeader = "X-Spooled-Timestamp"
	// DefaultWebhookTolerance is the age VerifyWebhookSignature accepts when it
	// is given a tolerance of zero or less.
	DefaultWebhookTolerance = 5 * time.Minute
)

// Errors returned by VerifyWebhookSignature. Answer any of them with a 4xx and
// do not process the delivery.
var (
	// ErrWebhookSecretEmpty means no secret was passed to verify with.
	ErrWebhookSecretEmpty = errors.New("spooled: webhook secret is empty")
	// ErrWebhookSignatureMissing means the signature header is empty or does
	// not have the "sha256=<hex>" form.
	ErrWebhookSignatureMissing = errors.New("spooled: webhook signature is missing or malformed")
	// ErrWebhookTimestampInvalid means the timestamp header is empty or not a
	// Unix time in seconds.
	ErrWebhookTimestampInvalid = errors.New("spooled: webhook timestamp is missing or invalid")
	// ErrWebhookTimestampExpired means the timestamp is further from now than
	// the tolerance (a replayed or delayed delivery).
	ErrWebhookTimestampExpired = errors.New("spooled: webhook timestamp is outside the tolerance")
	// ErrWebhookSignatureInvalid means the signature does not match the body.
	ErrWebhookSignatureInvalid = errors.New("spooled: webhook signature does not match")
)

// SignWebhookPayload returns the X-Spooled-Signature value Spooled sends for
// body signed at timestamp (Unix seconds): "sha256=" followed by the hex
// HMAC-SHA256 of "{timestamp}.{body}" keyed with secret. It is useful in tests
// of a webhook handler.
func SignWebhookPayload(secret string, timestamp int64, body []byte) string {
	return "sha256=" + hex.EncodeToString(webhookMAC(secret, strconv.FormatInt(timestamp, 10), body))
}

// VerifyWebhookSignature checks a webhook delivery from Spooled (an outgoing
// webhook, or a job's completion callback created with a secret).
//
// Pass the webhook secret, the X-Spooled-Timestamp and X-Spooled-Signature
// header values, and the raw request body exactly as received (before any JSON
// decoding). It returns nil when the signature matches and the timestamp is
// within tolerance of the current time; a tolerance of zero or less uses
// DefaultWebhookTolerance. The signature is compared in constant time.
//
//	body, _ := io.ReadAll(r.Body)
//	err := spooled.VerifyWebhookSignature(secret,
//		r.Header.Get(spooled.WebhookTimestampHeader),
//		r.Header.Get(spooled.WebhookSignatureHeader),
//		body, 5*time.Minute)
//	if err != nil {
//		http.Error(w, "invalid signature", http.StatusUnauthorized)
//		return
//	}
func VerifyWebhookSignature(secret, timestampHeader, signatureHeader string, body []byte, tolerance time.Duration) error {
	return verifyWebhookSignatureAt(secret, timestampHeader, signatureHeader, body, tolerance, time.Now())
}

func verifyWebhookSignatureAt(secret, timestampHeader, signatureHeader string, body []byte, tolerance time.Duration, now time.Time) error {
	if secret == "" {
		return ErrWebhookSecretEmpty
	}
	hexSig, ok := strings.CutPrefix(strings.TrimSpace(signatureHeader), "sha256=")
	if !ok || hexSig == "" {
		return ErrWebhookSignatureMissing
	}
	given, err := hex.DecodeString(hexSig)
	if err != nil {
		return ErrWebhookSignatureMissing
	}
	timestampHeader = strings.TrimSpace(timestampHeader)
	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil || ts <= 0 {
		return ErrWebhookTimestampInvalid
	}
	if tolerance <= 0 {
		tolerance = DefaultWebhookTolerance
	}
	// Compare whole seconds as int64: a time.Duration difference saturates for
	// a timestamp centuries away, and negating the saturated minimum overflows
	// back to a negative age that would pass the check.
	skew := now.Unix() - ts
	if skew < 0 {
		skew = -skew
	}
	if skew > int64(tolerance/time.Second) {
		return ErrWebhookTimestampExpired
	}
	if !hmac.Equal(given, webhookMAC(secret, timestampHeader, body)) {
		return ErrWebhookSignatureInvalid
	}
	return nil
}

func webhookMAC(secret, timestamp string, body []byte) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return mac.Sum(nil)
}
