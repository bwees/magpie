// Package live forwards bus events to browsers over one WebSocket per tab, with resume after reconnect.
package live

import "encoding/json"

// LiveMessage is one server-to-browser frame.
type LiveMessage struct {
	// Seq increases by one per buffered event within an epoch. Control messages carry the latest seq.
	Seq uint64 `json:"seq"`
	// Epoch identifies this server process; a new epoch means the browser must refetch everything.
	Epoch string `json:"epoch"`
	// Type is the event type, e.g. "acquisition.DownloadProgressed", or a live.* control type.
	Type string `json:"type"`
	// Topic is empty for broadcasts, or "search:<SearchID>".
	Topic string `json:"topic"`
	// Data is the event payload, the same struct the api package publishes.
	Data json.RawMessage `json:"data"`
}

// LiveClientMessage is one browser-to-server frame.
type LiveClientMessage struct {
	Op     Op       `json:"op"`
	Epoch  string   `json:"epoch,omitempty"`
	Seq    uint64   `json:"seq,omitempty"`
	Topics []string `json:"topics,omitempty"`
}

// Op values are lowercase on the wire, as the live protocol specifies.
type Op string

const (
	OpResume      Op = "resume"
	OpSubscribe   Op = "subscribe"
	OpUnsubscribe Op = "unsubscribe"
	OpPong        Op = "pong"
)

const (
	TypeHeartbeat = "live.Heartbeat"
	TypeReset     = "live.Reset"

	SearchTopicPrefix = "search:"
)

// searchTopicEvents go only to sockets subscribed to their search's topic.
var searchTopicEvents = map[string]bool{
	"acquisition.SearchResultsReceived": true,
	"acquisition.SearchCompleted":       true,
}
