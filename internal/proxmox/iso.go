package proxmox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type volumeRow struct {
	VolID string `json:"volid"`
	Size  num    `json:"size"`
	CTime num    `json:"ctime"`
}

func (d *driver) ListISOs(ctx context.Context, node, storage string) ([]Volume, error) {
	var rows []volumeRow
	if err := d.get(ctx, "/nodes/"+node+"/storage/"+storage+"/content?content=iso", &rows); err != nil {
		return nil, err
	}
	volumes := make([]Volume, 0, len(rows))
	for _, r := range rows {
		volumes = append(volumes, Volume{
			VolID: r.VolID,
			Size:  int64(r.Size),                //nolint:gosec // G115: volume sizes fit int64
			CTime: time.Unix(int64(r.CTime), 0), //nolint:gosec // G115: unix seconds fit int64
		})
	}
	return volumes, nil
}

func (d *driver) DeleteVolume(ctx context.Context, node, storage, volid string) (Task, error) {
	return d.task(ctx, http.MethodDelete, node, "/nodes/"+node+"/storage/"+storage+"/content/"+volid, nil)
}

// UploadISO streams path to the node's storage without buffering the file;
// the request bypasses go-proxmox because its uploader ranges a map and so
// emits the multipart fields in random order (luthermonson/go-proxmox#352).
func (d *driver) UploadISO(ctx context.Context, node, storage, path, sha256 string) (Task, error) {
	file, err := os.Open(path)
	if err != nil {
		return Task{}, fmt.Errorf("upload iso: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return Task{}, fmt.Errorf("upload iso: %w", err)
	}

	head, tail, contentType, err := isoMultipart(filepath.Base(path), sha256)
	if err != nil {
		return Task{}, fmt.Errorf("upload iso: %w", err)
	}
	url := d.baseURL + "/nodes/" + node + "/storage/" + storage + "/upload"
	body := io.MultiReader(bytes.NewReader(head), file, bytes.NewReader(tail))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return Task{}, fmt.Errorf("upload iso: %w", err)
	}
	// pveproxy rejects an upload without Content-Length, so the body is sized
	// up front rather than sent chunked.
	req.ContentLength = int64(len(head)) + info.Size() + int64(len(tail))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", d.auth)

	res, err := d.httpClient.Do(req)
	if err != nil {
		return Task{}, fmt.Errorf("upload iso: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return Task{}, fmt.Errorf("upload iso: %w", ErrPermission)
	case res.StatusCode >= http.StatusMultipleChoices:
		return Task{}, fmt.Errorf("upload iso: %s", res.Status)
	}
	var envelope struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		return Task{}, fmt.Errorf("upload iso: decode reply: %w", err)
	}
	return Task{Node: node, UPID: envelope.Data}, nil
}

// isoMultipart renders the text parts and file-part header (head) and the
// closing boundary (tail) so the file itself can be streamed between them.
// PVE reads content, checksum-algorithm and checksum before the file part.
func isoMultipart(filename, sha256 string) (head, tail []byte, contentType string, err error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fields := [][2]string{{"content", "iso"}, {"checksum-algorithm", "sha256"}, {"checksum", sha256}}
	for _, kv := range fields {
		if err := w.WriteField(kv[0], kv[1]); err != nil {
			return nil, nil, "", err
		}
	}
	if _, err := w.CreateFormFile("filename", filename); err != nil {
		return nil, nil, "", err
	}
	split := buf.Len()
	if err := w.Close(); err != nil {
		return nil, nil, "", err
	}
	all := buf.Bytes()
	return all[:split], all[split:], w.FormDataContentType(), nil
}
