package gfdi

// crcTable is the Garmin/ANT nibble table (reflected 0xA001, i.e. CRC-16/ARC).
// The same routine computes GFDI frame checksums, running file-transfer
// checksums and FIT file checksums.
var crcTable = [16]uint16{
	0x0000, 0xCC01, 0xD801, 0x1400, 0xF001, 0x3C00, 0x2800, 0xE401,
	0xA001, 0x6C00, 0x7800, 0xB401, 0x5000, 0x9C01, 0x8801, 0x4400,
}

// CRC16 continues a running checksum over data. Pass 0 as initial for a fresh
// computation.
func CRC16(initial uint16, data []byte) uint16 {
	crc := initial
	for _, b := range data {
		crc = ((crc >> 4) & 0x0FFF) ^ crcTable[crc&0x0F] ^ crcTable[b&0x0F]
		crc = ((crc >> 4) & 0x0FFF) ^ crcTable[crc&0x0F] ^ crcTable[(b>>4)&0x0F]
	}
	return crc
}
