package activity

import (
	"context"
	"net/http"
	"testing"

	"magpie/internal/activity/api"
	"magpie/internal/domain"
	requestsapi "magpie/internal/requests/api"
	transcodeapi "magpie/internal/transcode/api"
)

const clearAttentionPath = "/api/activity/attention/clear"

// Seed 24.
func TestClearAttentionDismissesEveryOpenItem(t *testing.T) {
	h := newHarness(t)
	agent := domain.NewID[domain.AgentID]()
	request := domain.NewID[domain.RequestID]()
	h.deliver(transcodeapi.AgentOffline{Agent: agent})
	h.deliver(requestsapi.RequestCreated{Request: request, Kind: domain.MediaKindMovie})
	opened := h.attentionChanges()

	if err := h.service.ClearAttention(context.Background()); err != nil {
		t.Fatal(err)
	}

	if items := h.attention(); len(items) != 0 {
		t.Errorf("open items = %+v, want none", items)
	}
	cleared := h.attentionChanges()
	if len(cleared) != len(opened) {
		t.Fatalf("changes = %+v, want one per open item", cleared)
	}
	for i, change := range cleared {
		if change.Open || change.Item != opened[i].Item || change.Count != 0 {
			t.Errorf("change = %+v, want %s cleared with count 0", change, opened[i].Item)
		}
	}
}

func TestClearedItemsReopenOnTheirNextEvent(t *testing.T) {
	h := newHarness(t)
	agent := domain.NewID[domain.AgentID]()
	h.deliver(transcodeapi.AgentOffline{Agent: agent})
	if err := h.service.ClearAttention(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.attentionChanges()

	h.deliver(transcodeapi.AgentOnline{Agent: agent})
	if changes := h.attentionChanges(); len(changes) != 0 {
		t.Errorf("clearing a dismissed item published %+v", changes)
	}

	h.deliver(transcodeapi.AgentOffline{Agent: agent})
	assertOpenKinds(t, h.attention(), api.AttentionAgentOffline)
}

func TestClearAttentionWithNothingOpenPublishesNothing(t *testing.T) {
	h := newHarness(t)

	if err := h.service.ClearAttention(context.Background()); err != nil {
		t.Fatal(err)
	}

	if changes := h.attentionChanges(); len(changes) != 0 {
		t.Errorf("changes = %+v, want none", changes)
	}
}

func TestOnlyAdminsClearAttention(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	h.deliver(transcodeapi.AgentOffline{Agent: domain.NewID[domain.AgentID]()})
	admin := h.serve(&domain.Principal{User: domain.NewID[domain.UserID](), Admin: true})

	call[any](t, h.serve(requesterOf(h, title)), http.MethodPost, clearAttentionPath, nil, http.StatusForbidden)
	if items := h.attention(); len(items) != 1 {
		t.Fatalf("open items = %+v, want the agent still open", items)
	}

	call[any](t, admin, http.MethodPost, clearAttentionPath, nil, http.StatusNoContent)
	if items := get[[]AttentionItemResponse](t, admin, "/api/activity/attention", http.StatusOK); len(items) != 0 {
		t.Errorf("attention = %+v, want none", items)
	}
}
