package flows_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"magpie/internal/platform/bus"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

const (
	typeSmoke      = "flows.SmokeTested"
	waitForHandler = 5 * time.Second
)

type smokeTested struct {
	Note string `json:"note"`
}

func TestSmokeBootsAllModulesAndDeliversEvents(t *testing.T) {
	received := make(chan string, 1)
	h := boot(t, fx.Provide(bus.AsHandler(func() bus.Handler {
		return bus.Handler{
			Name:       "flows.smoke",
			EventTypes: []string{typeSmoke},
			Handle: func(_ context.Context, _ *gorm.DB, ev bus.Event) error {
				payload, err := bus.Decode[smokeTested](ev)
				received <- payload.Note
				return err
			},
		}
	})))

	if rec := h.get("/api/health"); rec.Code != http.StatusOK {
		t.Fatalf("health = %d", rec.Code)
	}

	err := h.DB.Write(context.Background(), func(tx *gorm.DB) error {
		return h.Bus.Publish(tx, bus.NewEvent(typeSmoke, "smoke", smokeTested{Note: "hello"}))
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case note := <-received:
		if note != "hello" {
			t.Errorf("handler got %q", note)
		}
	case <-time.After(waitForHandler):
		t.Fatal("handler never received the event")
	}
}
