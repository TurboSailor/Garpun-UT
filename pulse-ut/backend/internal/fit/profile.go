// Package fit decodes and encodes Garmin FIT files. Message and field names,
// scales and units come from the same fit_profile.json the upstream Android
// code generator consumes, so the two stay in step.
package fit

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed fit_profile.json
var profileJSON []byte

// ProfileField describes one field of a global message.
type ProfileField struct {
	Num       uint8   `json:"num"`
	Name      string  `json:"name"`
	Base      string  `json:"base"`
	Type      string  `json:"type"`
	UOM       string  `json:"UOM"`
	Scale     float64 `json:"scale"`
	Offset    float64 `json:"offset"`
	ArrayLen  int     `json:"arrayLen"`
	StringLen int     `json:"stringLen"`
}

// ProfileMessage describes one global message.
type ProfileMessage struct {
	Num    uint16         `json:"num"`
	Name   string         `json:"name"`
	Fields []ProfileField `json:"fields"`

	byNum map[uint8]*ProfileField
}

// Field looks up a field definition by its number.
func (m *ProfileMessage) Field(num uint8) *ProfileField {
	if m == nil {
		return nil
	}
	return m.byNum[num]
}

// EnumEntry is one value of a FIT enumeration.
type EnumEntry struct {
	Num  int    `json:"num"`
	Name string `json:"name"`
}

// Enumeration is a named FIT enumeration.
type Enumeration struct {
	Name    string      `json:"name"`
	Entries []EnumEntry `json:"entries"`
}

type profileFile struct {
	Messages     []ProfileMessage `json:"messages"`
	Enumerations []Enumeration    `json:"enumerations"`
}

var (
	profileOnce sync.Once
	messages    map[uint16]*ProfileMessage
	byName      map[string]*ProfileMessage
	enums       map[string]map[int]string
)

func loadProfile() {
	profileOnce.Do(func() {
		var pf profileFile
		if err := json.Unmarshal(profileJSON, &pf); err != nil {
			panic(fmt.Sprintf("fit: bad embedded profile: %v", err))
		}
		messages = make(map[uint16]*ProfileMessage, len(pf.Messages))
		byName = make(map[string]*ProfileMessage, len(pf.Messages))
		for i := range pf.Messages {
			m := &pf.Messages[i]
			m.byNum = make(map[uint8]*ProfileField, len(m.Fields))
			for j := range m.Fields {
				f := &m.Fields[j]
				if f.Scale == 0 {
					f.Scale = 1
				}
				m.byNum[f.Num] = f
			}
			messages[m.Num] = m
			byName[m.Name] = m
		}
		enums = make(map[string]map[int]string, len(pf.Enumerations))
		for _, e := range pf.Enumerations {
			vals := make(map[int]string, len(e.Entries))
			for _, entry := range e.Entries {
				vals[entry.Num] = entry.Name
			}
			enums[e.Name] = vals
		}
	})
}

// Message returns the profile entry for a global message number.
func Message(num uint16) *ProfileMessage {
	loadProfile()
	return messages[num]
}

// MessageByName returns the profile entry for a global message name.
func MessageByName(name string) *ProfileMessage {
	loadProfile()
	return byName[strings.ToUpper(name)]
}

// EnumName resolves an enumeration value to its symbolic name.
func EnumName(enum string, value int) (string, bool) {
	loadProfile()
	vals, ok := enums[enum]
	if !ok {
		return "", false
	}
	name, ok := vals[value]
	return name, ok
}

// Global message numbers this port actually consumes or produces.
const (
	MsgFileID           uint16 = 0
	MsgDeviceSettings   uint16 = 2
	MsgUserProfile      uint16 = 3
	MsgSport            uint16 = 12
	MsgSession          uint16 = 18
	MsgLap              uint16 = 19
	MsgRecord           uint16 = 20
	MsgEvent            uint16 = 21
	MsgDeviceInfo       uint16 = 23
	MsgActivity         uint16 = 34
	MsgFileCreator      uint16 = 49
	MsgMonitoring       uint16 = 55
	MsgHRV              uint16 = 78
	MsgMonitoringInfo   uint16 = 103
	MsgDeviceStatus     uint16 = 104
	MsgWeather          uint16 = 128
	MsgWeatherAlert     uint16 = 129
	MsgFieldDescription uint16 = 206
	MsgMonitoringHRData uint16 = 211
	MsgSet              uint16 = 225
	MsgStressLevel      uint16 = 227
	MsgMaxMetData       uint16 = 229
	MsgSpo2             uint16 = 269
	MsgSleepDataInfo    uint16 = 273
	MsgSleepDataRaw     uint16 = 274
	MsgSleepStage       uint16 = 275
	MsgRespirationRate  uint16 = 297
	MsgHSABodyBattery   uint16 = 314
	MsgSleepStats       uint16 = 346
	MsgHRVSummary       uint16 = 370
	MsgHRVValue         uint16 = 371
	MsgTrainingLoad     uint16 = 378
	MsgSleepRestless    uint16 = 382
	MsgDailySleep       uint16 = 384
	MsgSleepDemand      uint16 = 410
	MsgSleepSummary     uint16 = 411
)
