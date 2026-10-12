package transcode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"magpie/internal/transcode/api/agentproto"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func socketPair(t *testing.T, serve func(conn *websocket.Conn)) *websocket.Conn {
	t.Helper()
	held := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		serve(conn)
		<-held
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(held) })
	client, _, err := websocket.Dial(context.Background(), server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.CloseNow() })
	return client
}

func TestAStoppedContextNeverCutsOffAControlFrame(t *testing.T) {
	stopped, stop := context.WithCancel(context.Background())
	stop()
	ack, err := newEnvelope(agentproto.TypeAck, agentproto.Ack{})
	if err != nil {
		t.Fatal(err)
	}
	written := make(chan error, 1)
	client := socketPair(t, func(conn *websocket.Conn) {
		written <- newControlChannel(conn).write(stopped, ack)
	})

	ctx, cancel := context.WithTimeout(context.Background(), frameTimeout)
	defer cancel()
	var got agentproto.Envelope
	if err := wsjson.Read(ctx, client, &got); err != nil {
		t.Fatalf("agent read error = %v, want the frame", err)
	}
	if err := <-written; err != nil {
		t.Fatalf("write error = %v, want nil", err)
	}
	if got.Type != agentproto.TypeAck {
		t.Fatalf("frame type = %q, want %q", got.Type, agentproto.TypeAck)
	}
}
