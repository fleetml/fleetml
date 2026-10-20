package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
)

// download fetches url into part, resuming a partial file with an HTTP Range
// request (a server that ignores Range sends everything and the file
// restarts), then checks its sha256. On success the complete file is at part.
func download(ctx context.Context, client *http.Client, url, part, wantSHA string) error {
	if err := os.MkdirAll(filepath.Dir(part), 0o700); err != nil {
		return err
	}
	var offset int64
	if fi, err := os.Stat(part); err == nil {
		offset = fi.Size()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := client.Do(req)
	if err != nil {
		// The presigned query string (signature, expiry) is noise in a log
		// line and in the failure event sent to the server.
		var ue *neturl.Error
		if errors.As(err, &ue) {
			ue.URL, _, _ = strings.Cut(ue.URL, "?")
		}
		return fmt.Errorf("downloading: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusPartialContent:
		flags |= os.O_APPEND
	case http.StatusOK:
		flags |= os.O_TRUNC
	case http.StatusRequestedRangeNotSatisfiable: // the part is already complete
	default:
		return fmt.Errorf("downloading: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		f, err := os.OpenFile(part, flags, 0o600)
		if err != nil {
			return err
		}
		_, cerr := io.Copy(f, resp.Body)
		if err := errors.Join(cerr, f.Close()); err != nil {
			return fmt.Errorf("interrupted after a partial download (the next attempt resumes): %w", err)
		}
	}
	got, err := fileSHA256(part)
	if err != nil {
		return err
	}
	if got != wantSHA {
		_ = os.Remove(part)
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, wantSHA)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
