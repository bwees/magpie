// Package shoutrrr sends external notifications through the nicholas-fedor fork of shoutrrr.
package shoutrrr

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"magpie/internal/activity"

	"github.com/nicholas-fedor/shoutrrr/pkg/router"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"
)

// Schemes are the services Magpie accepts. Each one's URL parses without a network call, which
// isn't true of every shoutrrr service (matrix signs in, for one).
var Schemes = []string{"discord", "gotify", "ntfy", "pushover", "slack", "smtp", "telegram"}

const (
	sendTimeout   = 30 * time.Second
	lineSeparator = "\n"
)

// Sender implements activity.NotificationSender.
type Sender struct {
	transport http.RoundTripper
}

var _ activity.NotificationSender = (*Sender)(nil)

func New() *Sender {
	return NewWithTransport(http.DefaultTransport)
}

// NewWithTransport sends every HTTP request through transport.
func NewWithTransport(transport http.RoundTripper) *Sender {
	return &Sender{transport: transport}
}

func (s *Sender) Validate(raw string) error {
	_, err := locate(raw, &http.Client{Transport: s.transport})
	return err
}

func (s *Sender) Send(ctx context.Context, raw string, m activity.Message) error {
	recorder := &statusRecorder{next: s.transport}
	service, err := locate(raw, &http.Client{Transport: recorder, Timeout: sendTimeout})
	if err != nil {
		return err
	}
	params := types.Params{}
	params.SetTitle(m.Title)
	text := strings.Join(slices.DeleteFunc([]string{m.Body, m.Link}, func(line string) bool { return line == "" }), lineSeparator)
	if text == "" {
		text = m.Title
	}
	if sender, ok := service.(types.ContextSender); ok {
		err = sender.SendContext(ctx, text, &params)
	} else {
		err = service.Send(text, &params)
	}
	status, transportErr := recorder.last()
	return classify(err, status, transportErr)
}

func locate(raw string, client *http.Client) (types.Service, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !slices.Contains(Schemes, parsed.Scheme) {
		return nil, fmt.Errorf("%w: unsupported service", activity.ErrInvalidURL)
	}
	r, err := router.NewWithOptions(nil, types.SenderOptions{HTTPClient: client})
	if err != nil {
		return nil, err
	}
	service, err := r.Locate(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", activity.ErrInvalidURL, err)
	}
	return service, nil
}

// classify decides from the service's last HTTP exchange as well as its error, because some services
// (telegram, for one) swallow failed responses. A refusal (4xx) is ErrRejected. Anything else that
// failed is ErrUnavailable, including timeouts and rate limits, which are worth retrying.
func classify(err error, status int, transportErr error) error {
	refused := status >= http.StatusBadRequest && status < http.StatusInternalServerError &&
		status != http.StatusRequestTimeout && status != http.StatusTooManyRequests
	switch {
	case transportErr != nil:
		return fmt.Errorf("%w: %w", activity.ErrUnavailable, transportErr)
	case refused:
		return fmt.Errorf("%w: %s", activity.ErrRejected, statusText(status))
	case status >= http.StatusBadRequest:
		return fmt.Errorf("%w: %s", activity.ErrUnavailable, statusText(status))
	case err != nil:
		return fmt.Errorf("%w: %w", activity.ErrUnavailable, err)
	}
	return nil
}

func statusText(status int) string {
	return fmt.Sprintf("HTTP %d %s", status, http.StatusText(status))
}

// statusRecorder remembers how the service's last HTTP request went.
type statusRecorder struct {
	next http.RoundTripper

	mu     sync.Mutex
	status int
	err    error
}

func (r *statusRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.next.RoundTrip(req)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status, r.err = 0, err
	if err == nil {
		r.status = resp.StatusCode
	}
	return resp, err
}

func (r *statusRecorder) last() (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status, r.err
}
