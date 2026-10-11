package settings

import (
	"context"
	"errors"
	"testing"
	"time"

	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/owner"

	"gorm.io/gorm"
)

type retention struct {
	Days    int  `json:"days"`
	Enabled bool `json:"enabled"`
}

var retentionKey = NewKey(owner.Activity, "retention", retention{Days: 90, Enabled: true})

func newStore(t *testing.T) *Store {
	return NewStore(dbtest.Open(t))
}

func TestKeyIsOwnerPrefixed(t *testing.T) {
	if got := retentionKey.String(); got != "activity.retention" {
		t.Errorf("key = %q", got)
	}
	if got := QBittorrent.String(); got != "platform.connections.qbittorrent" {
		t.Errorf("connection key = %q", got)
	}
}

func TestUnsetKeyReadsDefault(t *testing.T) {
	got, err := retentionKey.Get(context.Background(), newStore(t))
	if err != nil {
		t.Fatal(err)
	}
	if got != retentionKey.Default {
		t.Errorf("got %+v, want default %+v", got, retentionKey.Default)
	}
}

func TestSetThenGetRoundTrips(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	want := retention{Days: 30}

	for _, value := range []retention{{Days: 7, Enabled: true}, want} {
		if err := retentionKey.Set(ctx, store, value); err != nil {
			t.Fatalf("set: %v", err)
		}
	}

	got, err := retentionKey.Get(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSetTxRollsBackWithTheTransaction(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	errAbort := errors.New("abort")

	err := store.db.Write(ctx, func(tx *gorm.DB) error {
		if err := retentionKey.SetTx(tx, retention{Days: 1}); err != nil {
			return err
		}
		return errAbort
	})
	if !errors.Is(err, errAbort) {
		t.Fatal(err)
	}

	got, err := retentionKey.Get(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if got != retentionKey.Default {
		t.Errorf("rolled-back write is visible: %+v", got)
	}
}

func TestUnknownOwnerIsRejected(t *testing.T) {
	key := NewKey(owner.Name("billing"), "plan", "free")
	if err := key.Set(context.Background(), newStore(t), "pro"); !errors.Is(err, ErrUnknownOwner) {
		t.Fatalf("err = %v, want ErrUnknownOwner", err)
	}
}

func TestConnectionDefaults(t *testing.T) {
	got, err := QBittorrent.Get(context.Background(), newStore(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != DefaultQBittorrentCategory {
		t.Errorf("category = %q, want %q", got.Category, DefaultQBittorrentCategory)
	}
}

func TestDurationStoresAsText(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	key := NewKey(owner.Acquisition, "rss.interval", Duration(15*time.Minute))

	if err := key.Set(ctx, store, Duration(20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := store.db.Read(ctx).Table("platform_settings").Where("key = ?", key.String()).Pluck("value", &raw).Error; err != nil {
		t.Fatal(err)
	}
	if raw != `"20m0s"` {
		t.Errorf("stored %s, want \"20m0s\"", raw)
	}
	got, err := key.Get(ctx, store)
	if err != nil || got.Std() != 20*time.Minute {
		t.Errorf("got %v, %v", got.Std(), err)
	}
}
