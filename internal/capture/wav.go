package capture

import (
	"encoding/binary"
	"io"
	"os"
)

// writeWAVHeader writes a 44-byte RIFF/WAV header for signed 16-bit LE mono PCM.
// Pass dataSize=0 as a placeholder; call fixWAVHeader after recording completes.
func writeWAVHeader(w io.Writer, sampleRate, dataSize uint32) error {
	const (
		numChannels   = 1
		bitsPerSample = 16
		audioFormat   = 1 // PCM
	)

	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8

	h := struct {
		// RIFF header
		RiffID   [4]byte
		RiffSize uint32
		WaveID   [4]byte
		// fmt sub-chunk
		FmtID        [4]byte
		FmtSize      uint32
		AudioFormat  uint16
		NumChannels  uint16
		SampleRate   uint32
		ByteRate     uint32
		BlockAlign   uint16
		BitsPerSamp  uint16
		// data sub-chunk
		DataID   [4]byte
		DataSize uint32
	}{
		RiffID:       [4]byte{'R', 'I', 'F', 'F'},
		RiffSize:     36 + dataSize, // will be fixed later if dataSize==0
		WaveID:       [4]byte{'W', 'A', 'V', 'E'},
		FmtID:        [4]byte{'f', 'm', 't', ' '},
		FmtSize:      16,
		AudioFormat:  audioFormat,
		NumChannels:  numChannels,
		SampleRate:   sampleRate,
		ByteRate:     byteRate,
		BlockAlign:   uint16(blockAlign),
		BitsPerSamp:  bitsPerSample,
		DataID:       [4]byte{'d', 'a', 't', 'a'},
		DataSize:     dataSize,
	}

	return binary.Write(w, binary.LittleEndian, &h)
}

// fixWAVHeader seeks to the beginning of f and patches the RIFF chunk size
// (offset 4) and data sub-chunk size (offset 40) based on actual file size.
func fixWAVHeader(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}

	fileSize := info.Size()
	if fileSize < 44 {
		return nil // nothing to fix
	}

	dataSize := uint32(fileSize - 44)
	riffSize := uint32(fileSize - 8)

	// Patch RIFF chunk size at offset 4
	if _, err := f.Seek(4, io.SeekStart); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, riffSize); err != nil {
		return err
	}

	// Patch data sub-chunk size at offset 40
	if _, err := f.Seek(40, io.SeekStart); err != nil {
		return err
	}
	return binary.Write(f, binary.LittleEndian, dataSize)
}
