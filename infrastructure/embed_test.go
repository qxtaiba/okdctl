package infrastructure

import (
	"bytes"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// isRuntimeArtifact reports whether name is written by terraform or okdctl
// at deploy time rather than being a source file. Such files may exist in a
// dev checkout that has run a deploy and must stay out of the embed.
func isRuntimeArtifact(name string) bool {
	switch {
	case strings.HasPrefix(name, "terraform.tfstate"),
		strings.HasPrefix(name, ".terraform.tfstate"):
		return true
	case strings.HasSuffix(name, ".tfvars"), strings.HasSuffix(name, ".tfvars.json"):
		return true
	case name == "override.tf", strings.HasSuffix(name, "_override.tf"):
		return true
	case name == "tfplan", strings.HasSuffix(name, ".bak"):
		return true
	}
	return false
}

func diskTerraformSources(t *testing.T) map[string][]byte {
	t.Helper()
	root, err := os.OpenRoot(".")
	if err != nil {
		t.Fatalf("open package dir: %v", err)
	}
	defer func() { _ = root.Close() }()
	rootFS := root.FS()

	files := map[string][]byte{}
	for _, sub := range []string{"terraform/modules", "terraform/environments"} {
		err := fs.WalkDir(rootFS, sub, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".terraform" {
					return fs.SkipDir
				}
				return nil
			}
			if isRuntimeArtifact(d.Name()) {
				return nil
			}
			data, readErr := fs.ReadFile(rootFS, path)
			if readErr != nil {
				return readErr
			}
			files[path] = data
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", sub, err)
		}
	}
	return files
}

// TestEmbeddedTerraformMatchesDisk guards TerraformFS against drift: every
// source file on disk under terraform/modules and terraform/environments
// must be embedded byte-for-byte, and nothing else may be embedded. Adding
// an HCL file (or any non-.tf asset) without extending the embed list in
// embed.go fails here.
func TestEmbeddedTerraformMatchesDisk(t *testing.T) {
	disk := diskTerraformSources(t)

	embedded := map[string][]byte{}
	err := fs.WalkDir(TerraformFS, "terraform", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, readErr := TerraformFS.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		embedded[path] = data
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded FS: %v", err)
	}

	for path, want := range disk {
		got, ok := embedded[path]
		if !ok {
			t.Errorf("%s exists on disk but is not embedded; add it to the go:embed list in embed.go", path)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: embedded content differs from disk (stale build cache? rebuild)", path)
		}
	}
	for path := range embedded {
		if _, ok := disk[path]; !ok {
			t.Errorf("%s is embedded but missing on disk; remove it from embed.go or restore the file", path)
		}
	}
}

// TestEmbeddedMasterKeepsPreventDestroy is a tripwire on the module's last
// line of defense against etcd-quorum loss: the master VM resource must keep
// lifecycle { prevent_destroy = true }. Deleting or flipping it (e.g. while
// debugging a destroy) would let a targeted destroy or a replace-folding plan
// silently wipe a control-plane VM.
func TestEmbeddedMasterKeepsPreventDestroy(t *testing.T) {
	data, err := TerraformFS.ReadFile("terraform/modules/proxmox-okd/main.tf")
	if err != nil {
		t.Fatalf("read embedded main.tf: %v", err)
	}
	src := string(data)

	start := strings.Index(src, `resource "proxmox_virtual_environment_vm" "master"`)
	if start == -1 {
		t.Fatal("master VM resource not found in embedded main.tf")
	}
	block := src[start:]
	if next := strings.Index(block[1:], "\nresource "); next != -1 {
		block = block[:next+1]
	}
	if !strings.Contains(block, "prevent_destroy = true") {
		t.Fatal("master VM resource lost prevent_destroy = true; restore the lifecycle guard before shipping")
	}
}
