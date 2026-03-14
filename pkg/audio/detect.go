package audio

// AudioFormat represents a detected audio format.
type AudioFormat int

const (
	FormatUnknown AudioFormat = iota
	FormatOGG
	FormatWAV
	FormatMP3
	FormatFLAC
)

// String returns the format name.
func (f AudioFormat) String() string {
	switch f {
	case FormatOGG:
		return "OGG"
	case FormatWAV:
		return "WAV"
	case FormatMP3:
		return "MP3"
	case FormatFLAC:
		return "FLAC"
	default:
		return "unknown"
	}
}

// Extension returns the file extension for the format.
func (f AudioFormat) Extension() string {
	switch f {
	case FormatOGG:
		return ".ogg"
	case FormatWAV:
		return ".wav"
	case FormatMP3:
		return ".mp3"
	case FormatFLAC:
		return ".flac"
	default:
		return ""
	}
}

// DetectFormat returns the audio format from the first few bytes of a file.
// Returns FormatUnknown if not a recognized audio format.
func DetectFormat(data []byte) AudioFormat {
	if len(data) < 4 {
		return FormatUnknown
	}

	// OGG: "OggS"
	if data[0] == 0x4F && data[1] == 0x67 && data[2] == 0x67 && data[3] == 0x53 {
		return FormatOGG
	}

	// WAV: "RIFF" at 0 and "WAVE" at 8
	if data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 {
		if len(data) >= 12 &&
			data[8] == 0x57 && data[9] == 0x41 && data[10] == 0x56 && data[11] == 0x45 {
			return FormatWAV
		}
	}

	// FLAC: "fLaC"
	if data[0] == 0x66 && data[1] == 0x4C && data[2] == 0x61 && data[3] == 0x43 {
		return FormatFLAC
	}

	// MP3: ID3 tag
	if data[0] == 0x49 && data[1] == 0x44 && data[2] == 0x33 {
		return FormatMP3
	}

	// MP3: MPEG sync bytes
	if data[0] == 0xFF && (data[1] == 0xFB || data[1] == 0xF3 || data[1] == 0xF2) {
		return FormatMP3
	}

	return FormatUnknown
}
