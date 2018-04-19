package flv

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
)

type File struct {
	file              *os.File
	name              string
	readOnly          bool
	size              int64
	headerBuf         []byte
	duration          float64
	lastTimestamp     uint32
	firstTimestampSet bool
	firstTimestamp    uint32
}

type TagHeader struct {
	TagType   byte
	DataSize  uint32
	Timestamp uint32
}

func CreateFile(name string) (flvFile *File, err error) {
	var file *os.File
	// Create file
	if file, err = os.Create(name); err != nil {
		return
	}
	// Write flv header
	if _, err = file.Write(HEADER_BYTES); err != nil {
		file.Close()
		return
	}

	// Sync to disk
	if err = file.Sync(); err != nil {
		file.Close()
		return
	}

	flvFile = &File{
		file:      file,
		name:      name,
		readOnly:  false,
		headerBuf: make([]byte, 11),
		duration:  0.0,
	}

	return
}

func OpenFile(name string) (flvFile *File, err error) {
	var file *os.File
	// Open file
	file, err = os.Open(name)
	if err != nil {
		return
	}

	var size int64
	if size, err = file.Seek(0, 2); err != nil {
		file.Close()
		return
	}
	if _, err = file.Seek(0, 0); err != nil {
		file.Close()
		return
	}

	flvFile = &File{
		file:      file,
		name:      name,
		readOnly:  true,
		size:      size,
		headerBuf: make([]byte, 11),
	}

	// Read flv header
	remain := HEADER_LEN
	flvHeader := make([]byte, remain)

	if _, err = io.ReadFull(file, flvHeader); err != nil {
		file.Close()
		return
	}
	if flvHeader[0] != 'F' ||
		flvHeader[1] != 'L' ||
		flvHeader[2] != 'V' {
		file.Close()
		return nil, errors.New("File format error")
	}

	return
}

func (flvFile *File) Close() {
	flvFile.file.Close()
}

// Data with audio header
func (flvFile *File) WriteAudioTag(data []byte, timestamp uint32) (err error) {
	return flvFile.WriteTag(data, AUDIO_TAG, timestamp)
}

// Data with video header
func (flvFile *File) WriteVideoTag(data []byte, timestamp uint32) (err error) {
	return flvFile.WriteTag(data, VIDEO_TAG, timestamp)
}

// Write tag
func (flvFile *File) WriteTag(data []byte, tagType byte, timestamp uint32) (err error) {
	if timestamp < flvFile.lastTimestamp {
		timestamp = flvFile.lastTimestamp
	} else {
		flvFile.lastTimestamp = timestamp
	}
	if !flvFile.firstTimestampSet {
		flvFile.firstTimestampSet = true
		flvFile.firstTimestamp = timestamp
	}
	timestamp -= flvFile.firstTimestamp
	duration := float64(timestamp) / 1000.0
	if flvFile.duration < duration {
		flvFile.duration = duration
	}
	binary.BigEndian.PutUint32(flvFile.headerBuf[3:7], timestamp)
	flvFile.headerBuf[7] = flvFile.headerBuf[3]
	binary.BigEndian.PutUint32(flvFile.headerBuf[:4], uint32(len(data)))
	flvFile.headerBuf[0] = tagType
	// Write data
	if _, err = flvFile.file.Write(flvFile.headerBuf); err != nil {
		return
	}

	//tmpBuf := make([]byte, 4)
	//// Write tag header
	//if _, err = flvFile.file.Write([]byte{tagType}); err != nil {
	//	return
	//}

	//// Write tag size
	//binary.BigEndian.PutUint32(tmpBuf, uint32(len(data)))
	//if _, err = flvFile.file.Write(tmpBuf[1:]); err != nil {
	//	return
	//}

	//// Write timestamp
	//binary.BigEndian.PutUint32(tmpBuf, timestamp)
	//if _, err = flvFile.file.Write(tmpBuf[1:]); err != nil {
	//	return
	//}
	//if _, err = flvFile.file.Write(tmpBuf[:1]); err != nil {
	//	return
	//}

	//// Write stream ID
	//if _, err = flvFile.file.Write([]byte{0, 0, 0}); err != nil {
	//	return
	//}

	// Write data
	if _, err = flvFile.file.Write(data); err != nil {
		return
	}

	// Write previous tag size
	if err = binary.Write(flvFile.file, binary.BigEndian, uint32(len(data)+11)); err != nil {
		return
	}

	// Sync to disk
	//if err = flvFile.file.Sync(); err != nil {
	//	return
	//}
	return
}

func (flvFile *File) SetDuration(duration float64) {
	flvFile.duration = duration
}

func (flvFile *File) Sync() (err error) {
	// Update duration on MetaData
	if _, err = flvFile.file.Seek(DURATION_OFFSET, 0); err != nil {
		return
	}
	if err = binary.Write(flvFile.file, binary.BigEndian, flvFile.duration); err != nil {
		return
	}
	if _, err = flvFile.file.Seek(0, 2); err != nil {
		return
	}

	err = flvFile.file.Sync()
	return
}
func (flvFile *File) Size() (size int64) {
	size = flvFile.size
	return
}

func (flvFile *File) ReadTag() (header *TagHeader, data []byte, err error) {
	tmpBuf := make([]byte, 4)
	header = &TagHeader{}
	// Read tag header
	if _, err = io.ReadFull(flvFile.file, tmpBuf[3:]); err != nil {
		return
	}
	header.TagType = tmpBuf[3]

	// Read tag size
	if _, err = io.ReadFull(flvFile.file, tmpBuf[1:]); err != nil {
		return
	}
	header.DataSize = uint32(tmpBuf[1])<<16 | uint32(tmpBuf[2])<<8 | uint32(tmpBuf[3])

	// Read timestamp
	if _, err = io.ReadFull(flvFile.file, tmpBuf); err != nil {
		return
	}
	header.Timestamp = uint32(tmpBuf[0])<<24 + uint32(tmpBuf[1])<<16 + uint32(tmpBuf[2])<<8 + uint32(tmpBuf[3])

	// Read stream ID
	if _, err = io.ReadFull(flvFile.file, tmpBuf[1:]); err != nil {
		return
	}

	// Read data
	data = make([]byte, header.DataSize)
	if _, err = io.ReadFull(flvFile.file, data); err != nil {
		return
	}

	// Read previous tag size
	if _, err = io.ReadFull(flvFile.file, tmpBuf); err != nil {
		return
	}

	return
}

func (flvFile *File) IsFinished() bool {
	pos, err := flvFile.file.Seek(0, 1)
	return (err != nil) || (pos >= flvFile.size)
}
func (flvFile *File) LoopBack() {
	flvFile.file.Seek(HEADER_LEN, 0)
}
func (flvFile *File) FilePath() string {
	return flvFile.name
}
