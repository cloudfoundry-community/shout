package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
)

// The Lisp Shout! server (lisp/api.lisp, write-database) saved its state
// with cl-json as a JSON list of topic objects:
//
//	[{"name": "...", "state": "broken", "notified": 1700000000,
//	  "reminder": 3600, "previous": {...}, "first": {...}, "last": {...}}]
//
// and each event as
//
//	{"occurred_at": 1700000000, "reported_at": 1700000000, "ok": true,
//	 "metadata": {...}, "message": "...", "link": "..."}
//
// The field names match the Go format, but cl-json writes Lisp NIL as
// null. A failed event therefore has "ok": null rather than false, unset
// fields are null, and a server with no topics wrote the bare word null.
// Metadata values keep whatever JSON type the client sent. A Lisp server
// that reloaded its own file also lost the event timestamps, because it
// read them back under different keys, so those are null too.

// lispTopic is one entry of a Lisp state file. Scalars are decoded as any
// so that the conversion can follow Lisp's loose typing.
type lispTopic struct {
	Name     any        `json:"name"`
	State    any        `json:"state"`
	Notified any        `json:"notified"`
	Reminder any        `json:"reminder"`
	Previous *lispEvent `json:"previous"`
	First    *lispEvent `json:"first"`
	Last     *lispEvent `json:"last"`
}

type lispEvent struct {
	OccurredAt any `json:"occurred_at"`
	ReportedAt any `json:"reported_at"`
	OK         any `json:"ok"`
	Metadata   any `json:"metadata"`
	Message    any `json:"message"`
	Link       any `json:"link"`
}

// parseLisp converts the contents of a Lisp state file. now stands in for
// a last-event report time the file lost, so that the topic does not
// expire on the first scan. expiry is given to every imported topic. It
// returns the topics keyed by name and the number of entries skipped for
// lack of a name. When a name repeats, the later entry wins, as it did when
// the Lisp server loaded the file.
func parseLisp(data []byte, now, expiry int64) (map[string]*TopicState, int, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var entries []*lispTopic
	if err := dec.Decode(&entries); err != nil {
		return nil, 0, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, 0, errors.New("unexpected data after the list")
	}

	states := make(map[string]*TopicState, len(entries))
	skipped := 0
	for _, lt := range entries {
		name := lt.name()
		if name == "" {
			skipped++
			continue
		}
		ts := &TopicState{
			Name:           name,
			Status:         lispString(lt.State),
			LastNotifiedAt: lispInt(lt.Notified),
			RemindEvery:    lispInt(lt.Reminder),
			Expiry:         expiry,
			PreviousEvent:  lt.Previous.event(name),
			FirstEvent:     lt.First.event(name),
			LastEvent:      lt.Last.event(name),
		}
		if ts.Status == "" {
			ts.Status = "broken"
			if ts.IsOK() {
				ts.Status = "working"
			}
		}
		if ts.LastEvent != nil && ts.LastEvent.ReportedAt == 0 {
			ts.LastEvent.ReportedAt = ts.LastEvent.OccurredAt
			if ts.LastEvent.ReportedAt == 0 {
				ts.LastEvent.ReportedAt = now
			}
		}
		states[name] = ts
	}
	return states, skipped, nil
}

// name returns the topic name, or "" when the entry is null or its name
// is missing or not a string.
func (lt *lispTopic) name() string {
	if lt == nil {
		return ""
	}
	name, _ := lt.Name.(string)
	return name
}

func (le *lispEvent) event(topic string) *Event {
	if le == nil {
		return nil
	}
	return &Event{
		Topic:      topic,
		OK:         lispTruthy(le.OK),
		Message:    lispString(le.Message),
		Link:       lispString(le.Link),
		Metadata:   lispMetadata(le.Metadata),
		OccurredAt: lispInt(le.OccurredAt),
		ReportedAt: lispInt(le.ReportedAt),
	}
}

// lispTruthy follows Lisp: only NIL, which cl-json writes as null, is
// false. JSON false is treated as false too.
func lispTruthy(v any) bool {
	switch b := v.(type) {
	case nil:
		return false
	case bool:
		return b
	default:
		return true
	}
}

// lispString renders a scalar as text. Null becomes the empty string, and
// a nested value becomes its JSON text.
func lispString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	default:
		out, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(out)
	}
}

// lispInt reads a timestamp or a duration in seconds. Anything that is not
// a number, including null, is zero.
func lispInt(v any) int64 {
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n
		}
		if f, err := x.Float64(); err == nil && !math.IsNaN(f) && f >= math.MinInt64 && f < math.MaxInt64 {
			return int64(f)
		}
	case string:
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// lispMetadata converts an event's metadata object to the Go string map.
// Values of other JSON types become their text, and anything other than an
// object is dropped.
func lispMetadata(v any) map[string]string {
	obj, ok := v.(map[string]any)
	if !ok || len(obj) == 0 {
		return nil
	}
	out := make(map[string]string, len(obj))
	for k, val := range obj {
		out[k] = lispString(val)
	}
	return out
}
