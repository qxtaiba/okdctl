package proxmox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type volumeRow struct {
	VolID string `json:"volid"`
	Size  num    `json:"size"`
	CTime num    `json:"ctime"`
}

func (d *driver) ListISOs(ctx context.Context, node, storage string) ([]Volume, error) {
	var rows []volumeRow
	if err := d.get(ctx, storagePath(node, storage, "content?content=iso"), &rows); err != nil {
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
	return d.task(ctx, http.MethodDelete, node, storagePath(node, storage, "content/"+url.PathEscape(volid)), nil)
}

func storagePath(node, storage, rest string) string {
	return nodePath(node, "storage/"+url.PathEscape(storage)+"/"+rest)
}

// UploadISO streams path to the node's storage as the volume name without
// buffering the file; the request bypasses go-proxmox because its uploader
// ranges a map and so emits the multipart fields in random order
// (luthermonson/go-proxmox#352).
func (d *driver) UploadISO(ctx context.Context, node, storage, path, name, sha256 string) (Task, error) {
	file, err := os.Open(path)
	if err != nil {
		return Task{}, fmt.Errorf("upload iso: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return Task{}, fmt.Errorf("upload iso: %w", err)
	}

	head, tail, contentType, err := isoMultipart(name, sha256)
	if err != nil {
		return Task{}, fmt.Errorf("upload iso: %w", err)
	}
	body := io.MultiReader(bytes.NewReader(head), file, bytes.NewReader(tail))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+storagePath(node, storage, "upload"), body)
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
		// A 400's body carries the per-parameter detail the status line lacks.
		body, _ := io.ReadAll(io.LimitReader(res.Body, errorBodyLimit))
		return Task{}, fmt.Errorf("upload iso: %w", mapAPIError(errors.New(statusWithMessage(res.Status, body))))
	}
	var envelope struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		return Task{}, fmt.Errorf("upload iso: decode reply: %w", err)
	}
	return Task{Node: node, UPID: envelope.Data}, nil
}

// isoMultipart renders the text parts PVE reads before the file (head) and
// the closing boundary (tail) so the file itself streams between them.
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

// errorBodyLimit bounds how much of a failed reply's body is read for its message.
const errorBodyLimit = 64 << 10

// statusWithMessage appends PVE's JSON message to a status line that does not already carry it.
func statusWithMessage(status string, body []byte) string {
	if msg := pveMessage(body); msg != "" && !strings.Contains(status, msg) {
		return status + ": " + msg
	}
	return status
}

// pveMessage joins the envelope's message with its per-parameter errors, which a 400 carries.
func pveMessage(body []byte) string {
	var envelope struct {
		Message string          `json:"message"`
		Errors  json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	msg, errs := strings.TrimSpace(envelope.Message), strings.TrimSpace(string(envelope.Errors))
	switch {
	case msg == "":
		return errs
	case errs == "":
		return msg
	}
	return msg + " " + errs
}
