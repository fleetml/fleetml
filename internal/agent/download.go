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
	"sync/atomic"
	"time"
)

// downloadStall is how long a download may receive nothing before it is cut
// off; the next attempt resumes the partial file.
var downloadStall = 60 * time.Second

// downloadClient fetches models. A download may take up to 30 minutes, but
// the object store must answer within callTimeout, and stallReader cuts a
// download that stops receiving.
func downloadClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = callTimeout
	return &http.Client{Timeout: 30 * time.Minute, Transport: t}
}

// stallReader cancels the request when no bytes arrive for d.
type stallReader struct {
	r       io.Reader
	d       time.Duration
	timer   *time.Timer
	stalled atomic.Bool
}

func newStallReader(r io.Reader, d time.Duration, cancel context.CancelFunc) *stallReader {
	s := &stallReader{r: r, d: d}
	s.timer = time.AfterFunc(d, func() { s.stalled.Store(true); cancel() })
	return s
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.timer.Reset(s.d)
	}
	return n, err
}

// download fetches url into part, resuming a partial file with an HTTP Range
// request (a server that ignores Range sends everything and the file
// restarts), then checks its sha256. On success the complete file is at part.
// A non-empty hostOverride (host:port) is where the request goes instead of
// the URL's host; see overrideHost.
func download(ctx context.Context, client *http.Client, url, part, wantSHA, hostOverride string) error {
	if err := os.MkdirAll(filepath.Dir(part), 0o700); err != nil {
		return err
	}
	var offset int64
	if fi, err := os.Stat(part); err == nil {
		offset = fi.Size()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	overrideHost(req, hostOverride)
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
		body := newStallReader(resp.Body, downloadStall, cancel)
		_, cerr := io.Copy(f, body)
		body.timer.Stop()
		if body.stalled.Load() {
			cerr = fmt.Errorf("no data for %s", downloadStall)
		}
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

// overrideHost sends req to host (host:port) instead of the URL's host. The
// path and query are unchanged and the Host header stays the URL's original
// host: a presigned S3 URL's signature covers that header, so the object
// store accepts the request although it arrived at another address.
func overrideHost(req *http.Request, host string) {
	if host == "" {
		return
	}
	req.Host = req.URL.Host
	req.URL.Host = host
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
