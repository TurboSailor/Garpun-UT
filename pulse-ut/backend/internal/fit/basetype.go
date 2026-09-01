package fit

// BaseType describes one FIT wire type.
type BaseType struct {
	ID      byte
	Name    string
	Size    int
	Signed  bool
	Float   bool
	Str     bool
	Invalid uint64
}

var baseTypes = map[byte]BaseType{
	0x00: {0x00, "ENUM", 1, false, false, false, 0xFF},
	0x01: {0x01, "SINT8", 1, true, false, false, 0x7F},
	0x02: {0x02, "UINT8", 1, false, false, false, 0xFF},
	0x83: {0x83, "SINT16", 2, true, false, false, 0x7FFF},
	0x84: {0x84, "UINT16", 2, false, false, false, 0xFFFF},
	0x85: {0x85, "SINT32", 4, true, false, false, 0x7FFFFFFF},
	0x86: {0x86, "UINT32", 4, false, false, false, 0xFFFFFFFF},
	0x07: {0x07, "STRING", 1, false, false, true, 0x00},
	0x88: {0x88, "FLOAT32", 4, false, true, false, 0xFFFFFFFF},
	0x89: {0x89, "FLOAT64", 8, false, true, false, 0xFFFFFFFFFFFFFFFF},
	0x0A: {0x0A, "UINT8Z", 1, false, false, false, 0x00},
	0x8B: {0x8B, "UINT16Z", 2, false, false, false, 0x00},
	0x8C: {0x8C, "UINT32Z", 4, false, false, false, 0x00},
	0x0D: {0x0D, "BYTE", 1, false, false, false, 0xFF},
	0x8E: {0x8E, "SINT64", 8, true, false, false, 0x7FFFFFFFFFFFFFFF},
	0x8F: {0x8F, "UINT64", 8, false, false, false, 0xFFFFFFFFFFFFFFFF},
	0x90: {0x90, "UINT64Z", 8, false, false, false, 0x00},
}

var baseTypesByName = func() map[string]BaseType {
	m := make(map[string]BaseType, len(baseTypes))
	for _, bt := range baseTypes {
		m[bt.Name] = bt
	}
	return m
}()

// LookupBaseType resolves a wire type id, falling back to BYTE like upstream.
func LookupBaseType(id byte) BaseType {
	if bt, ok := baseTypes[id]; ok {
		return bt
	}
	return baseTypes[0x0D]
}

// BaseTypeByName resolves a profile base type name.
func BaseTypeByName(name string) (BaseType, bool) {
	bt, ok := baseTypesByName[name]
	return bt, ok
}

// GarminEpoch is the FIT epoch in Unix seconds (1989-12-31T00:00:00Z).
const GarminEpoch int64 = 631065600

// semicircleToDegrees converts a semicircle coordinate to degrees.
func semicircleToDegrees(v int64) float64 {
	return float64(v) * (180.0 / 2147483648.0)
}
