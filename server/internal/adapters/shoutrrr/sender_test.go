package shoutrrr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"magpie/internal/activity"
)

const (
	discordURL  = "discord://token@123456789"
	telegramURL = "telegram://123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw@telegram?chats=@magpie"
	ntfyURL     = "ntfy://ntfy.sh/magpie"
	pushoverURL = "pushover://shoutrrr:apptoken@userkey/"
	smtpURL     = "smtp://user:secret@mail.example.com:587/?from=magpie@example.com&to=me@example.com"
	gotifyURL   = "gotify://gotify.example.com/AzyoeNS.D4iJLVa"
	slackURL    = "slack://hook:WNA3PBYV6-F20DUQND3RQ-Webc4MAvoacrpPakR8phF0zi@webhook"

	// Bodies shaped like Telegram's, which the other services ignore or accept.
	okBody    = `{"ok":true,"result":{}}`
	errorBody = `{"ok":false,"error_code":%d,"description":"refused"}`
)

func TestValidateAcceptsEachSupportedScheme(t *testing.T) {
	s := New()
	for _, raw := range []string{discordURL, telegramURL, ntfyURL, pushoverURL, smtpURL, gotifyURL, slackURL} {
		if err := s.Validate(raw); err != nil {
			t.Errorf("Validate(%q) = %v", raw, err)
		}
	}
}

func TestValidateRejectsUnknownSchemesAndMalformedTokens(t *testing.T) {
	s := New()
	for _, raw := range []string{
		"",
		"not a url",
		"gopher://nowhere",
		"matrix://user:pass@matrix.example.com",
		"discord://",
		"telegram://not-a-token@telegram?chats=@magpie",
		"pushover://shoutrrr:@userkey/",
	} {
		if err := s.Validate(raw); !errors.Is(err, activity.ErrInvalidURL) {
			t.Errorf("Validate(%q) = %v, want ErrInvalidURL", raw, err)
		}
	}
}

// service answers every request with status and records the bodies it received.
type service struct {
	status int

	mu     sync.Mutex
	bodies []string
}

func (s *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.bodies = append(s.bodies, string(body))
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s.status)
	if s.status < http.StatusBadRequest {
		_, _ = io.WriteString(w, okBody)
		return
	}
	_, _ = fmt.Fprintf(w, errorBody, s.status)
}

func (s *service) received() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bodies
}

// redirect sends every request to target, whatever host the service URL names.
type redirect struct{ target *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func senderFor(t *testing.T, status int) (*Sender, *service) {
	t.Helper()
	svc := &service{status: status}
	server := httptest.NewServer(svc)
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return NewWithTransport(redirect{target: target}), svc
}

var message = activity.Message{Title: "Dune is available", Body: "Your request is ready to watch.", Link: "https://magpie.example.com/titles/1"}

func TestSendDeliversTheMessage(t *testing.T) {
	for _, raw := range []string{discordURL, ntfyURL, telegramURL, pushoverURL} {
		s, svc := senderFor(t, http.StatusOK)
		if err := s.Send(context.Background(), raw, message); err != nil {
			t.Errorf("Send(%q) = %v", raw, err)
		}
		if len(svc.received()) == 0 {
			t.Errorf("Send(%q) reached no service", raw)
		}
	}
}

func TestSendMapsClientErrorsToRejected(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound} {
		for _, raw := range []string{discordURL, ntfyURL, telegramURL, pushoverURL} {
			s, _ := senderFor(t, status)
			if err := s.Send(context.Background(), raw, message); !errors.Is(err, activity.ErrRejected) {
				t.Errorf("Send(%q) with %d = %v, want ErrRejected", raw, status, err)
			}
		}
	}
}

func TestSendMapsServerErrorsToUnavailable(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		for _, raw := range []string{ntfyURL, telegramURL, pushoverURL} {
			s, _ := senderFor(t, status)
			if err := s.Send(context.Background(), raw, message); !errors.Is(err, activity.ErrUnavailable) {
				t.Errorf("Send(%q) with %d = %v, want ErrUnavailable", raw, status, err)
			}
		}
	}
}

type unreachable struct{}

func (unreachable) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection refused")
}

func TestSendMapsNetworkErrorsToUnavailable(t *testing.T) {
	s := NewWithTransport(unreachable{})
	for _, raw := range []string{ntfyURL, telegramURL, pushoverURL} {
		if err := s.Send(context.Background(), raw, message); !errors.Is(err, activity.ErrUnavailable) {
			t.Errorf("Send(%q) = %v, want ErrUnavailable", raw, err)
		}
	}
}

func TestSendRejectsAnInvalidURL(t *testing.T) {
	s := NewWithTransport(unreachable{})
	if err := s.Send(context.Background(), "gopher://nowhere", message); !errors.Is(err, activity.ErrInvalidURL) {
		t.Errorf("Send = %v, want ErrInvalidURL", err)
	}
}
