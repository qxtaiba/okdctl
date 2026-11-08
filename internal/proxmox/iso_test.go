package proxmox

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const fakeSHA256 = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

func writeISO(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "grappleberry-worker.iso")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write iso: %v", err)
	}
	return path
}

func TestUploadISO(t *testing.T) {
	t.Run("posts ordered multipart with explicit content length", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		path := writeISO(t, "iso-bytes")
		task, err := f.driver(t).UploadISO(t.Context(), "pve", "local", path, fakeSHA256)
		if err != nil {
			t.Fatalf("UploadISO(): %v", err)
		}
		if want := (Task{Node: "pve", UPID: upidUpload}); task != want {
			t.Fatalf("UploadISO() = %+v, want %+v", task, want)
		}

		call, mr := f.multipartBody("POST /nodes/pve/storage/local/upload")
		if call.ContentLength <= 0 || call.ContentLength != int64(len(call.Body)) {
			t.Fatalf("Content-Length = %d, want %d (explicit, not chunked)", call.ContentLength, len(call.Body))
		}
		var names []string
		values := map[string]string{}
		var filename string
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("parse multipart: %v", err)
			}
			data, _ := io.ReadAll(part)
			names = append(names, part.FormName())
			values[part.FormName()] = string(data)
			if part.FormName() == "filename" {
				filename = part.FileName()
			}
		}
		wantNames := []string{"content", "checksum-algorithm", "checksum", "filename"}
		if !reflect.DeepEqual(names, wantNames) {
			t.Fatalf("part order = %v, want %v", names, wantNames)
		}
		wantValues := map[string]string{
			"content": "iso", "checksum-algorithm": "sha256", "checksum": fakeSHA256, "filename": "iso-bytes",
		}
		if !reflect.DeepEqual(values, wantValues) {
			t.Fatalf("part values = %v, want %v", values, wantValues)
		}
		if filename != "grappleberry-worker.iso" {
			t.Fatalf("file part name = %q, want grappleberry-worker.iso", filename)
		}
	})

	t.Run("forbidden maps to ErrPermission", func(t *testing.T) {
		f := newFakeAPI(t, map[string]http.HandlerFunc{
			"/nodes/pve/storage/local/upload": jsonHandler(http.StatusForbidden, `{"data":null}`),
		})
		_, err := f.driver(t).UploadISO(t.Context(), "pve", "local", writeISO(t, "x"), fakeSHA256)
		if !errors.Is(err, ErrPermission) {
			t.Fatalf("UploadISO() error = %v, want ErrPermission", err)
		}
	})

	t.Run("pve failure carries its message", func(t *testing.T) {
		f := newFakeAPI(t, map[string]http.HandlerFunc{
			"/nodes/pve/storage/local/upload": pveErrorHandler("checksum mismatch"),
		})
		_, err := f.driver(t).UploadISO(t.Context(), "pve", "local", writeISO(t, "x"), fakeSHA256)
		if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("UploadISO() error = %v, want the pve message", err)
		}
	})

	t.Run("missing file never reaches the api", func(t *testing.T) {
		f := newFakeAPI(t, nil)
		_, err := f.driver(t).UploadISO(t.Context(), "pve", "local", filepath.Join(t.TempDir(), "nope.iso"), fakeSHA256)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("UploadISO() error = %v, want os.ErrNotExist", err)
		}
		if f.called("POST /nodes/pve/storage/local/upload") {
			t.Fatal("UploadISO() posted without a file")
		}
	})
}

func TestListISOs(t *testing.T) {
	got, err := newFakeAPI(t, nil).driver(t).ListISOs(t.Context(), "pve", "local")
	if err != nil {
		t.Fatalf("ListISOs(): %v", err)
	}
	want := []Volume{
		{VolID: "local:iso/grappleberry-worker.iso", Size: 1073741824, CTime: time.Unix(1730000000, 0)},
		{VolID: "local:iso/debian-12.iso", Size: 805306368, CTime: time.Unix(1720000000, 0)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListISOs() = %+v, want %+v", got, want)
	}
}

func TestDeleteVolume(t *testing.T) {
	f := newFakeAPI(t, nil)
	task, err := f.driver(t).DeleteVolume(t.Context(), "pve", "local", "local:iso/grappleberry-worker.iso")
	if err != nil {
		t.Fatalf("DeleteVolume(): %v", err)
	}
	if want := (Task{Node: "pve", UPID: upidImgDel}); task != want {
		t.Fatalf("DeleteVolume() = %+v, want %+v", task, want)
	}
}

func TestISOStoreMapsForbiddenToErrPermission(t *testing.T) {
	f := newFakeAPI(t, map[string]http.HandlerFunc{
		"/nodes/pve/storage/local/content?content=iso":                       jsonHandler(http.StatusForbidden, `{"data":null}`),
		"/nodes/pve/storage/local/content/local:iso/grappleberry-worker.iso": jsonHandler(http.StatusForbidden, `{"data":null}`),
	})
	d := f.driver(t)
	if _, err := d.ListISOs(t.Context(), "pve", "local"); !errors.Is(err, ErrPermission) {
		t.Fatalf("ListISOs() error = %v, want ErrPermission", err)
	}
	if _, err := d.DeleteVolume(t.Context(), "pve", "local", "local:iso/grappleberry-worker.iso"); !errors.Is(err, ErrPermission) {
		t.Fatalf("DeleteVolume() error = %v, want ErrPermission", err)
	}
}
