package audio

import (
	"encoding/binary"
	"testing"
)

// --- DetectFormat tests ---

func TestDetectFormat_OGG(t *testing.T) {
	data := []byte{0x4F, 0x67, 0x67, 0x53, 0x00, 0x02}
	if got := DetectFormat(data); got != FormatOGG {
		t.Errorf("DetectFormat OGG = %v, want %v", got, FormatOGG)
	}
}

func TestDetectFormat_WAV(t *testing.T) {
	data := make([]byte, 12)
	copy(data[0:4], []byte{0x52, 0x49, 0x46, 0x46}) // "RIFF"
	copy(data[8:12], []byte{0x57, 0x41, 0x56, 0x45}) // "WAVE"
	if got := DetectFormat(data); got != FormatWAV {
		t.Errorf("DetectFormat WAV = %v, want %v", got, FormatWAV)
	}
}

func TestDetectFormat_WAV_NotWave(t *testing.T) {
	// RIFF but not WAVE (e.g. AVI)
	data := make([]byte, 12)
	copy(data[0:4], []byte{0x52, 0x49, 0x46, 0x46}) // "RIFF"
	copy(data[8:12], []byte{0x41, 0x56, 0x49, 0x20}) // "AVI "
	if got := DetectFormat(data); got != FormatUnknown {
		t.Errorf("DetectFormat RIFF/AVI = %v, want FormatUnknown", got)
	}
}

func TestDetectFormat_FLAC(t *testing.T) {
	data := []byte{0x66, 0x4C, 0x61, 0x43, 0x00}
	if got := DetectFormat(data); got != FormatFLAC {
		t.Errorf("DetectFormat FLAC = %v, want %v", got, FormatFLAC)
	}
}

func TestDetectFormat_MP3_ID3(t *testing.T) {
	data := []byte{0x49, 0x44, 0x33, 0x03, 0x00}
	if got := DetectFormat(data); got != FormatMP3 {
		t.Errorf("DetectFormat MP3/ID3 = %v, want %v", got, FormatMP3)
	}
}

func TestDetectFormat_MP3_Sync(t *testing.T) {
	for _, b := range []byte{0xFB, 0xF3, 0xF2} {
		data := []byte{0xFF, b, 0x00, 0x00}
		if got := DetectFormat(data); got != FormatMP3 {
			t.Errorf("DetectFormat MP3 sync (ff %02x) = %v, want %v", b, got, FormatMP3)
		}
	}
}

func TestDetectFormat_Unknown(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02, 0x03}
	if got := DetectFormat(data); got != FormatUnknown {
		t.Errorf("DetectFormat unknown = %v, want FormatUnknown", got)
	}
}

func TestDetectFormat_TooShort(t *testing.T) {
	if got := DetectFormat([]byte{0x4F, 0x67}); got != FormatUnknown {
		t.Errorf("DetectFormat short = %v, want FormatUnknown", got)
	}
}

// --- AudioFormat methods ---

func TestAudioFormat_String(t *testing.T) {
	tests := []struct {
		f    AudioFormat
		want string
	}{
		{FormatOGG, "OGG"},
		{FormatWAV, "WAV"},
		{FormatMP3, "MP3"},
		{FormatFLAC, "FLAC"},
		{FormatUnknown, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.f.String(); got != tt.want {
			t.Errorf("AudioFormat(%d).String() = %q, want %q", tt.f, got, tt.want)
		}
	}
}

func TestAudioFormat_Extension(t *testing.T) {
	tests := []struct {
		f    AudioFormat
		want string
	}{
		{FormatOGG, ".ogg"},
		{FormatWAV, ".wav"},
		{FormatMP3, ".mp3"},
		{FormatFLAC, ".flac"},
		{FormatUnknown, ""},
	}
	for _, tt := range tests {
		if got := tt.f.Extension(); got != tt.want {
			t.Errorf("AudioFormat(%d).Extension() = %q, want %q", tt.f, got, tt.want)
		}
	}
}

// --- ParseInfo tests ---

func TestParseInfo_Unknown(t *testing.T) {
	if got := ParseInfo([]byte{0x00, 0x01, 0x02, 0x03}); got != nil {
		t.Errorf("ParseInfo unknown = %v, want nil", got)
	}
}

func TestParseInfo_FLAC(t *testing.T) {
	data := []byte{0x66, 0x4C, 0x61, 0x43, 0x00}
	info := ParseInfo(data)
	if info == nil {
		t.Fatal("ParseInfo FLAC returned nil")
	}
	if info.Format != FormatFLAC {
		t.Errorf("ParseInfo FLAC format = %v, want FormatFLAC", info.Format)
	}
}

func TestParseInfo_MP3(t *testing.T) {
	data := []byte{0x49, 0x44, 0x33, 0x03, 0x00, 0x00}
	info := ParseInfo(data)
	if info == nil {
		t.Fatal("ParseInfo MP3 returned nil")
	}
	if info.Format != FormatMP3 {
		t.Errorf("ParseInfo MP3 format = %v, want FormatMP3", info.Format)
	}
}

// buildWAV constructs a minimal valid WAV file for testing.
func buildWAV(channels uint16, sampleRate uint32, bitDepth uint16, numSamples uint32) []byte {
	bytesPerSample := uint32(bitDepth) / 8
	dataSize := numSamples * bytesPerSample * uint32(channels)
	totalSize := 36 + dataSize // fmt chunk is 16 bytes + 8 header = 24; data chunk header 8; "WAVE" 4; "RIFF" header 8
	// RIFF chunk size = 4 ("WAVE") + 8 (fmt header) + 16 (fmt data) + 8 (data header) + dataSize
	riffSize := 4 + 8 + 16 + 8 + dataSize

	buf := make([]byte, 12+24+8+int(dataSize))
	// RIFF header
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], riffSize)
	copy(buf[8:12], "WAVE")
	// fmt chunk
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16) // chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(buf[22:24], channels)
	binary.LittleEndian.PutUint32(buf[24:28], sampleRate)
	byteRate := sampleRate * uint32(channels) * bytesPerSample
	binary.LittleEndian.PutUint32(buf[28:32], byteRate)
	blockAlign := uint16(channels) * uint16(bytesPerSample)
	binary.LittleEndian.PutUint16(buf[32:34], blockAlign)
	binary.LittleEndian.PutUint16(buf[34:36], bitDepth)
	// data chunk
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], dataSize)
	// (sample data is zero-initialized)
	_ = totalSize
	return buf
}

func TestParseInfo_WAV(t *testing.T) {
	// 2 channels, 44100 Hz, 16-bit, 44100 samples = 1 second
	data := buildWAV(2, 44100, 16, 44100)
	info := ParseInfo(data)
	if info == nil {
		t.Fatal("ParseInfo WAV returned nil")
	}
	if info.Format != FormatWAV {
		t.Errorf("WAV format = %v, want FormatWAV", info.Format)
	}
	if info.Channels != 2 {
		t.Errorf("WAV channels = %d, want 2", info.Channels)
	}
	if info.SampleRate != 44100 {
		t.Errorf("WAV sample rate = %d, want 44100", info.SampleRate)
	}
	if info.BitDepth != 16 {
		t.Errorf("WAV bit depth = %d, want 16", info.BitDepth)
	}
	// Duration should be approximately 1.0 second
	if info.Duration < 0.99 || info.Duration > 1.01 {
		t.Errorf("WAV duration = %f, want ~1.0", info.Duration)
	}
}

func TestParseInfo_WAV_Mono(t *testing.T) {
	// 1 channel, 22050 Hz, 8-bit, 22050 samples = 1 second
	data := buildWAV(1, 22050, 8, 22050)
	info := ParseInfo(data)
	if info == nil {
		t.Fatal("ParseInfo WAV mono returned nil")
	}
	if info.Channels != 1 {
		t.Errorf("WAV mono channels = %d, want 1", info.Channels)
	}
	if info.SampleRate != 22050 {
		t.Errorf("WAV mono sample rate = %d, want 22050", info.SampleRate)
	}
	if info.Duration < 0.99 || info.Duration > 1.01 {
		t.Errorf("WAV mono duration = %f, want ~1.0", info.Duration)
	}
}

// buildOGG constructs a minimal Ogg Vorbis identification header for testing.
func buildOGG(channels uint8, sampleRate uint32) []byte {
	// Vorbis ident header content: \x01vorbis + version(4) + channels(1) + sampleRate(4) + ...
	vorbisIdent := make([]byte, 30)
	vorbisIdent[0] = 0x01
	copy(vorbisIdent[1:7], "vorbis")
	binary.LittleEndian.PutUint32(vorbisIdent[7:11], 0) // version
	vorbisIdent[11] = channels
	binary.LittleEndian.PutUint32(vorbisIdent[12:16], sampleRate)

	// Build OGG page
	// page_segments = 1, lace_values = [len(vorbisIdent)]
	pageSegments := byte(1)
	header := make([]byte, 27+int(pageSegments))
	copy(header[0:4], "OggS")
	header[4] = 0                 // version
	header[5] = 0x02              // BOS (beginning of stream)
	// granule_position = 0 (bytes 6-13)
	// serial, page_sequence, checksum (bytes 14-25) = 0
	header[26] = pageSegments
	header[27] = byte(len(vorbisIdent))

	return append(header, vorbisIdent...)
}

func TestParseInfo_OGG(t *testing.T) {
	data := buildOGG(2, 48000)
	info := ParseInfo(data)
	if info == nil {
		t.Fatal("ParseInfo OGG returned nil")
	}
	if info.Format != FormatOGG {
		t.Errorf("OGG format = %v, want FormatOGG", info.Format)
	}
	if info.Channels != 2 {
		t.Errorf("OGG channels = %d, want 2", info.Channels)
	}
	if info.SampleRate != 48000 {
		t.Errorf("OGG sample rate = %d, want 48000", info.SampleRate)
	}
	if info.Duration != 0 {
		t.Errorf("OGG duration = %f, want 0 (not computable from header)", info.Duration)
	}
}

func TestParseInfo_OGG_Mono(t *testing.T) {
	data := buildOGG(1, 44100)
	info := ParseInfo(data)
	if info == nil {
		t.Fatal("ParseInfo OGG mono returned nil")
	}
	if info.Channels != 1 {
		t.Errorf("OGG mono channels = %d, want 1", info.Channels)
	}
	if info.SampleRate != 44100 {
		t.Errorf("OGG mono sample rate = %d, want 44100", info.SampleRate)
	}
}
