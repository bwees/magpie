package transcode

import (
	"testing"

	"magpie/internal/domain"
)

// Migration seed 8 (11-migration.md): a migrated file waits behind Magpie's own downloads.
func TestAMigratedFileQueuesAtBacklogPriorityBehindALaterDownload(t *testing.T) {
	f := newFixture(t)
	migrated := f.addFile(h264Movie(0))
	f.cutoff(migrated, domain.OriginMigration)
	downloaded := f.addFile(h264Movie(0))
	f.cutoff(downloaded, domain.OriginDownload)

	if job := f.onlyJob(migrated); job.Priority != priorityBacklog || job.Origin != domain.OriginMigration {
		t.Fatalf("migrated job = %+v, want backlog priority", job)
	}
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	if spec.Job != f.onlyJob(downloaded).ID {
		t.Fatalf("the agent got job %s, want the download's %s first", spec.Job, f.onlyJob(downloaded).ID)
	}
}
