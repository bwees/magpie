package health

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/settings"
)

const testVersion = "5.2.1"

var errRefused = errors.New("connection refused")

type scriptedProbe struct {
	service platformapi.Service
	result  Result
	err     error
}

func (p *scriptedProbe) Service() platformapi.Service { return p.service }

func (p *scriptedProbe) Probe(context.Context) (Result, error) { return p.result, p.err }

func newChecker(t *testing.T, probes ...Probe) (*Checker, *db.DB) {
	t.Helper()
	d := dbtest.Open(t)
	return NewChecker(CheckerParams{DB: d, Bus: bus.New(), Store: settings.NewStore(d), Probes: probes}), d
}

func published(t *testing.T, d *db.DB) []platformapi.ServiceHealthChanged {
	t.Helper()
	var payloads []string
	err := d.Read(context.Background()).Table("platform_outbox").
		Where("type = ?", platformapi.TypeServiceHealthChanged).Order("seq").Pluck("payload", &payloads).Error
	if err != nil {
		t.Fatal(err)
	}
	events := make([]platformapi.ServiceHealthChanged, 0, len(payloads))
	for _, p := range payloads {
		var ev platformapi.ServiceHealthChanged
		if err := json.Unmarshal([]byte(p), &ev); err != nil {
			t.Fatal(err)
		}
		events = append(events, ev)
	}
	return events
}

func TestServiceHealthChangedIsPublishedOnlyWhenHealthFlips(t *testing.T) {
	probe := &scriptedProbe{service: platformapi.ServiceQBittorrent}
	checker, d := newChecker(t, probe)
	ctx := context.Background()

	steps := []struct {
		name   string
		result Result
		err    error
		want   Status
	}{
		{"first healthy probe", Result{Version: testVersion}, nil, StatusHealthy},
		{"still healthy", Result{Version: testVersion}, nil, StatusHealthy},
		{"goes down", Result{}, errRefused, StatusUnhealthy},
		{"still down", Result{}, errRefused, StatusUnhealthy},
		{"recovers", Result{Version: testVersion}, nil, StatusHealthy},
		{"credentials removed", Result{}, ErrNotConfigured, StatusNotConfigured},
	}
	for _, step := range steps {
		probe.result, probe.err = step.result, step.err
		check, err := checker.Check(ctx, probe.service)
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if check.Status != step.want {
			t.Errorf("%s: status = %s, want %s", step.name, check.Status, step.want)
		}
	}

	want := []platformapi.ServiceHealthChanged{
		{Service: platformapi.ServiceQBittorrent, Healthy: false, Error: errRefused.Error()},
		{Service: platformapi.ServiceQBittorrent, Healthy: true},
	}
	if got := published(t, d); !slices.Equal(got, want) {
		t.Errorf("published %+v, want %+v", got, want)
	}
}

func TestAFlipIsDetectedAcrossRestarts(t *testing.T) {
	probe := &scriptedProbe{service: platformapi.ServiceJellyfin, err: errRefused}
	checker, d := newChecker(t, probe)
	ctx := context.Background()
	if _, err := checker.Check(ctx, probe.service); err != nil {
		t.Fatal(err)
	}

	restarted := NewChecker(CheckerParams{DB: d, Bus: bus.New(), Store: settings.NewStore(d), Probes: []Probe{probe}})
	if _, err := restarted.Check(ctx, probe.service); err != nil {
		t.Fatal(err)
	}
	if got := published(t, d); len(got) != 1 {
		t.Errorf("published %d events for one outage across a restart, want 1", len(got))
	}
}

func TestLatestListsEveryServiceInOrder(t *testing.T) {
	probe := &scriptedProbe{service: platformapi.ServiceProwlarr, result: Result{Version: testVersion, Detail: "18 indexers"}}
	checker, _ := newChecker(t, probe)
	ctx := context.Background()
	if _, err := checker.Check(ctx, probe.service); err != nil {
		t.Fatal(err)
	}

	checks, err := checker.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != len(Services) {
		t.Fatalf("got %d checks, want %d", len(checks), len(Services))
	}
	for i, check := range checks {
		if check.Service != Services[i] {
			t.Errorf("check %d is %s, want %s", i, check.Service, Services[i])
		}
		wantStatus := StatusUnknown
		if check.Service == probe.service {
			wantStatus = StatusHealthy
		}
		if check.Status != wantStatus {
			t.Errorf("%s status = %s, want %s", check.Service, check.Status, wantStatus)
		}
	}
}

func TestCheckRejectsAServiceWithoutAProbe(t *testing.T) {
	checker, _ := newChecker(t)
	if _, err := checker.Check(context.Background(), platformapi.ServiceTMDB); !errors.Is(err, ErrUnknownService) {
		t.Errorf("err = %v, want ErrUnknownService", err)
	}
}
