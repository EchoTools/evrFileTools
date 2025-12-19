package tool

import (
	"bytes"
	"os"
	"testing"
)

func TestManifestParseHeader(t *testing.T) {
	t.Run("Valid Compressed Header", func(t *testing.T) {

		testData := []byte{
			0x5a, 0x53, 0x54, 0x44, 0x10, 0x00, 0x00, 0x00,
			0x0a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x0a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x28, 0xb5, 0x2f, 0xfd, 0x00, 0x58,
			0x51, 0x00, 0x00, 0x28, 0xb5, 0x2f, 0xfd, 0x60,
			0x28, 0x0d, 0x1d, 0x2e, 0x00,
		}

		reader := bytes.NewReader(testData)

		// Call ParseCompressedHeader with the buffer's bytes
		data, err := ArchiveDecode(reader)
		if err != nil {
			t.Fatalf("Expected no error, but got: %v", err)
		}
		if data == nil {
			t.Fatal("Expected non-nil data, but got nil")
		}

		file, _ := os.CreateTemp("/tmp", "testfile")
		defer file.Close()

		// Write the data to a temporary file
		if _, err := file.Write(data); err != nil {
			t.Fatalf("Failed to write data to file: %v", err)
		}

		t.Errorf("data: %v, err: %v", data, err)
	})
}
func TestManifestUnmarshalBinary(t *testing.T) {
	t.Run("Unmarshal Valid Manifest", func(t *testing.T) {
		manifestFilePath := "/mnt/c/Users/User/source/repos/EchoRelay9/_local/newnakama/echovr-newnakama/_data/5932408047/rad15/win10/manifests/2b47aab238f60515"

		manifest, err := ManifestReadFile(manifestFilePath)
		if err != nil {
			t.Fatalf("Failed to read manifest file: %v", err)
		}

		_ = manifest
	})

}
