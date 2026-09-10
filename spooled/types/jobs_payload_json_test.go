package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

// CreateJobRequest, BulkJobItem and ClaimedJob all carry a payload that the API
// stores as serde_json::Value. While these were map[string]any, encoding/json
// rejected a string or array outright, so such a job could be neither sent nor
// claimed through this package.
func TestCreateJobRequest_RoundTripsNonObjectPayload(t *testing.T) {
	cases := []struct {
		name string
		json string
		want any
	}{
		{name: "string", json: `"hello"`, want: "hello"},
		{name: "array", json: `[1,2]`, want: []any{float64(1), float64(2)}},
		{name: "object", json: `{"k":"v"}`, want: map[string]any{"k": "v"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req CreateJobRequest
			if err := json.Unmarshal([]byte(`{"queue_name":"q","payload":`+tc.json+`}`), &req); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(req.Payload, tc.want) {
				t.Fatalf("Payload = %#v, want %#v", req.Payload, tc.want)
			}

			out, err := json.Marshal(&req)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("Unmarshal round trip: %v", err)
			}
			if string(got["payload"]) != tc.json {
				t.Fatalf("payload on the wire = %s, want %s", got["payload"], tc.json)
			}
		})
	}
}

func TestCreateJobRequest_RoundTripsArrayTags(t *testing.T) {
	var req CreateJobRequest
	if err := json.Unmarshal([]byte(`{"queue_name":"q","payload":{},"tags":["urgent"]}`), &req); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(req.Tags, []any{"urgent"}) {
		t.Fatalf("Tags = %#v, want []any{\"urgent\"}", req.Tags)
	}
}

func TestBulkJobItem_RoundTripsNonObjectPayload(t *testing.T) {
	var item BulkJobItem
	if err := json.Unmarshal([]byte(`{"payload":["a"]}`), &item); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(item.Payload, []any{"a"}) {
		t.Fatalf("Payload = %#v, want []any{\"a\"}", item.Payload)
	}
}

func TestClaimedJob_RoundTripsNonObjectPayload(t *testing.T) {
	var job ClaimedJob
	body := `{"id":"job_1","queue_name":"q","payload":["item"],"retry_count":0,"max_retries":3,"timeout_seconds":30}`
	if err := json.Unmarshal([]byte(body), &job); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(job.Payload, []any{"item"}) {
		t.Fatalf("Payload = %#v, want []any{\"item\"}", job.Payload)
	}
}
