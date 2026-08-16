package resources

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUpdateOutgoingWebhookRequest_SecretThreeState(t *testing.T) {
	tests := []struct {
		name string
		req  UpdateOutgoingWebhookRequest
		want any // nil means the key must be absent
	}{
		{
			name: "omitted keeps the current secret",
			req:  UpdateOutgoingWebhookRequest{Name: strPtr("Job Notifications")},
			want: nil,
		},
		{
			name: "string replaces the secret",
			req:  UpdateOutgoingWebhookRequest{Secret: strPtr("whsec_rotated")},
			want: "whsec_rotated",
		},
		{
			name: "ClearSecret sends an explicit null",
			req:  UpdateOutgoingWebhookRequest{ClearSecret: true},
			want: json.RawMessage("null"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(&tt.req)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			fields := map[string]json.RawMessage{}
			if err := json.Unmarshal(body, &fields); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// ClearSecret is a client-side flag and must never reach the wire.
			if _, ok := fields["ClearSecret"]; ok {
				t.Errorf("body %s leaks the ClearSecret flag", body)
			}

			raw, ok := fields["secret"]
			switch want := tt.want.(type) {
			case nil:
				if ok {
					t.Errorf("secret = %s, want the key to be absent", raw)
				}
			case json.RawMessage:
				if !ok || string(raw) != string(want) {
					t.Errorf("secret = %s (present=%t), want %s", raw, ok, want)
				}
			case string:
				if !ok {
					t.Fatalf("secret missing from %s", body)
				}
				var got string
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Fatalf("secret is not a string: %v", err)
				}
				if got != want {
					t.Errorf("secret = %q, want %q", got, want)
				}
			}
		})
	}
}

func TestUpdateOutgoingWebhookRequest_SecretAndClearSecretConflict(t *testing.T) {
	_, err := json.Marshal(&UpdateOutgoingWebhookRequest{
		Secret:      strPtr("whsec_rotated"),
		ClearSecret: true,
	})
	if err == nil {
		t.Fatal("Marshal succeeded, want an error when Secret and ClearSecret are both set")
	}
	if !strings.Contains(err.Error(), "set exactly one") {
		t.Errorf("error = %v, want it to explain that exactly one may be set", err)
	}
}
