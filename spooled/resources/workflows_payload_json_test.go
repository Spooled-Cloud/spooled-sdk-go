package resources

import (
	"encoding/json"
	"reflect"
	"testing"
)

// WorkflowJobDefinition.payload is serde_json::Value on the API, so a workflow
// job may carry any JSON. A map[string]any field rejected a string or array.
func TestWorkflowJobDefinition_RoundTripsNonObjectPayload(t *testing.T) {
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
			var def WorkflowJobDefinition
			body := `{"key":"k","queue_name":"q","payload":` + tc.json + `}`
			if err := json.Unmarshal([]byte(body), &def); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(def.Payload, tc.want) {
				t.Fatalf("Payload = %#v, want %#v", def.Payload, tc.want)
			}

			out, err := json.Marshal(&def)
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
