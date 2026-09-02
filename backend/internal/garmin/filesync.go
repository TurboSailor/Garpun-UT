package garmin

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"time"

	"pulse/backend/internal/gfdi"
)

// DirectoryEntry is one 16 byte record of the watch file directory.
type DirectoryEntry struct {
	FileIndex     uint16 `json:"fileIndex"`
	FileDataType  uint8  `json:"fileDataType"`
	FileSubType   uint8  `json:"fileSubType"`
	FileNumber    uint16 `json:"fileNumber"`
	SpecificFlags uint8  `json:"specificFlags"`
	FileFlags     uint8  `json:"fileFlags"`
	FileSize      int32  `json:"fileSize"`
	Timestamp     int64  `json:"timestamp"` // Unix seconds, 0 when the watch gave none
}

// IsFit reports whether the entry is a FIT file (data type 128).
func (e DirectoryEntry) IsFit() bool { return e.FileDataType == 128 }

// Name renders a stable filename for exports and the raw file cache.
func (e DirectoryEntry) Name() string {
	ext := "bin"
	if e.IsFit() {
		ext = "fit"
	}
	label := FileTypeName(e.FileDataType, e.FileSubType)
	if e.Timestamp > 0 {
		t := time.Unix(e.Timestamp, 0).UTC()
		return fmt.Sprintf("%s_%s_%d.%s", label, t.Format("2006-01-02_15-04-05"), e.FileIndex, ext)
	}
	return fmt.Sprintf("%s_%d.%s", label, e.FileIndex, ext)
}

const (
	fileIndexDirectory = uint16(0)
	fileIndexDeviceXML = uint16(0xFFFD)
)

type downloadState struct {
	entry   DirectoryEntry
	buf     []byte
	crc     uint16
	total   int32
	started time.Time
}

type uploadState struct {
	fileType  FileType
	data      []byte
	fileIndex uint16
	offset    int32
	crc       uint16
	created   bool
}

// StartSync asks the watch for its file directory. The watch answers the
// FILTER acknowledgement, which is what actually kicks off the listing.
func (s *Session) StartSync() {
	s.mu.Lock()
	if s.syncing {
		s.mu.Unlock()
		return
	}
	s.syncing = true
	s.mu.Unlock()
	s.emit(EventSyncStarted, nil)
	s.send(gfdi.Filter())
}

func (s *Session) initiateDirectoryDownload() {
	s.mu.Lock()
	s.download = &downloadState{
		entry:   DirectoryEntry{FileIndex: fileIndexDirectory},
		started: time.Now(),
	}
	s.mu.Unlock()
	s.send(gfdi.DownloadRequest(fileIndexDirectory, 0, true, 0, 0))
}

func (s *Session) onDownloadStatus(st *gfdi.StatusMessage) {
	ds, err := st.DownloadStatus()
	if err != nil || !st.OK() || ds.Code != 0 {
		code := -1
		if ds != nil {
			code = int(ds.Code)
		}
		s.log.Warn("garmin: download refused", "status", st.Status, "code", code)
		s.mu.Lock()
		s.download = nil
		s.mu.Unlock()
		s.nextDownload()
		return
	}
	s.mu.Lock()
	if s.download == nil {
		s.mu.Unlock()
		return
	}
	s.download.total = ds.MaxFileSize
	s.download.buf = make([]byte, 0, ds.MaxFileSize)
	s.download.crc = 0
	entry := s.download.entry
	s.mu.Unlock()
	s.log.Info("garmin: downloading", "index", entry.FileIndex, "size", ds.MaxFileSize,
		"type", FileTypeName(entry.FileDataType, entry.FileSubType))
}

func (s *Session) onFileTransferData(f *gfdi.Frame) {
	ftd, err := gfdi.ParseFileTransferData(f.Payload)
	if err != nil {
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusDecodeError))
		return
	}

	s.mu.Lock()
	d := s.download
	if d == nil {
		s.mu.Unlock()
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusNAK))
		return
	}
	if int(ftd.DataOffset) != len(d.buf) {
		want := len(d.buf)
		s.mu.Unlock()
		s.log.Warn("garmin: offset mismatch", "got", ftd.DataOffset, "want", want)
		w := gfdi.NewWriter()
		w.U16(gfdi.MsgFileTransferData)
		w.U8(gfdi.StatusACK)
		w.U8(gfdi.TransferOffsetMismatch)
		w.I32(int32(want))
		s.send(gfdi.BuildFrame(gfdi.MsgResponse, w.Bytes()))
		return
	}
	if got := gfdi.CRC16(d.crc, ftd.Data); got != ftd.CRC {
		s.mu.Unlock()
		s.log.Warn("garmin: chunk crc mismatch", "got", got, "want", ftd.CRC)
		w := gfdi.NewWriter()
		w.U16(gfdi.MsgFileTransferData)
		w.U8(gfdi.StatusACK)
		w.U8(gfdi.TransferCRCMismatch)
		w.I32(ftd.DataOffset)
		s.send(gfdi.BuildFrame(gfdi.MsgResponse, w.Bytes()))
		return
	}
	d.crc = gfdi.CRC16(d.crc, ftd.Data)
	d.buf = append(d.buf, ftd.Data...)
	done := int32(len(d.buf)) >= d.total
	entry := d.entry
	progress := len(d.buf)
	total := int(d.total)
	s.mu.Unlock()

	s.send(gfdi.FileTransferDataAck(ftd.DataOffset + int32(len(ftd.Data))))
	s.emit(EventSyncProgress, map[string]any{
		"fileIndex": entry.FileIndex,
		"received":  progress,
		"total":     total,
	})

	if done {
		s.finishDownload()
	}
}

func (s *Session) finishDownload() {
	s.mu.Lock()
	d := s.download
	s.download = nil
	if d == nil {
		s.mu.Unlock()
		return
	}
	entry := d.entry
	data := d.buf
	s.mu.Unlock()

	if entry.FileIndex == fileIndexDirectory && entry.FileDataType == 0 {
		s.onDirectory(data)
		return
	}

	s.log.Info("garmin: file downloaded", "index", entry.FileIndex, "bytes", len(data),
		"type", FileTypeName(entry.FileDataType, entry.FileSubType))
	if s.Hooks.FileDownloaded != nil {
		if err := s.Hooks.FileDownloaded(entry, data); err != nil {
			s.log.Error("garmin: file handler failed", "err", err, "index", entry.FileIndex)
		}
	}
	s.emit(EventFileDownloaded, map[string]any{
		"entry": entry,
		"bytes": len(data),
	})
	s.archive(entry)
	s.nextDownload()
}

func (s *Session) archive(entry DirectoryEntry) {
	if s.opts.KeepFilesOnWatch || entry.FileIndex == fileIndexDeviceXML {
		return
	}
	s.mu.Lock()
	if s.archived[entry.FileIndex] {
		s.mu.Unlock()
		return
	}
	s.archived[entry.FileIndex] = true
	s.mu.Unlock()
	s.send(gfdi.SetFileFlags(entry.FileIndex, gfdi.FileFlagArchive))
}

func (s *Session) onDirectory(data []byte) {
	if len(data)%16 != 0 {
		s.log.Warn("garmin: directory size not a multiple of 16", "len", len(data))
	}
	var queue []DirectoryEntry
	for off := 0; off+16 <= len(data); off += 16 {
		rec := data[off : off+16]
		e := DirectoryEntry{
			FileIndex:     binary.LittleEndian.Uint16(rec[0:2]),
			FileDataType:  rec[2],
			FileSubType:   rec[3],
			FileNumber:    binary.LittleEndian.Uint16(rec[4:6]),
			SpecificFlags: rec[6],
			FileFlags:     rec[7],
			FileSize:      int32(binary.LittleEndian.Uint32(rec[8:12])),
		}
		if ts := int32(binary.LittleEndian.Uint32(rec[12:16])); ts != 0 {
			e.Timestamp = int64(ts) + gfdi.GarminEpoch
		}
		if e.FileIndex == 0 && e.FileDataType == 0 && e.FileSubType == 0 && e.FileSize == 0 {
			continue // all-zero padding record
		}
		s.log.Debug("garmin: directory entry", "index", e.FileIndex,
			"type", e.FileDataType, "subtype", e.FileSubType,
			"name", FileTypeName(e.FileDataType, e.FileSubType),
			"size", e.FileSize, "flags", e.FileFlags, "specific", e.SpecificFlags,
			"ts", e.Timestamp)
		ft, known := LookupFileType(e.FileDataType, e.FileSubType)
		if !known {
			s.log.Debug("garmin: unknown file type in directory",
				"type", e.FileDataType, "subtype", e.FileSubType, "index", e.FileIndex)
			if !s.opts.FetchUnknownFiles {
				continue
			}
		} else if !ft.Pull && !s.opts.FetchUnknownFiles {
			continue
		}
		queue = append(queue, e)
	}

	s.log.Info("garmin: directory received", "entries", len(data)/16, "queued", len(queue))
	// Files already stored locally only need the ARCHIVE flag so the watch
	// stops offering them; downloading them again wastes minutes of airtime.
	if s.Hooks.HaveFile != nil {
		kept := queue[:0]
		for _, e := range queue {
			if s.Hooks.HaveFile(e) {
				s.archive(e)
				continue
			}
			kept = append(kept, e)
		}
		queue = kept
	}

	s.mu.Lock()
	s.queue = queue
	s.mu.Unlock()
	s.emit(EventSyncProgress, map[string]any{"queued": len(queue)})
	s.nextDownload()
}

func (s *Session) nextDownload() {
	s.mu.Lock()
	if s.download != nil {
		s.mu.Unlock()
		return
	}
	if len(s.queue) == 0 {
		s.syncing = false
		s.mu.Unlock()
		s.send(gfdi.SystemEvent(gfdi.SysSyncComplete, 0))
		s.emit(EventSyncFinished, nil)
		return
	}
	entry := s.queue[0]
	s.queue = s.queue[1:]
	s.download = &downloadState{entry: entry, started: time.Now()}
	remaining := len(s.queue)
	s.mu.Unlock()

	s.emit(EventSyncProgress, map[string]any{"remaining": remaining, "fileIndex": entry.FileIndex})
	s.send(gfdi.DownloadRequest(entry.FileIndex, 0, true, 0, 0))
}

// DownloadFile queues one specific file index outside of a directory sync.
func (s *Session) DownloadFile(entry DirectoryEntry) {
	s.mu.Lock()
	s.queue = append(s.queue, entry)
	s.mu.Unlock()
	s.nextDownload()
}

// ------------------------------------------------------------------ upload --

// Upload pushes a file to the watch: CREATE_FILE, then UPLOAD_REQUEST, then
// FILE_TRANSFER_DATA chunks paced by the watch acknowledgements.
func (s *Session) Upload(ft FileType, data []byte) error {
	s.mu.Lock()
	if s.upload != nil {
		s.mu.Unlock()
		return fmt.Errorf("garmin: an upload is already in flight")
	}
	s.upload = &uploadState{fileType: ft, data: data}
	s.mu.Unlock()

	s.send(gfdi.CreateFile(int32(len(data)), ft.DataType, ft.SubType, int64(rand.Uint64()>>1)))
	return nil
}

func (s *Session) onCreateFileStatus(st *gfdi.StatusMessage) {
	cs, err := st.CreateFileStatus()
	s.mu.Lock()
	u := s.upload
	s.mu.Unlock()
	if u == nil {
		return
	}
	if err != nil || !st.OK() || cs.Code != 0 {
		code := -1
		if cs != nil {
			code = int(cs.Code)
		}
		s.log.Error("garmin: create file refused", "status", st.Status, "code", code)
		s.mu.Lock()
		s.upload = nil
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	u.fileIndex = cs.FileIndex
	u.created = true
	size := int32(len(u.data))
	s.mu.Unlock()
	s.send(gfdi.UploadRequest(cs.FileIndex, size, 0, 0))
}

func (s *Session) onUploadStatus(st *gfdi.StatusMessage) {
	us, err := st.UploadStatus()
	s.mu.Lock()
	u := s.upload
	s.mu.Unlock()
	if u == nil {
		return
	}
	if err != nil || !st.OK() || us.Code != 0 {
		s.log.Error("garmin: upload refused", "status", st.Status)
		s.mu.Lock()
		s.upload = nil
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	u.offset = us.DataOffset
	u.crc = us.CRCSeed
	s.mu.Unlock()
	s.sendUploadChunk()
}

func (s *Session) sendUploadChunk() {
	s.mu.Lock()
	u := s.upload
	if u == nil {
		s.mu.Unlock()
		return
	}
	if int(u.offset) >= len(u.data) {
		s.upload = nil
		s.mu.Unlock()
		s.send(gfdi.SystemEvent(gfdi.SysSyncComplete, 0))
		s.log.Info("garmin: upload complete")
		return
	}
	limit := s.maxPacketSize - 13
	if limit < 32 {
		limit = 32
	}
	end := int(u.offset) + limit
	if end > len(u.data) {
		end = len(u.data)
	}
	chunk := u.data[u.offset:end]
	u.crc = gfdi.CRC16(u.crc, chunk)
	frame := gfdi.FileTransferDataChunk(u.crc, u.offset, chunk)
	u.offset = int32(end)
	s.mu.Unlock()
	s.send(frame)
}

func (s *Session) onTransferStatus(st *gfdi.StatusMessage) {
	ts, err := st.TransferStatus()
	if err != nil {
		return
	}
	s.mu.Lock()
	u := s.upload
	s.mu.Unlock()
	if u == nil {
		return
	}
	switch ts.Code {
	case gfdi.TransferOK:
		s.sendUploadChunk()
	case gfdi.TransferResend, gfdi.TransferOffsetMismatch, gfdi.TransferCRCMismatch:
		s.log.Warn("garmin: upload chunk rejected, aborting", "code", ts.Code)
		s.mu.Lock()
		s.upload = nil
		s.mu.Unlock()
	case gfdi.TransferAbort:
		s.mu.Lock()
		s.upload = nil
		s.mu.Unlock()
	}
}
