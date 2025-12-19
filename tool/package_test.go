package tool

import (
	"testing"
)

func TestPackageExtract(t *testing.T) {
	t.Run("Unmarshal Valid Manifest", func(t *testing.T) {
		manifestFilePath := "/mnt/c/Users/User/source/repos/EchoRelay9/_local/newnakama/echovr-newnakama/_data/5932408047/rad15/win10/manifests/2b47aab238f60515"

		manifest, err := ManifestReadFile(manifestFilePath)
		if err != nil {
			t.Fatalf("Failed to read manifest file: %v", err)
		}

		path := "/mnt/c/Users/User/source/repos/EchoRelay9/_local/newnakama/echovr-newnakama/_data/5932408047/rad15/win10/packages/2b47aab238f60515"
		resource, err := PackageOpenMultiPart(manifest, path)
		if err != nil {
			t.Fatalf("Failed to open package files: %v", err)
		}

		err = PackageExtract(resource, "/tmp/output", false)
		if err != nil {
			t.Fatalf("Failed to extract package files: %v", err)
		}
		_ = resource
	})

}
