package spooled

import (
	"errors"
	"math"
	"strconv"
	"testing"
	"time"
)

func TestVerifyWebhookSignature(t *testing.T) {
	const secret = "whsec_test"
	body := []byte(`{"event":"job.completed","data":{"job_id":"j1"}}`)
	now := time.Unix(1_800_000_000, 0)
	ts := strconv.FormatInt(now.Unix(), 10)
	sig := SignWebhookPayload(secret, now.Unix(), body)

	tests := []struct {
		name      string
		secret    string
		timestamp string
		signature string
		body      []byte
		tolerance time.Duration
		at        time.Time
		want      error
	}{
		{"valid", secret, ts, sig, body, time.Minute, now, nil},
		{"valid with default tolerance", secret, ts, sig, body, 0, now.Add(4 * time.Minute), nil},
		{"uppercase hex", secret, ts, "sha256=" + upperHex(sig[len("sha256="):]), body, time.Minute, now, nil},
		{"tampered body", secret, ts, sig, []byte(`{"event":"job.failed"}`), time.Minute, now, ErrWebhookSignatureInvalid},
		{"wrong secret", "whsec_other", ts, sig, body, time.Minute, now, ErrWebhookSignatureInvalid},
		{"timestamp swapped", secret, strconv.FormatInt(now.Unix()+1, 10), sig, body, time.Minute, now, ErrWebhookSignatureInvalid},
		{"too old", secret, ts, sig, body, time.Minute, now.Add(2 * time.Minute), ErrWebhookTimestampExpired},
		{"from the future", secret, ts, sig, body, time.Minute, now.Add(-2 * time.Minute), ErrWebhookTimestampExpired},
		{"default tolerance exceeded", secret, ts, sig, body, 0, now.Add(6 * time.Minute), ErrWebhookTimestampExpired},
		{"missing signature", secret, ts, "", body, time.Minute, now, ErrWebhookSignatureMissing},
		{"no sha256 prefix", secret, ts, sig[len("sha256="):], body, time.Minute, now, ErrWebhookSignatureMissing},
		{"non-hex signature", secret, ts, "sha256=zz", body, time.Minute, now, ErrWebhookSignatureMissing},
		{"missing timestamp", secret, "", sig, body, time.Minute, now, ErrWebhookTimestampInvalid},
		{"non-numeric timestamp", secret, "yesterday", sig, body, time.Minute, now, ErrWebhookTimestampInvalid},
		{"zero timestamp", secret, "0", SignWebhookPayload(secret, 0, body), body, time.Minute, now, ErrWebhookTimestampInvalid},
		{"negative timestamp", secret, "-1", SignWebhookPayload(secret, -1, body), body, time.Minute, now, ErrWebhookTimestampInvalid},
		{"far future (max int64)", secret, strconv.FormatInt(math.MaxInt64, 10), SignWebhookPayload(secret, math.MaxInt64, body), body, time.Minute, now, ErrWebhookTimestampExpired},
		{"far future (year 2603, past time.Duration range)", secret, "20000000000", SignWebhookPayload(secret, 20_000_000_000, body), body, time.Minute, now, ErrWebhookTimestampExpired},
		{"empty secret", "", ts, sig, body, time.Minute, now, ErrWebhookSecretEmpty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyWebhookSignatureAt(tt.secret, tt.timestamp, tt.signature, tt.body, tt.tolerance, tt.at)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSignWebhookPayload_MatchesBackendScheme(t *testing.T) {
	got := SignWebhookPayload("whsec_test", 1_700_000_000, []byte("{}"))
	const want = "sha256=" + knownSignature
	if got != want {
		t.Fatalf("SignWebhookPayload = %s, want %s", got, want)
	}
}

func TestVerifyWebhookSignature_UsesCurrentTime(t *testing.T) {
	body := []byte("{}")
	now := time.Now().Unix()
	sig := SignWebhookPayload("s", now, body)
	if err := VerifyWebhookSignature("s", strconv.FormatInt(now, 10), sig, body, time.Minute); err != nil {
		t.Fatalf("fresh delivery rejected: %v", err)
	}
}

// knownSignature is HMAC-SHA256 keyed "whsec_test" over "1700000000.{}",
// computed with: printf '%s' '1700000000.{}' | openssl dgst -sha256 -hmac whsec_test
const knownSignature = "35495024f4ef3f94e5a93e22221544c4b75e9a42300cd965ab81cb85cd994e91"

func upperHex(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'f' {
			b[i] = c - 'a' + 'A'
		}
	}
	return string(b)
}
