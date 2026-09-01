package garminhttp

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestEncodeGarminJSONSimpleObject(t *testing.T) {
	obj := &JSONObject{}
	obj.Set("a", json.Number("1"))

	got, err := EncodeGarminJSON(obj)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	want := []byte{
		0xab, 0xcd, 0xab, 0xcd, // string section magic
		0x00, 0x00, 0x00, 0x04, // string section length
		0x00, 0x02, 'a', 0x00, // "a" plus terminator, length includes it
		0xda, 0x7a, 0xda, 0x7a, // data section magic
		0x00, 0x00, 0x00, 0x0f, // data section length
		0x0b, 0x00, 0x00, 0x00, 0x01, // map with one entry
		0x03, 0x00, 0x00, 0x00, 0x00, // key: string at offset 0
		0x01, 0x00, 0x00, 0x00, 0x01, // value: sint32 1
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded bytes mismatch\n got %x\nwant %x", got, want)
	}
}

func TestEncodeGarminJSONNoStringSection(t *testing.T) {
	got, err := EncodeGarminJSON(json.Number("-2"))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := []byte{
		0xda, 0x7a, 0xda, 0x7a,
		0x00, 0x00, 0x00, 0x05,
		0x01, 0xff, 0xff, 0xff, 0xfe,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded bytes mismatch\n got %x\nwant %x", got, want)
	}
}

func TestEncodeGarminJSONBreadthFirstAndDedup(t *testing.T) {
	// {"k":{"k":"k"},"b":[true,null]} - "k" is shared by key and value, and
	// nested containers are emitted after every value of the outer object.
	inner := &JSONObject{}
	inner.Set("k", "k")
	outer := &JSONObject{}
	outer.Set("k", inner)
	outer.Set("b", []any{true, nil})

	got, err := EncodeGarminJSON(outer)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	want := []byte{
		0xab, 0xcd, 0xab, 0xcd,
		0x00, 0x00, 0x00, 0x08,
		0x00, 0x02, 'k', 0x00, // offset 0
		0x00, 0x02, 'b', 0x00, // offset 4
		0xda, 0x7a, 0xda, 0x7a,
		0x00, 0x00, 0x00, 0x1c,
		0x0b, 0x00, 0x00, 0x00, 0x02, // outer map, 2 entries
		0x03, 0x00, 0x00, 0x00, 0x00, // "k"
		0x0b, 0x00, 0x00, 0x00, 0x01, // inner map, 1 entry
		0x03, 0x00, 0x00, 0x00, 0x04, // "b"
		0x05, 0x00, 0x00, 0x00, 0x02, // array, 2 elements
		0x03, 0x00, 0x00, 0x00, 0x00, // inner key "k"
		0x03, 0x00, 0x00, 0x00, 0x00, // inner value "k"
		0x09, 0x01, // true
		0x00, // null
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded bytes mismatch\n got %x\nwant %x", got, want)
	}
}

func TestEncodeGarminJSONNumberKinds(t *testing.T) {
	tests := []struct {
		in   string
		want []byte
	}{
		// "1.0" carries no fraction, so it degrades to an integer.
		{"1.0", []byte{0x01, 0x00, 0x00, 0x00, 0x01}},
		{"1.5", []byte{0x02, 0x3f, 0xc0, 0x00, 0x00}},
		{"4294967296", []byte{0x0e, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}},
		{"0.1", []byte{0x0f, 0x3f, 0xb9, 0x99, 0x99, 0x99, 0x99, 0x99, 0x9a}},
	}
	for _, tt := range tests {
		got, err := EncodeGarminJSON(json.Number(tt.in))
		if err != nil {
			t.Fatalf("encode %s: %v", tt.in, err)
		}
		body := got[8:]
		if !bytes.Equal(body, tt.want) {
			t.Errorf("number %s: got %x, want %x", tt.in, body, tt.want)
		}
	}
}

func TestGarminJSONRoundTrip(t *testing.T) {
	src := []byte(`{"content-type":"application/json","list":[1,"two",false,null],"nested":{"x":2.5}}`)
	parsed, err := ParseJSON(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	encoded, err := EncodeGarminJSON(parsed)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeGarminJSON(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	obj, ok := decoded.(*JSONObject)
	if !ok {
		t.Fatalf("decoded root is %T, want *JSONObject", decoded)
	}
	if !reflect.DeepEqual(obj.Keys, []string{"content-type", "list", "nested"}) {
		t.Fatalf("key order lost: %v", obj.Keys)
	}
	if v, _ := obj.GetString("content-type"); v != "application/json" {
		t.Fatalf("content-type = %q", v)
	}
	list, _ := obj.Get("list")
	arr, ok := list.([]any)
	if !ok || len(arr) != 4 {
		t.Fatalf("list decoded as %#v", list)
	}
	if arr[1] != "two" || arr[2] != false || arr[3] != nil {
		t.Fatalf("list values wrong: %#v", arr)
	}
	nested, _ := obj.Get("nested")
	nestedObj, ok := nested.(*JSONObject)
	if !ok {
		t.Fatalf("nested decoded as %T", nested)
	}
	if v, _ := nestedObj.Get("x"); v != 2.5 {
		t.Fatalf("nested x = %#v", v)
	}
}

func TestDecodeGarminJSONRejectsGarbage(t *testing.T) {
	if _, err := DecodeGarminJSON([]byte("not a garmin json body")); err == nil {
		t.Fatal("expected an error for a bogus payload")
	}
}
