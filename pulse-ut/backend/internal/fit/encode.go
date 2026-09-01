package fit

import (
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

// Outgoing file header constants, matching the upstream generator
// (FitFile.java:57-62).
const (
	fileHeaderSize     = 14
	outProtocolVersion = 16
	outProfileVersion  = 21117

	// A record header carries the local message type in its low nibble.
	maxLocalTypes = 16

	headerDefinition = 0x40
	headerDevData    = 0x20
	headerCompressed = 0x80
)

// semicircleDegrees is one semicircle expressed in degrees; the inverse of the
// conversion the decoder applies.
const semicircleDegrees = 180.0 / 2147483648.0

// Builder assembles a FIT record stream. Definitions are emitted ahead of the
// data records that reference them: that is valid FIT and it is also the order
// the watch needs when the stream is split into GFDI FIT_DEFINITION (5011) and
// FIT_DATA (5012) messages.
type Builder struct {
	closed []byte // groups closed after all local types were handed out
	defs   []byte // definitions of the open group
	data   []byte // data records of the open group

	sigs [maxLocalTypes]string // definition body per local type
	used int
}

// NewBuilder returns an empty builder.
func NewBuilder() *Builder { return &Builder{} }

// Add appends one data message. Values are keyed by profile field name and are
// scaled, offset and range-checked according to the embedded profile. A nil
// value declares the field in the definition but writes the type's invalid
// marker, which is how the watch is told "field known, no data".
func (b *Builder) Add(msgName string, fields map[string]any) error {
	msg := MessageByName(msgName)
	if msg == nil {
		return fmt.Errorf("fit: unknown message %q", msgName)
	}
	if len(fields) == 0 {
		return fmt.Errorf("fit: %s: no fields", msg.Name)
	}
	if len(fields) > 255 {
		return fmt.Errorf("fit: %s: %d fields exceed the definition limit", msg.Name, len(fields))
	}

	nums := make([]uint8, 0, len(fields))
	values := make(map[uint8]any, len(fields))
	for name, v := range fields {
		pf := profileField(msg, name)
		if pf == nil {
			return fmt.Errorf("fit: %s has no field %q", msg.Name, name)
		}
		if _, dup := values[pf.Num]; dup {
			return fmt.Errorf("fit: %s: field %d given twice", msg.Name, pf.Num)
		}
		values[pf.Num] = v
		nums = append(nums, pf.Num)
	}
	// Ascending field number keeps the wire order stable despite Go's random
	// map iteration, and puts timestamp (253) last like Garmin's own files.
	slices.Sort(nums)

	def := make([]byte, 0, 5+3*len(nums))
	def = append(def, 0, 0) // reserved, architecture: little endian
	def = binary.LittleEndian.AppendUint16(def, msg.Num)
	def = append(def, byte(len(nums)))
	body := make([]byte, 0, 4*len(nums))
	for _, num := range nums {
		pf := msg.Field(num)
		bt := encodeBaseType(pf)
		raw, err := encodeField(pf, bt, values[num])
		if err != nil {
			return fmt.Errorf("fit: %s.%s: %w", msg.Name, pf.Name, err)
		}
		if len(raw) > 255 {
			return fmt.Errorf("fit: %s.%s: %d bytes exceed the field limit", msg.Name, pf.Name, len(raw))
		}
		def = append(def, num, byte(len(raw)), bt.ID)
		body = append(body, raw...)
	}

	local := b.slotFor(string(def), def)
	b.data = append(b.data, byte(local))
	b.data = append(b.data, body...)
	return nil
}

// slotFor returns the local message type serving this definition, emitting the
// definition record when the field set is new.
func (b *Builder) slotFor(sig string, def []byte) int {
	for i := range b.used {
		if b.sigs[i] == sig {
			return i
		}
	}
	if b.used == maxLocalTypes {
		// Local types cycle back to 0, so close the group first: otherwise the
		// reissued definition would land behind data records that still refer
		// to the previous meaning of the slot.
		b.closeGroup()
	}
	local := b.used
	b.used++
	b.sigs[local] = sig
	b.defs = append(b.defs, headerDefinition|byte(local))
	b.defs = append(b.defs, def...)
	return local
}

func (b *Builder) closeGroup() {
	b.closed = append(b.closed, b.defs...)
	b.closed = append(b.closed, b.data...)
	b.defs, b.data, b.used = nil, nil, 0
	clear(b.sigs[:])
}

// Records returns the definition+data record stream, without file header or
// CRC. This is what goes to the watch over GFDI.
func (b *Builder) Records() []byte {
	out := make([]byte, 0, len(b.closed)+len(b.defs)+len(b.data))
	out = append(out, b.closed...)
	out = append(out, b.defs...)
	return append(out, b.data...)
}

// File returns a complete FIT file: 14 byte header, the record stream and the
// trailing file CRC.
func (b *Builder) File() []byte {
	records := b.Records()
	out := make([]byte, fileHeaderSize, fileHeaderSize+len(records)+2)
	out[0] = fileHeaderSize
	out[1] = outProtocolVersion
	binary.LittleEndian.PutUint16(out[2:4], outProfileVersion)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(records)))
	copy(out[8:12], ".FIT")
	binary.LittleEndian.PutUint16(out[12:14], CRC16(0, out[:12]))
	out = append(out, records...)
	// The file CRC covers the header too, which is what VerifyCRC checks.
	return binary.LittleEndian.AppendUint16(out, CRC16(0, out))
}

// SplitRecords separates the leading definition records of a Builder stream
// from the data records. The GFDI transport carries them in different
// messages: FIT_DEFINITION (5011) first, then FIT_DATA (5012).
func SplitRecords(blob []byte) (definitions, data []byte) {
	pos := 0
	for pos < len(blob) {
		hdr := blob[pos]
		if hdr&headerCompressed != 0 || hdr&headerDefinition == 0 {
			break
		}
		if pos+6 > len(blob) {
			return blob, nil
		}
		next := pos + 6 + 3*int(blob[pos+5])
		if hdr&headerDevData != 0 || next > len(blob) {
			return blob, nil
		}
		pos = next
	}
	return blob[:pos], blob[pos:]
}

// fieldNameIndex caches per message name -> field lookups; the profile itself
// is only indexed by field number.
var fieldNameIndex sync.Map // uint16 -> map[string]*ProfileField

func profileField(m *ProfileMessage, name string) *ProfileField {
	idx, ok := fieldNameIndex.Load(m.Num)
	if !ok {
		built := make(map[string]*ProfileField, len(m.Fields))
		for i := range m.Fields {
			built[m.Fields[i].Name] = &m.Fields[i]
		}
		idx, _ = fieldNameIndex.LoadOrStore(m.Num, built)
	}
	byName := idx.(map[string]*ProfileField)
	if pf, ok := byName[name]; ok {
		return pf
	}
	return byName[strings.ToLower(name)]
}

func encodeBaseType(pf *ProfileField) BaseType {
	if bt, ok := BaseTypeByName(pf.Base); ok {
		return bt
	}
	return LookupBaseType(0x0D)
}

// encodeFieldScale mirrors fieldScale on the decoding side.
func encodeFieldScale(pf *ProfileField) (scale, offset float64, kind string) {
	scale, offset, kind = 1, pf.Offset, pf.Type
	if pf.Scale != 0 {
		scale = pf.Scale
	}
	switch kind {
	case "HR_TIME_IN_ZONE":
		return 1000, 0, ""
	case "TIMESTAMP":
		return 1, 0, kind
	}
	return scale, offset, kind
}

func encodeField(pf *ProfileField, bt BaseType, v any) ([]byte, error) {
	if bt.Str {
		return encodeString(v, pf.StringLen)
	}
	scale, offset, kind := encodeFieldScale(pf)
	count := valueCount(v)
	if pf.ArrayLen > count {
		count = pf.ArrayLen
	}
	out := make([]byte, 0, count*bt.Size)
	for i := range count {
		out = appendScalar(out, bt, scale, offset, kind, valueAt(v, i))
	}
	return out, nil
}

// encodeString writes UTF-8 truncated on a rune boundary plus a terminator,
// zero padded to the profile's declared length.
func encodeString(v any, declared int) ([]byte, error) {
	var s string
	switch t := v.(type) {
	case nil:
	case string:
		s = t
	case []byte:
		s = string(t)
	default:
		return nil, fmt.Errorf("expected string, got %T", v)
	}
	size := declared
	if size <= 0 {
		size = len(s) + 1
	}
	out := make([]byte, size)
	limit := size - 1
	n := 0
	for n < len(s) {
		_, w := utf8.DecodeRuneInString(s[n:])
		if n+w > limit {
			break
		}
		n += w
	}
	copy(out, s[:n])
	return out, nil
}

func valueCount(v any) int {
	switch t := v.(type) {
	case []any:
		return len(t)
	case []int64:
		return len(t)
	case []int:
		return len(t)
	case []float64:
		return len(t)
	case []byte:
		return len(t)
	}
	return 1
}

func valueAt(v any, i int) any {
	switch t := v.(type) {
	case []any:
		if i < len(t) {
			return t[i]
		}
	case []int64:
		if i < len(t) {
			return t[i]
		}
	case []int:
		if i < len(t) {
			return t[i]
		}
	case []float64:
		if i < len(t) {
			return t[i]
		}
	case []byte:
		if i < len(t) {
			return t[i]
		}
	default:
		if i == 0 {
			return v
		}
	}
	return nil
}

func appendScalar(dst []byte, bt BaseType, scale, offset float64, kind string, v any) []byte {
	u, ok := rawScalar(bt, scale, offset, kind, v)
	if !ok {
		u = bt.Invalid
	}
	switch bt.Size {
	case 2:
		return binary.LittleEndian.AppendUint16(dst, uint16(u))
	case 4:
		return binary.LittleEndian.AppendUint32(dst, uint32(u))
	case 8:
		return binary.LittleEndian.AppendUint64(dst, u)
	}
	return append(dst, byte(u))
}

// rawScalar turns a Go value into the field's wire representation, reporting
// false when the value is missing or out of range so the caller writes the
// invalid marker instead.
func rawScalar(bt BaseType, scale, offset float64, kind string, v any) (uint64, bool) {
	if bt.Float {
		f, ok := numeric(v)
		if !ok {
			return 0, false
		}
		x := (f + offset) * scale
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		if bt.Size == 4 {
			return uint64(math.Float32bits(float32(x))), true
		}
		return math.Float64bits(x), true
	}

	// Integer inputs that need no scaling stay exact.
	if i, ok := integer(v); ok {
		switch kind {
		case "TIMESTAMP":
			return fitInt(bt, i-GarminEpoch)
		case "COORDINATE":
		default:
			if scale == 1 && offset == 0 {
				return fitInt(bt, i)
			}
		}
	}

	f, ok := numeric(v)
	if !ok {
		return 0, false
	}
	var raw float64
	switch kind {
	case "TIMESTAMP":
		raw = f - float64(GarminEpoch)
	case "COORDINATE":
		raw = f / semicircleDegrees
	default:
		raw = (f + offset) * scale
	}
	raw = math.Round(raw)
	if math.IsNaN(raw) || raw < math.MinInt64 || raw > math.MaxInt64 {
		return 0, false
	}
	return fitInt(bt, int64(raw))
}

// fitInt range-checks a raw integer against the base type.
func fitInt(bt BaseType, raw int64) (uint64, bool) {
	bits := uint(bt.Size) * 8
	if bits == 0 || bits > 64 {
		return 0, false
	}
	if bt.Signed {
		if bits < 64 {
			lo := int64(-1) << (bits - 1)
			hi := int64(1)<<(bits-1) - 1
			if raw < lo || raw > hi {
				return 0, false
			}
		}
	} else {
		if raw < 0 {
			return 0, false
		}
		if bits < 64 && uint64(raw) > (uint64(1)<<bits)-1 {
			return 0, false
		}
	}
	u := uint64(raw)
	if bits < 64 {
		u &= (uint64(1) << bits) - 1
	}
	if u == bt.Invalid {
		return 0, false
	}
	return u, true
}

func integer(v any) (int64, bool) {
	switch t := v.(type) {
	case int:
		return int64(t), true
	case int8:
		return int64(t), true
	case int16:
		return int64(t), true
	case int32:
		return int64(t), true
	case int64:
		return t, true
	case uint:
		if t <= math.MaxInt64 {
			return int64(t), true
		}
	case uint8:
		return int64(t), true
	case uint16:
		return int64(t), true
	case uint32:
		return int64(t), true
	case uint64:
		if t <= math.MaxInt64 {
			return int64(t), true
		}
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func numeric(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	}
	i, ok := integer(v)
	return float64(i), ok
}
