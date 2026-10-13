package agentproto

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"magpie/internal/domain"
)

const (
	heartbeatEvery = 10 * time.Second
	leaseTTL       = 60 * time.Second
	scratchFree    = 200 << 30
	deviceSlots    = 2
	testDevice     = "cuda:0"
)

type envelopeID string

func roundTrip[T any](t *testing.T, messageType MessageType, body T) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := json.Marshal(Envelope{Type: messageType, ID: string(domain.NewID[envelopeID]()), Body: raw})
	if err != nil {
		t.Fatal(err)
	}

	var env Envelope
	if err := json.Unmarshal(frame, &env); err != nil {
		t.Fatal(err)
	}
	if env.Type != messageType {
		t.Errorf("type = %q, want %q", env.Type, messageType)
	}
	var got T
	if err := json.Unmarshal(env.Body, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, body) {
		t.Errorf("body = %+v, want %+v", got, body)
	}
}

func TestAgentToServerRoundTrip(t *testing.T) {
	roundTrip(t, TypeHello, Hello{
		AgentID:     domain.NewID[domain.AgentID](),
		Hostname:    "gpu-box",
		Mode:        ModeDownloadFirst,
		ScratchFree: scratchFree,
		Devices: []DeviceReport{{
			DeviceID:     testDevice,
			Vendor:       VendorNVIDIA,
			Capabilities: []CapabilityResult{{Capability: CapEncHEVC10, OK: true}},
		}},
		ResumableJobs: []ResumableJob{{Job: domain.NewID[domain.JobID](), SourceETag: `"1-2-3"`}},
	})
}

func TestServerToAgentRoundTrip(t *testing.T) {
	roundTrip(t, TypeWelcome, Welcome{
		HeartbeatEvery: heartbeatEvery,
		LeaseTTL:       leaseTTL,
		Mode:           ModeDirectRead,
		Slots:          map[string]int{testDevice: deviceSlots},
		Stages:         []Stage{StageFetching, StageEncoding, StageChecking, StageUploading},
	})
}
