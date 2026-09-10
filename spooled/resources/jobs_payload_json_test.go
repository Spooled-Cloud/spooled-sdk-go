package resources

import (
	"encoding/json"
	"reflect"
	"testing"
)

// CreateJobRequest.Payload / .Tags and BulkJobItem.Payload map onto
// serde_json::Value fields, so every JSON kind has to survive a round trip.
// While they were map[string]any, encoding/json rejected a string or array
// outright and the SDK could not enqueue such a payload at all.
func TestCreateJobRequest_RoundTripsNonObjectPayload(t *testing.T) {
	cases := []struct {
		name string
		json string
		want any
	}{
		{name: "string", json: `"hello"`, want: "hello"},
		{name: "array", json: `[1,2]`, want: []any{float64(1), float64(2)}},
		{name: "number", json: `7`, want: float64(7)},
		{name: "bool", json: `false`, want: false},
		{name: "object", json: `{"k":"v"}`, want: map[string]any{"k": "v"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req CreateJobRequest
			body := `{"queue_name":"q","payload":` + tc.json + `}`
			if err := json.Unmarshal([]byte(body), &req); err != nil {
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

func TestBulkJobItem_RoundTripsNonObjectPayload(t *testing.T) {
	cases := []struct {
		name string
		json string
		want any
	}{
		{name: "string", json: `"hello"`, want: "hello"},
		{name: "array", json: `["a"]`, want: []any{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var item BulkJobItem
			if err := json.Unmarshal([]byte(`{"payload":`+tc.json+`}`), &item); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(item.Payload, tc.want) {
				t.Fatalf("Payload = %#v, want %#v", item.Payload, tc.want)
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

// Tags stays omitted when unset, so an update does not clear server-side tags.
func TestCreateJobRequest_OmitsUnsetTags(t *testing.T) {
	out, err := json.Marshal(&CreateJobRequest{QueueName: "q", Payload: map[string]any{}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := got["tags"]; ok {
		t.Fatalf("tags present in %s, want omitted", out)
	}
}
