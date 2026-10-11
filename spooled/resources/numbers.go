package resources

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
)

// maxExactFloatInt is the largest magnitude below which every integer is
// exactly representable as a float64 (2^53).
const maxExactFloatInt = 1 << 53

// Job payloads, results and tags are free-form JSON, so they decode into `any`
// and encoding/json turns every number there into a float64. A float64 holds
// integers exactly only up to 2^53: a Discord/Twitter/bigint ID such as
// 1234567890123456789 came back as 1234567890123456768, and a handler would act
// on the wrong entity without any error.
//
// The helpers below keep such numbers exact without changing what callers see
// for ordinary values: a number decodes to float64 exactly as before, and only
// an integer that float64 cannot hold comes back as a json.Number (use
// Int64() or String() on it).

// unmarshalJSON is json.Unmarshal that keeps number literals as json.Number
// while decoding, so they can be inspected before they lose precision.
func unmarshalJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("invalid JSON: unexpected data after top-level value")
	}
	return nil
}

// normalizeNumber returns n as a float64 when that is lossless (or when it is a
// decimal, where float64 is what callers have always received), and n itself
// when it is an integer too large for float64 to hold exactly.
func normalizeNumber(n json.Number) any {
	s := n.String()
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		if i >= -maxExactFloatInt && i <= maxExactFloatInt {
			return float64(i)
		}
		return n
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return n
	}
	for _, c := range s {
		if c == '.' || c == 'e' || c == 'E' {
			return f
		}
	}
	// A plain integer literal that overflowed int64.
	return n
}

// normalizeTree applies normalizeNumber to every number in a decoded JSON tree.
func normalizeTree(v any) any {
	switch t := v.(type) {
	case json.Number:
		return normalizeNumber(t)
	case map[string]any:
		for k, e := range t {
			t[k] = normalizeTree(e)
		}
		return t
	case []any:
		for i, e := range t {
			t[i] = normalizeTree(e)
		}
		return t
	default:
		return v
	}
}

// restoreNumbers walks a decoded value and applies normalizeNumber to every
// json.Number held in an `any` slot (a Payload, Result, Tags, Metadata…).
// Typed numeric fields were already filled by the decoder and are untouched.
func restoreNumbers(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			restoreNumbers(v.Elem())
		}
	case reflect.Interface:
		if v.IsNil() {
			return
		}
		e := v.Elem()
		if n, ok := e.Interface().(json.Number); ok {
			if v.CanSet() {
				v.Set(reflect.ValueOf(normalizeNumber(n)))
			}
			return
		}
		switch e.Kind() {
		case reflect.Map, reflect.Slice:
			restoreNumbers(e)
		}
	case reflect.Map:
		if v.IsNil() || !holdsAny(v.Type().Elem()) {
			return
		}
		for _, key := range v.MapKeys() {
			cp := reflect.New(v.Type().Elem()).Elem()
			cp.Set(v.MapIndex(key))
			restoreNumbers(cp)
			v.SetMapIndex(key, cp)
		}
	case reflect.Slice, reflect.Array:
		if !holdsAny(v.Type().Elem()) {
			return
		}
		for i := 0; i < v.Len(); i++ {
			restoreNumbers(v.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if f := v.Field(i); f.CanSet() {
				restoreNumbers(f)
			}
		}
	}
}

// holdsAny reports whether a container element type can contain an `any` slot,
// so the walk skips []string, []int, map[string]string and similar.
func holdsAny(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Interface, reflect.Map, reflect.Slice, reflect.Array, reflect.Pointer, reflect.Struct:
		return true
	default:
		return false
	}
}
