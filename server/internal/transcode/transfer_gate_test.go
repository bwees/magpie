package transcode

import (
	"testing"
	"time"

	"magpie/internal/domain"
)

func holder(direction TransferDirection) TransferHolder {
	return TransferHolder{Job: domain.NewID[domain.JobID](), Direction: direction}
}

// Seed 20: with the cap full, a waiting download gets the freed slot before a prefetch that asked first.
func TestFreedSlotGoesToTheWaitingDownloadBeforeAPrefetch(t *testing.T) {
	clock := &testClock{now: testStart}
	gate := NewTransferGate()
	gate.now = clock.Now
	const capacity = 2
	first, second := holder(DirectionOutput), holder(DirectionOutput)
	for _, h := range []TransferHolder{first, second} {
		if !gate.Acquire(h, RankUpload, capacity) {
			t.Fatal("upload refused under the cap")
		}
	}
	prefetch, download := holder(DirectionSource), holder(DirectionSource)
	if gate.Acquire(prefetch, RankPrefetch, capacity) || gate.Acquire(download, RankDownload, capacity) {
		t.Fatal("a third transfer got a slot over the cap")
	}

	gate.Release(first)
	clock.Advance(RetryAfter)

	if gate.Acquire(prefetch, RankPrefetch, capacity) {
		t.Fatal("the prefetch took the slot a download is waiting for")
	}
	if !gate.Acquire(download, RankDownload, capacity) {
		t.Fatal("the waiting download didn't get the freed slot")
	}
}

func TestUploadsGoAheadOfDownloadsThatWaitedLonger(t *testing.T) {
	gate := NewTransferGate()
	busy, download, upload := holder(DirectionSource), holder(DirectionSource), holder(DirectionOutput)
	gate.Acquire(busy, RankDownload, 1)
	gate.Acquire(download, RankDownload, 1)
	gate.Acquire(upload, RankUpload, 1)

	gate.Release(busy)

	if gate.Acquire(download, RankDownload, 1) || !gate.Acquire(upload, RankUpload, 1) {
		t.Fatal("want the upload to skip ahead of the waiting download")
	}
}

func TestAWaiterThatStopsAskingLosesItsPlace(t *testing.T) {
	clock := &testClock{now: testStart}
	gate := NewTransferGate()
	gate.now = clock.Now
	busy, gone, prefetch := holder(DirectionSource), holder(DirectionOutput), holder(DirectionSource)
	gate.Acquire(busy, RankDownload, 1)
	gate.Acquire(gone, RankUpload, 1)

	gate.Release(busy)
	clock.Advance(waiterTimeout + time.Second)

	if !gate.Acquire(prefetch, RankPrefetch, 1) {
		t.Fatal("a slot stayed reserved for an upload that gave up")
	}
}

func TestReleaseJobFreesADirectReadSlot(t *testing.T) {
	gate := NewTransferGate()
	direct := holder(DirectionSource)
	gate.Acquire(direct, RankDownload, 1)
	if !gate.Acquire(direct, RankDownload, 1) {
		t.Fatal("asking again while holding the slot was refused")
	}

	gate.ReleaseJob(direct.Job)

	if !gate.Acquire(holder(DirectionSource), RankDownload, 1) {
		t.Fatal("the job's slot wasn't freed")
	}
}
