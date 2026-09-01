package fit

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
)

// Record is one decoded FIT data message.
type Record struct {
	Num  uint16 // global message number
	Name string // profile name, or MESG_<num> when unknown

	// Timestamp is the record time in Unix seconds, resolved from field 253 or
	// the compressed timestamp header. Zero when the record carries no time.
	Timestamp int64

	// Fields holds decoded values keyed by profile field name. Scalars are
	// int64, float64 or string; repeated fields are slices of those.
	Fields map[string]any
	// ByNum holds the same values keyed by field number, for messages the
	// profile does not describe.
	ByNum map[uint8]any
}

// Int returns an integer field.
func (r *Record) Int(name string) (int64, bool) {
	v, ok := r.Fields[name]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case int64:
		return t, true
	case float64:
		return int64(t), true
	}
	return 0, false
}

// Float returns a floating point field.
func (r *Record) Float(name string) (float64, bool) {
	v, ok := r.Fields[name]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case int64:
		return float64(t), true
	}
	return 0, false
}

// Str returns a string field.
func (r *Record) Str(name string) (string, bool) {
	v, ok := r.Fields[name].(string)
	return v, ok
}

// Ints returns a repeated integer field, accepting a scalar as a single entry.
func (r *Record) Ints(name string) ([]int64, bool) {
	v, ok := r.Fields[name]
	if !ok {
		return nil, false
	}
	switch t := v.(type) {
	case []any:
		out := make([]int64, 0, len(t))
		for _, e := range t {
			switch n := e.(type) {
			case int64:
				out = append(out, n)
			case float64:
				out = append(out, int64(n))
			}
		}
		return out, true
	case int64:
		return []int64{t}, true
	case float64:
		return []int64{int64(t)}, true
	}
	return nil, false
}

// File is a decoded FIT file.
type File struct {
	ProtocolVersion byte
	ProfileVersion  uint16
	Records         []Record
}

// Records of a given global message number.
func (f *File) Of(num uint16) []Record {
	var out []Record
	for i := range f.Records {
		if f.Records[i].Num == num {
			out = append(out, f.Records[i])
		}
	}
	return out
}

// ErrBadHeader marks a stream that is not a FIT file.
var ErrBadHeader = errors.New("fit: bad header")

type fieldDef struct {
	num      uint8
	size     int
	base     BaseType
	profile  *ProfileField
	devScale float64
	devName  string
}

type recordDef struct {
	globalNum uint16
	bigEndian bool
	fields    []fieldDef
	devFields []fieldDef
	profile   *ProfileMessage
}

// devDescriptor mirrors a FIELD_DESCRIPTION record.
type devDescriptor struct {
	name   string
	base   BaseType
	scale  float64
	offset float64
}

// Decode parses a complete FIT file.
func Decode(data []byte) (*File, error) {
	if len(data) < 14 {
		return nil, fmt.Errorf("%w: %d bytes", ErrBadHeader, len(data))
	}
	headerSize := int(data[0])
	if headerSize < 12 || headerSize > len(data) {
		return nil, fmt.Errorf("%w: header size %d", ErrBadHeader, headerSize)
	}
	if string(data[8:12]) != ".FIT" {
		return nil, fmt.Errorf("%w: missing .FIT magic", ErrBadHeader)
	}
	dataSize := int(binary.LittleEndian.Uint32(data[4:8]))
	end := headerSize + dataSize
	if end > len(data) {
		// Trailing bytes may be missing on a truncated download; decode what
		// is present rather than throwing the whole file away.
		end = len(data)
	}

	f := &File{
		ProtocolVersion: data[1],
		ProfileVersion:  binary.LittleEndian.Uint16(data[2:4]),
	}

	defs := map[uint8]*recordDef{}
	devDescs := map[uint16]devDescriptor{} // (devIndex<<8 | fieldNum) -> descriptor
	var lastTimestamp int64

	pos := headerSize
	for pos < end {
		hdr := data[pos]
		pos++

		compressed := hdr&0x80 != 0
		var timeOffset = -1
		var localType uint8
		isDef := false
		hasDev := false
		if compressed {
			localType = (hdr >> 5) & 0x03
			timeOffset = int(hdr & 0x1F)
		} else {
			isDef = hdr&0x40 != 0
			hasDev = hdr&0x20 != 0
			localType = hdr & 0x0F
		}

		if timeOffset >= 0 && lastTimestamp != 0 {
			ref := lastTimestamp - GarminEpoch
			if int64(timeOffset) >= ref&0x1F {
				ref = (ref &^ 0x1F) + int64(timeOffset)
			} else {
				ref = (ref &^ 0x1F) + int64(timeOffset) + 0x20
			}
			lastTimestamp = ref + GarminEpoch
		}

		if isDef {
			def, n, err := parseDefinition(data[pos:end], hasDev, devDescs)
			if err != nil {
				return f, fmt.Errorf("fit: definition at %d: %w", pos, err)
			}
			pos += n
			defs[localType] = def
			continue
		}

		def := defs[localType]
		if def == nil {
			// Without a definition the record length is unknown, so the rest
			// of the file cannot be walked.
			return f, fmt.Errorf("fit: data record for undefined local type %d at %d", localType, pos)
		}
		rec, n, err := parseData(data[pos:end], def, devDescs)
		if err != nil {
			return f, fmt.Errorf("fit: data at %d: %w", pos, err)
		}
		pos += n

		if rec.Timestamp != 0 {
			lastTimestamp = rec.Timestamp
		} else if lastTimestamp != 0 {
			rec.Timestamp = lastTimestamp
		}
		if def.globalNum == MsgFieldDescription {
			registerDevField(&rec, devDescs)
		}
		f.Records = append(f.Records, rec)
	}
	return f, nil
}

func parseDefinition(b []byte, hasDev bool, devDescs map[uint16]devDescriptor) (*recordDef, int, error) {
	if len(b) < 5 {
		return nil, 0, errors.New("truncated")
	}
	def := &recordDef{bigEndian: b[1] == 0x01}
	order := binary.ByteOrder(binary.LittleEndian)
	if def.bigEndian {
		order = binary.BigEndian
	}
	def.globalNum = order.Uint16(b[2:4])
	numFields := int(b[4])
	pos := 5
	if len(b) < pos+numFields*3 {
		return nil, 0, errors.New("truncated field list")
	}
	def.profile = Message(def.globalNum)
	for range numFields {
		fd := fieldDef{
			num:  b[pos],
			size: int(b[pos+1]),
			base: LookupBaseType(b[pos+2]),
		}
		fd.profile = def.profile.Field(fd.num)
		// COROS style devices declare wider types than they emit; upstream
		// narrows to UINT8 when the wire size is a single byte.
		if fd.size == 1 && fd.base.Size > 1 && !fd.base.Float {
			fd.base = LookupBaseType(0x02)
		}
		def.fields = append(def.fields, fd)
		pos += 3
	}
	if hasDev {
		if len(b) < pos+1 {
			return nil, 0, errors.New("truncated dev field count")
		}
		numDev := int(b[pos])
		pos++
		if len(b) < pos+numDev*3 {
			return nil, 0, errors.New("truncated dev field list")
		}
		for range numDev {
			fieldNum := b[pos]
			size := int(b[pos+1])
			devIndex := b[pos+2]
			key := uint16(devIndex)<<8 | uint16(fieldNum)
			fd := fieldDef{num: fieldNum, size: size, base: LookupBaseType(0x0D), devScale: 1}
			if d, ok := devDescs[key]; ok {
				fd.base = d.base
				fd.devName = d.name
				fd.devScale = d.scale
			}
			def.devFields = append(def.devFields, fd)
			pos += 3
		}
	}
	return def, pos, nil
}

func parseData(b []byte, def *recordDef, devDescs map[uint16]devDescriptor) (Record, int, error) {
	rec := Record{
		Num:    def.globalNum,
		Fields: map[string]any{},
		ByNum:  map[uint8]any{},
	}
	if def.profile != nil {
		rec.Name = def.profile.Name
	} else {
		rec.Name = fmt.Sprintf("MESG_%d", def.globalNum)
	}

	order := binary.ByteOrder(binary.LittleEndian)
	if def.bigEndian {
		order = binary.BigEndian
	}

	pos := 0
	read := func(fd fieldDef) (any, error) {
		if pos+fd.size > len(b) {
			return nil, errors.New("truncated value")
		}
		raw := b[pos : pos+fd.size]
		pos += fd.size
		return decodeValue(raw, fd, order), nil
	}

	for _, fd := range def.fields {
		v, err := read(fd)
		if err != nil {
			return rec, pos, err
		}
		if v == nil {
			continue
		}
		rec.ByNum[fd.num] = v
		name := ""
		if fd.profile != nil {
			name = fd.profile.Name
		}
		if fd.num == 253 {
			if ts, ok := v.(int64); ok {
				rec.Timestamp = ts
			}
			if name == "" {
				name = "timestamp"
			}
		}
		if name != "" {
			rec.Fields[name] = v
		}
	}
	for _, fd := range def.devFields {
		v, err := read(fd)
		if err != nil {
			return rec, pos, err
		}
		if v == nil || fd.devName == "" {
			continue
		}
		rec.Fields[fd.devName] = v
	}
	return rec, pos, nil
}

// decodeValue turns raw bytes into a scaled Go value, or nil when the field
// holds the type's invalid marker.
func decodeValue(raw []byte, fd fieldDef, order binary.ByteOrder) any {
	bt := fd.base
	if bt.Str {
		s := string(raw)
		if i := strings.IndexByte(s, 0); i >= 0 {
			s = s[:i]
		}
		if s == "" {
			return nil
		}
		return s
	}
	if bt.Size <= 0 || len(raw) < bt.Size {
		return nil
	}

	count := len(raw) / bt.Size
	values := make([]any, 0, count)
	for i := range count {
		chunk := raw[i*bt.Size : (i+1)*bt.Size]
		v := decodeScalar(chunk, bt, fd, order)
		if v == nil {
			continue
		}
		values = append(values, v)
	}
	switch len(values) {
	case 0:
		return nil
	case 1:
		if count == 1 {
			return values[0]
		}
		return values
	default:
		return values
	}
}

func decodeScalar(chunk []byte, bt BaseType, fd fieldDef, order binary.ByteOrder) any {
	var u uint64
	switch bt.Size {
	case 1:
		u = uint64(chunk[0])
	case 2:
		u = uint64(order.Uint16(chunk))
	case 4:
		u = uint64(order.Uint32(chunk))
	case 8:
		u = order.Uint64(chunk)
	default:
		return nil
	}
	if u == bt.Invalid {
		return nil
	}

	if bt.Float {
		var f float64
		if bt.Size == 4 {
			f = float64(math.Float32frombits(uint32(u)))
		} else {
			f = math.Float64frombits(u)
		}
		if math.IsNaN(f) {
			return nil
		}
		return applyScale(f, fd)
	}

	var i int64
	if bt.Signed {
		switch bt.Size {
		case 1:
			i = int64(int8(u))
		case 2:
			i = int64(int16(u))
		case 4:
			i = int64(int32(u))
		case 8:
			i = int64(u)
		}
	} else {
		i = int64(u)
	}
	return applyScaleInt(i, fd)
}

func applyScale(f float64, fd fieldDef) any {
	scale, offset, kind := fieldScale(fd)
	if kind == "COORDINATE" {
		return semicircleToDegrees(int64(f))
	}
	v := f/scale - offset
	if kind == "TIMESTAMP" {
		return int64(v)
	}
	return v
}

func applyScaleInt(i int64, fd fieldDef) any {
	scale, offset, kind := fieldScale(fd)
	switch kind {
	case "COORDINATE":
		return semicircleToDegrees(i)
	case "TIMESTAMP":
		// Profile timestamps carry offset -GarminEpoch, giving Unix seconds.
		return i + GarminEpoch
	}
	if scale == 1 && offset == 0 {
		return i
	}
	return float64(i)/scale - offset
}

func fieldScale(fd fieldDef) (scale, offset float64, kind string) {
	scale, offset = 1, 0
	if fd.devScale > 0 {
		scale = fd.devScale
	}
	if fd.profile == nil {
		return scale, offset, ""
	}
	if fd.profile.Scale != 0 {
		scale = fd.profile.Scale
	}
	offset = fd.profile.Offset
	kind = fd.profile.Type
	switch kind {
	case "HR_TIME_IN_ZONE":
		return 1000, 0, ""
	case "TIMESTAMP":
		return 1, 0, kind
	}
	return scale, offset, kind
}

// registerDevField records a FIELD_DESCRIPTION so later developer fields decode
// with the right name and type.
func registerDevField(rec *Record, devDescs map[uint16]devDescriptor) {
	idx, ok1 := rec.Int("developer_data_index")
	num, ok2 := rec.Int("field_definition_number")
	if !ok1 || !ok2 {
		return
	}
	name, _ := rec.Str("field_name")
	if name == "" {
		name = fmt.Sprintf("dev_%d_%d", idx, num)
	}
	baseID := byte(0x0D)
	if v, ok := rec.Int("fit_base_type_id"); ok {
		baseID = byte(v)
	}
	scale := 1.0
	if v, ok := rec.Float("scale"); ok && v != 0 {
		scale = v
	}
	offset := 0.0
	if v, ok := rec.Float("offset"); ok {
		offset = v
	}
	devDescs[uint16(idx)<<8|uint16(num)] = devDescriptor{
		name:   name,
		base:   LookupBaseType(baseID),
		scale:  scale,
		offset: offset,
	}
}

// VerifyCRC checks the trailing file checksum.
func VerifyCRC(data []byte) error {
	if len(data) < 14 {
		return ErrBadHeader
	}
	headerSize := int(data[0])
	dataSize := int(binary.LittleEndian.Uint32(data[4:8]))
	end := headerSize + dataSize
	if end+2 > len(data) {
		return fmt.Errorf("fit: file shorter than declared (%d + 2 > %d)", end, len(data))
	}
	want := binary.LittleEndian.Uint16(data[end : end+2])
	if got := CRC16(0, data[:end]); got != want {
		return fmt.Errorf("fit: crc mismatch got %04x want %04x", got, want)
	}
	return nil
}
