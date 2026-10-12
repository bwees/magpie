package transcode

import (
	"context"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/settings"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/contracttest"

	"gorm.io/gorm"
)

func TestJobRepositoryMeetsTheContract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T, f contracttest.Fixture) api.Reader {
		d := dbtest.Open(t, Migrations())
		store := settings.NewStore(d)
		err := d.Write(context.Background(), func(tx *gorm.DB) error { return seedReader(tx, f) })
		if err != nil {
			t.Fatal(err)
		}
		return NewJobRepository(d, NewAgentRepository(d), store)
	})
}

func seedReader(tx *gorm.DB, f contracttest.Fixture) error {
	holder := map[domain.JobID]domain.AgentID{}
	for _, agent := range f.Agents {
		row := agentRow{
			ID: string(agent.ID), Hostname: agent.Hostname, ReportedMode: string(agent.Mode), Online: agent.Online,
			Draining: agent.Draining, LastSeenAt: agent.LastSeenAt, CreatedAt: agent.LastSeenAt,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for _, job := range agent.Jobs {
			holder[job] = agent.ID
		}
	}
	for _, job := range f.Jobs {
		row := jobRow{
			ID: job.ID, Kind: job.Kind, MediaFileID: job.MediaFile, TitleID: domain.NewID[domain.TitleID](),
			Origin: domain.OriginDownload, State: job.State, Priority: job.Priority, Attempt: job.Attempt,
			ExcludedAgents: emptyAgents, QueuedAt: job.QueuedAt, UpdatedAt: job.QueuedAt, FinishedAt: job.FinishedAt,
			Progress: job.Progress, Error: job.Error,
		}
		if agent, ok := holder[job.ID]; ok {
			row.AgentID = &agent
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	for _, title := range f.Excluded {
		if err := tx.Create(&exclusionRow{Scope: ExcludeTitle, Ref: string(title)}).Error; err != nil {
			return err
		}
	}
	policy := policyKey.Default
	policy.TargetCodec, policy.Preset, policy.CapsMbps = f.Target.Codec, f.Target.Preset, f.Target.CapsMbps
	return policyKey.SetTx(tx, policy)
}
