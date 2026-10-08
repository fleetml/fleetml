package agent

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"google.golang.org/protobuf/proto"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

// maxQueued bounds the telemetry queue; the oldest events are dropped first.
const maxQueued = 10000

// Queue is a bounded FIFO of telemetry events in a file of length-prefixed
// protobuf records, so events survive restarts and outages. A torn last
// record (power loss during a write) is ignored.
type Queue struct {
	path    string
	mu      sync.Mutex
	events  []*fleetmlv1.TelemetryEvent
	dropped int
}

// OpenQueue loads the queue file, if any.
func OpenQueue(path string) (*Queue, error) {
	q := &Queue{path: path}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	r := bytes.NewReader(data)
	for {
		var n uint32
		if err := binary.Read(r, binary.BigEndian, &n); err != nil {
			break
		}
		if n > 64*1024 || int(n) > r.Len() {
			break
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			break
		}
		e := &fleetmlv1.TelemetryEvent{}
		if proto.Unmarshal(buf, e) != nil {
			break
		}
		q.events = append(q.events, e)
	}
	q.trim()
	return q, q.persist()
}

// Add queues an event.
func (q *Queue) Add(e *fleetmlv1.TelemetryEvent) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = append(q.events, e)
	if q.trim() {
		return q.persist()
	}
	return q.appendRecord(e)
}

// Peek returns up to n events from the front.
func (q *Queue) Peek(n int) []*fleetmlv1.TelemetryEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	if n > len(q.events) {
		n = len(q.events)
	}
	return append([]*fleetmlv1.TelemetryEvent(nil), q.events[:n]...)
}

// Remove drops n events from the front (after they were sent).
func (q *Queue) Remove(n int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if n > len(q.events) {
		n = len(q.events)
	}
	q.events = q.events[n:]
	return q.persist()
}

// Len is the number of queued events.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.events)
}

// Dropped counts events dropped because the queue was full.
func (q *Queue) Dropped() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.dropped
}

func (q *Queue) trim() bool {
	if excess := len(q.events) - maxQueued; excess > 0 {
		q.events = q.events[excess:]
		q.dropped += excess
		return true
	}
	return false
}

func record(e *fleetmlv1.TelemetryEvent) ([]byte, error) {
	body, err := proto.Marshal(e)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 4, 4+len(body))
	binary.BigEndian.PutUint32(out, uint32(len(body)))
	return append(out, body...), nil
}

func (q *Queue) appendRecord(e *fleetmlv1.TelemetryEvent) error {
	rec, err := record(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(q.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(rec)
	return errors.Join(werr, f.Close())
}

func (q *Queue) persist() error {
	var buf bytes.Buffer
	for _, e := range q.events {
		rec, err := record(e)
		if err != nil {
			return err
		}
		buf.Write(rec)
	}
	if err := os.MkdirAll(filepath.Dir(q.path), 0o700); err != nil {
		return err
	}
	tmp := q.path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, q.path)
}
