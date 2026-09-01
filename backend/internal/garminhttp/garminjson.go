package garminhttp

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
)

// Garmin's own binary JSON representation, used by the legacy webRequest /
// webResponse pair for headers and bodies. Big-endian, two sections: a string
// pool and a breadth-first stream of values that reference it by offset.

var (
	stringSectionMagic = [4]byte{0xab, 0xcd, 0xab, 0xcd}
	dataSectionMagic   = [4]byte{0xda, 0x7a, 0xda, 0x7a}
)

const (
	gjNull   = 0x00
	gjSInt32 = 0x01
	gjFloat  = 0x02
	gjString = 0x03
	gjArray  = 0x05
	gjBool   = 0x09
	gjMap    = 0x0b
	gjSInt64 = 0x0e
	gjDouble = 0x0f
)

// JSONObject is a JSON object that keeps its key order, which the encoder needs
// to produce byte-identical output for a given input document.
type JSONObject struct {
	Keys   []string
	Values []any
}

// Set appends or replaces a key, preserving first-insertion order.
func (o *JSONObject) Set(key string, value any) {
	for i, k := range o.Keys {
		if k == key {
			o.Values[i] = value
			return
		}
	}
	o.Keys = append(o.Keys, key)
	o.Values = append(o.Values, value)
}

// Get returns the value for key.
func (o *JSONObject) Get(key string) (any, bool) {
	for i, k := range o.Keys {
		if k == key {
			return o.Values[i], true
		}
	}
	return nil, false
}

// GetString returns a string-ish value for key, mirroring Gson's getAsString.
func (o *JSONObject) GetString(key string) (string, bool) {
	v, ok := o.Get(key)
	if !ok {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case json.Number:
		return t.String(), true
	case bool:
		if t {
			return "true", true
		}
		return "false", true
	default:
		return "", false
	}
}

// ParseJSON decodes a JSON document into the ordered representation understood
// by EncodeGarminJSON. Numbers stay as json.Number so integers do not degrade
// into floats.
func ParseJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	// Reject trailing garbage the same way json.Unmarshal would.
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("garminhttp: trailing data after json value")
	}
	return v, nil
}

func parseValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("garminhttp: parse json: %w", err)
	}
	return parseFromToken(dec, tok)
}

func parseFromToken(dec *json.Decoder, tok json.Token) (any, error) {
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := &JSONObject{}
			for {
				kt, err := dec.Token()
				if err != nil {
					return nil, fmt.Errorf("garminhttp: parse json object: %w", err)
				}
				if d, ok := kt.(json.Delim); ok && d == '}' {
					return obj, nil
				}
				key, ok := kt.(string)
				if !ok {
					return nil, fmt.Errorf("garminhttp: parse json: non-string object key")
				}
				val, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				obj.Set(key, val)
			}
		case '[':
			arr := []any{}
			for {
				et, err := dec.Token()
				if err != nil {
					return nil, fmt.Errorf("garminhttp: parse json array: %w", err)
				}
				if d, ok := et.(json.Delim); ok && d == ']' {
					return arr, nil
				}
				val, err := parseFromToken(dec, et)
				if err != nil {
					return nil, err
				}
				arr = append(arr, val)
			}
		}
		return nil, fmt.Errorf("garminhttp: parse json: unexpected delimiter %v", t)
	default:
		return tok, nil
	}
}

// EncodeGarminJSON serialises a value tree (*JSONObject, []any, string,
// json.Number, float64, int-ish, bool, nil) into Garmin's binary JSON.
func EncodeGarminJSON(root any) ([]byte, error) {
	strings := collectStrings(root)

	var pool bytes.Buffer
	offsets := make(map[string]uint32, len(strings))
	for _, s := range strings {
		offsets[s] = uint32(pool.Len())
		// Length covers the payload plus the null terminator.
		var hdr [2]byte
		binary.BigEndian.PutUint16(hdr[:], uint16(len(s)+1))
		pool.Write(hdr[:])
		pool.WriteString(s)
		pool.WriteByte(0)
	}

	var data bytes.Buffer
	if err := encodeBreadthFirst(root, &data, offsets); err != nil {
		return nil, err
	}

	out := bytes.NewBuffer(make([]byte, 0, pool.Len()+data.Len()+16))
	if pool.Len() > 0 {
		out.Write(stringSectionMagic[:])
		writeU32(out, uint32(pool.Len()))
		out.Write(pool.Bytes())
	}
	out.Write(dataSectionMagic[:])
	writeU32(out, uint32(data.Len()))
	out.Write(data.Bytes())
	return out.Bytes(), nil
}

// collectStrings walks containers breadth-first, recording object keys and
// string values in first-seen order. The order fixes the pool offsets, so it
// must match the reference implementation exactly.
func collectStrings(root any) []string {
	seen := make(map[string]struct{})
	var order []string
	add := func(s string) {
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		order = append(order, s)
	}

	queue := []any{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		switch v := cur.(type) {
		case *JSONObject:
			for i, k := range v.Keys {
				add(k)
				switch val := v.Values[i].(type) {
				case string:
					add(val)
				case *JSONObject:
					queue = append(queue, val)
				case []any:
					queue = append(queue, val)
				}
			}
		case []any:
			for _, e := range v {
				switch val := e.(type) {
				case string:
					add(val)
				case *JSONObject:
					queue = append(queue, val)
				case []any:
					queue = append(queue, val)
				}
			}
		case string:
			add(v)
		}
	}
	return order
}

func encodeBreadthFirst(root any, out *bytes.Buffer, offsets map[string]uint32) error {
	queue := []any{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		switch v := cur.(type) {
		case nil:
			out.WriteByte(gjNull)
		case bool:
			out.WriteByte(gjBool)
			if v {
				out.WriteByte(1)
			} else {
				out.WriteByte(0)
			}
		case string:
			off, ok := offsets[v]
			if !ok {
				return fmt.Errorf("garminhttp: string %q missing from pool", v)
			}
			out.WriteByte(gjString)
			writeU32(out, off)
		case json.Number:
			if err := encodeNumber(v.String(), out); err != nil {
				return err
			}
		case *JSONObject:
			out.WriteByte(gjMap)
			writeU32(out, uint32(len(v.Keys)))
			for i, k := range v.Keys {
				queue = append(queue, k, v.Values[i])
			}
		case []any:
			out.WriteByte(gjArray)
			writeU32(out, uint32(len(v)))
			queue = append(queue, v...)
		case int:
			encodeInt(int64(v), out)
		case int32:
			encodeInt(int64(v), out)
		case int64:
			encodeInt(v, out)
		case float32:
			out.WriteByte(gjFloat)
			writeU32(out, math.Float32bits(v))
		case float64:
			encodeFloat(v, out)
		default:
			return fmt.Errorf("garminhttp: unsupported garmin json type %T", v)
		}
	}
	return nil
}

// encodeNumber keeps the integer/float distinction of the original text, the
// way Gson's lazily parsed numbers do.
func encodeNumber(text string, out *bytes.Buffer) error {
	if i, err := strconv.ParseInt(text, 10, 64); err == nil {
		encodeInt(i, out)
		return nil
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("garminhttp: parse number %q: %w", text, err)
	}
	// "1.0" and "1e3" carry no fraction, so they go out as integers.
	if !math.IsInf(f, 0) && !math.IsNaN(f) && f == math.Trunc(f) &&
		f >= math.MinInt64 && f <= math.MaxInt64 {
		encodeInt(int64(f), out)
		return nil
	}
	encodeFloat(f, out)
	return nil
}

func encodeInt(v int64, out *bytes.Buffer) {
	if v >= math.MinInt32 && v <= math.MaxInt32 {
		out.WriteByte(gjSInt32)
		writeU32(out, uint32(int32(v)))
		return
	}
	out.WriteByte(gjSInt64)
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	out.Write(b[:])
}

func encodeFloat(v float64, out *bytes.Buffer) {
	if math.Abs(v) < math.MaxFloat32 && float64(float32(v)) == v {
		out.WriteByte(gjFloat)
		writeU32(out, math.Float32bits(float32(v)))
		return
	}
	out.WriteByte(gjDouble)
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], math.Float64bits(v))
	out.Write(b[:])
}

func writeU32(out *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	out.Write(b[:])
}

// DecodeGarminJSON parses Garmin's binary JSON back into the ordered value
// tree. Only needed for inbound webRequest headers.
func DecodeGarminJSON(raw []byte) (any, error) {
	if len(raw) < 9 {
		return nil, fmt.Errorf("garminhttp: garmin json too short (%d bytes)", len(raw))
	}
	r := &beReader{buf: raw}
	pool := make(map[uint32]string)

	magic, err := r.bytes(4)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(magic, stringSectionMagic[:]) {
		size, err := r.u32()
		if err != nil {
			return nil, err
		}
		start := r.pos
		end := start + int(size)
		if end > len(raw) {
			return nil, fmt.Errorf("garminhttp: garmin json string section overruns buffer")
		}
		for r.pos < end {
			off := uint32(r.pos - start)
			n, err := r.u16()
			if err != nil {
				return nil, err
			}
			if n == 0 {
				return nil, fmt.Errorf("garminhttp: garmin json zero-length string")
			}
			s, err := r.bytes(int(n))
			if err != nil {
				return nil, err
			}
			pool[off] = string(s[:n-1])
		}
		if magic, err = r.bytes(4); err != nil {
			return nil, err
		}
	}
	if !bytes.Equal(magic, dataSectionMagic[:]) {
		return nil, fmt.Errorf("garminhttp: garmin json data magic mismatch %x", magic)
	}
	if _, err := r.u32(); err != nil {
		return nil, err
	}

	root, kind, count, err := r.value(pool)
	if err != nil {
		return nil, err
	}
	// Containers are filled breadth-first, in the same order they were written.
	type pending struct {
		container any
		count     int
	}
	var queue []pending
	if kind == gjMap || kind == gjArray {
		queue = append(queue, pending{root, count})
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for range cur.count {
			var key string
			if _, isObj := cur.container.(*JSONObject); isObj {
				kv, kkind, _, err := r.value(pool)
				if err != nil {
					return nil, err
				}
				if kkind != gjString {
					return nil, fmt.Errorf("garminhttp: garmin json non-string map key")
				}
				key, _ = kv.(string)
			}
			val, vkind, vcount, err := r.value(pool)
			if err != nil {
				return nil, err
			}
			if vkind == gjMap || vkind == gjArray {
				queue = append(queue, pending{val, vcount})
			}
			switch c := cur.container.(type) {
			case *JSONObject:
				c.Set(key, val)
			case *jsonArray:
				c.items = append(c.items, val)
			}
		}
	}
	return normalizeArrays(root), nil
}

// jsonArray is a mutable array placeholder; the public tree uses []any.
type jsonArray struct {
	items []any
}

func normalizeArrays(v any) any {
	switch t := v.(type) {
	case *jsonArray:
		out := make([]any, len(t.items))
		for i, e := range t.items {
			out[i] = normalizeArrays(e)
		}
		return out
	case *JSONObject:
		for i, e := range t.Values {
			t.Values[i] = normalizeArrays(e)
		}
		return t
	default:
		return v
	}
}

// value reads one value header. Containers come back empty together with the
// number of children still to be read from the stream.
func (r *beReader) value(pool map[uint32]string) (any, byte, int, error) {
	t, err := r.u8()
	if err != nil {
		return nil, 0, 0, err
	}
	switch t {
	case gjNull:
		return nil, t, 0, nil
	case gjBool:
		b, err := r.u8()
		if err != nil {
			return nil, t, 0, err
		}
		return b != 0, t, 0, nil
	case gjSInt32:
		v, err := r.u32()
		if err != nil {
			return nil, t, 0, err
		}
		return json.Number(fmt.Sprint(int32(v))), t, 0, nil
	case gjSInt64:
		v, err := r.u64()
		if err != nil {
			return nil, t, 0, err
		}
		return json.Number(fmt.Sprint(int64(v))), t, 0, nil
	case gjFloat:
		v, err := r.u32()
		if err != nil {
			return nil, t, 0, err
		}
		return float64(math.Float32frombits(v)), t, 0, nil
	case gjDouble:
		v, err := r.u64()
		if err != nil {
			return nil, t, 0, err
		}
		return math.Float64frombits(v), t, 0, nil
	case gjString:
		off, err := r.u32()
		if err != nil {
			return nil, t, 0, err
		}
		s, ok := pool[off]
		if !ok {
			return nil, t, 0, fmt.Errorf("garminhttp: garmin json string offset %d not in pool", off)
		}
		return s, t, 0, nil
	case gjMap:
		n, err := r.u32()
		if err != nil {
			return nil, t, 0, err
		}
		return &JSONObject{}, t, int(n), nil
	case gjArray:
		n, err := r.u32()
		if err != nil {
			return nil, t, 0, err
		}
		return &jsonArray{items: make([]any, 0, n)}, t, int(n), nil
	default:
		return nil, t, 0, fmt.Errorf("garminhttp: garmin json unknown type 0x%02x", t)
	}
}

type beReader struct {
	buf []byte
	pos int
}

func (r *beReader) bytes(n int) ([]byte, error) {
	if r.pos+n > len(r.buf) {
		return nil, fmt.Errorf("garminhttp: garmin json truncated")
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

func (r *beReader) u8() (byte, error) {
	b, err := r.bytes(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (r *beReader) u16() (uint16, error) {
	b, err := r.bytes(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (r *beReader) u32() (uint32, error) {
	b, err := r.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (r *beReader) u64() (uint64, error) {
	b, err := r.bytes(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b), nil
}
