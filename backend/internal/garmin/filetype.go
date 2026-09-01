package garmin

import "fmt"

// FileType names one (dataType, subType) pair from the watch directory. Pull
// marks the types a normal sync downloads; everything else is only fetched when
// the user explicitly asks for unknown files.
type FileType struct {
	DataType uint8
	SubType  uint8
	Name     string
	Pull     bool
}

// Well-known file types, mirroring FileType.FILETYPE upstream.
var fileTypes = []FileType{
	{0, 0, "DIRECTORY", false},
	{1, 0, "UNKNOWN_1_0", false},
	{8, 255, "DEVICE_XML", false},

	{128, 1, "DEVICE_1", false},
	{128, 2, "SETTINGS", false},
	{128, 3, "SPORTS", false},
	{128, 4, "ACTIVITY", true},
	{128, 5, "WORKOUTS", false},
	{128, 6, "COURSES", false},
	{128, 7, "SCHEDULES", false},
	{128, 8, "LOCATION", false},
	{128, 9, "WEIGHT", true},
	{128, 10, "TOTALS", false},
	{128, 11, "GOALS", false},
	{128, 12, "MAP", false},
	{128, 13, "DEBUG", false},
	{128, 14, "BLOOD_PRESSURE", false},
	{128, 15, "MONITOR_A", true},
	{128, 16, "FIT_TYPE_16", false},
	{128, 17, "FIT_TYPE_17", false},
	{128, 18, "FIT_TYPE_18", false},
	{128, 19, "FIT_TYPE_19", false},
	{128, 20, "SUMMARY", false},
	{128, 21, "GLUCOSE", false},
	{128, 22, "TRACKING_RECORDS", false},
	{128, 23, "TRACKING_EVENTS", false},
	{128, 24, "FIT_TYPE_24", false},
	{128, 25, "VECTOR", false},
	{128, 26, "FIT_TYPE_26", false},
	{128, 27, "FIT_TYPE_27", false},
	{128, 28, "MONITOR_DAILY", true},
	{128, 29, "RECORDS", false},
	{128, 30, "ALERT", false},
	{128, 31, "UNKNOWN_31", false},
	{128, 32, "MONITOR", true},
	{128, 33, "MLT_SPORT", false},
	{128, 34, "SEGMENTS", false},
	{128, 35, "SEGMENT_LIST", true},
	{128, 36, "GOLF", false},
	{128, 37, "CLUBS", false},
	{128, 38, "SCORE", true},
	{128, 39, "ADJUSTMENTS", false},
	{128, 40, "HMD", false},
	{128, 41, "CHANGELOG", true},
	{128, 42, "FIT_TYPE_42", false},
	{128, 43, "FIT_TYPE_43", false},
	{128, 44, "METRICS", true},
	{128, 45, "BAT_SWING", false},
	{128, 46, "ROSTER", false},
	{128, 47, "DIVE_PLAN", false},
	{128, 48, "HSA_DATA", false},
	{128, 49, "SLEEP", true},
	{128, 50, "SOFTWARE", false},
	{128, 51, "CHALLENGE_RESULT", false},
	{128, 52, "USER_BEHAVIOR_LOG", true},
	{128, 53, "CHRONO_ROUND", false},
	{128, 54, "CHRONO_SHOT", false},
	{128, 55, "CHRONO_SCORECARD", false},
	{128, 56, "PACE_BANDS", false},
	{128, 57, "SPORTS_BACKUP", true},
	{128, 58, "DEVICE_58", true},
	{128, 59, "MUSCLE_MAP", false},
	{128, 60, "RUNNING_TRACK", false},
	{128, 61, "ECG", true},
	{128, 62, "BENCHMARK", false},
	{128, 63, "POWER_GUIDANCE", false},
	{128, 64, "FIT_TYPE_64", false},
	{128, 65, "CALENDAR", false},
	{128, 66, "FIT_TYPE_66", true},
	{128, 67, "FIT_TYPE_67", false},
	{128, 68, "HRV_STATUS", true},
	{128, 70, "HSA", true},
	{128, 71, "COM_ACT", true},
	{128, 72, "FBT_BACKUP", true},
	{128, 73, "SKIN_TEMP", true},
	{128, 74, "FBT_PTD_BACKUP", true},
	{128, 75, "FIT_TYPE_75", false},
	{128, 76, "FIT_TYPE_76", false},
	{128, 77, "SCHEDULE", true},
	{128, 78, "FIT_TYPE_78", false},
	{128, 79, "SLP_DISR", true},
	{128, 80, "FIT_TYPE_80", false},
	{128, 81, "FIT_TYPE_81", false},
	{128, 82, "AREA_COURSES", true},
	{128, 83, "FIT_TYPE_83", false},
	{128, 85, "FIT_TYPE_85", false},
	{128, 86, "FIT_TYPE_86", false},
	{128, 87, "GEAR", false},
	{128, 88, "FIT_TYPE_88", false},
	{128, 89, "FIT_TYPE_89", false},
	{128, 90, "FIT_TYPE_90", false},
	{128, 91, "FIT_TYPE_91", false},
	{128, 92, "FIT_TYPE_92", false},
	{128, 93, "FIT_TYPE_93", false},
	{128, 94, "FIT_TYPE_94", false},
	{128, 95, "FIT_TYPE_95", false},
	{128, 96, "FIT_TYPE_96", false},
	{128, 97, "FIT_TYPE_97", false},
	{128, 98, "FIT_TYPE_98", false},
	{128, 99, "FIT_TYPE_99", false},

	{255, 4, "DOWNLOAD_COURSE", false},
	{255, 8, "UNKNOWN_255_008", false},
	{255, 17, "PRG", false},
	{255, 20, "UNKNOWN_255_020", false},
	{255, 22, "UNKNOWN_255_022", false},
	{255, 244, "IQ_ERROR_REPORTS", true},
	{255, 245, "ERROR_SHUTDOWN_REPORTS", true},
	{255, 246, "GOLF_SCORECARD", true},
	{255, 247, "ULF_LOGS", true},
	{255, 248, "KPI", true},
}

var fileTypeIndex = func() map[uint16]FileType {
	m := make(map[uint16]FileType, len(fileTypes))
	for _, ft := range fileTypes {
		m[uint16(ft.DataType)<<8|uint16(ft.SubType)] = ft
	}
	return m
}()

// LookupFileType resolves a directory entry type pair.
func LookupFileType(dataType, subType uint8) (FileType, bool) {
	ft, ok := fileTypeIndex[uint16(dataType)<<8|uint16(subType)]
	return ft, ok
}

// FileTypeName renders a human readable label, falling back to the numeric pair.
func FileTypeName(dataType, subType uint8) string {
	if ft, ok := LookupFileType(dataType, subType); ok {
		return ft.Name
	}
	return fmt.Sprintf("TYPE_%d_%d", dataType, subType)
}

// Named file types used when uploading to the watch.
var (
	FileTypeSettings  = FileType{128, 2, "SETTINGS", false}
	FileTypeWeight    = FileType{128, 9, "WEIGHT", true}
	FileTypeCourses   = FileType{128, 6, "COURSES", false}
	FileTypeWorkouts  = FileType{128, 5, "WORKOUTS", false}
	FileTypeSchedules = FileType{128, 7, "SCHEDULES", false}
	FileTypeGoals     = FileType{128, 11, "GOALS", false}
	FileTypeSports    = FileType{128, 3, "SPORTS", false}
	FileTypePRG       = FileType{255, 17, "PRG", false}
)
