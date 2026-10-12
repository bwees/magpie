package acquisition

import (
	"context"
	"errors"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	requestsapi "magpie/internal/requests/api"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const handlerRegrabOnIssue = "acquisition.regrab-on-issue-action"

func newIssueActionHandler(s *SearchService) bus.Handler {
	return handle(handlerRegrabOnIssue, requestsapi.TypeIssueActionRequested, s.Regrab)
}

// Regrab answers an issue's REGRAB: it blocklists the release of the reported file for good, even a
// transcoded one, and queues a REGRAB search for the episode or the movie.
func (s *SearchService) Regrab(ctx context.Context, tx *gorm.DB, e requestsapi.IssueActionRequested) error {
	if e.Action != requestsapi.IssueActionRegrab {
		return nil
	}
	first, err := claimRegrab(tx, e.Issue, s.now().UTC())
	if err != nil || !first {
		return err
	}
	files, err := s.reportedFiles(ctx, e)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := s.blocklistReported(tx, f); err != nil {
			return err
		}
	}
	task := searchTask{Title: e.Title, Episodes: []domain.EpisodeID{}, Trigger: api.TriggerRegrab, Removed: e.RemovedDownloads}
	if e.Episode != nil {
		task.Episodes = append(task.Episodes, *e.Episode)
	}
	return queueSearch(tx, s.deps.Jobs, task, s.now().UTC())
}

// reportedFiles is the file covering the issue's episode, else every file of the title.
func (s *SearchService) reportedFiles(ctx context.Context, e requestsapi.IssueActionRequested) ([]libraryapi.MediaFile, error) {
	if e.Episode == nil {
		return s.deps.Library.FilesForTitle(ctx, e.Title)
	}
	f, err := s.deps.Library.FileForEpisode(ctx, *e.Episode)
	if errors.Is(err, libraryapi.ErrNotFound) {
		return nil, nil
	}
	return []libraryapi.MediaFile{f}, err
}

// blocklistReported blocks the file's source torrent by hash, and by release name and indexer when its
// download is known, so an indexer that reports no hash can't offer it again. A scanned file with
// neither is only searched again.
func (s *SearchService) blocklistReported(tx *gorm.DB, f libraryapi.MediaFile) error {
	indexer, err := sourceIndexer(tx, f.Download)
	if err != nil {
		return err
	}
	entry := api.BlocklistEntry{Title: f.Title, InfoHash: f.InfoHash, ReleaseName: f.ReleaseName, Indexer: indexer, Reason: api.BlocklistIssue, At: s.now().UTC()}
	if entry.InfoHash == "" && (indexer == "" || entry.ReleaseName == "") {
		return nil
	}
	blocked, err := issueBlocked(tx, entry)
	if err != nil || blocked {
		return err
	}
	_, err = insertBlocklistEntry(tx, entry)
	return err
}

// sourceIndexer is the indexer of the download a file came from; "" when unknown.
func sourceIndexer(tx *gorm.DB, id *domain.DownloadID) (string, error) {
	if id == nil {
		return "", nil
	}
	var names []string
	err := tx.Model(&downloadRow{}).Where("id = ?", *id).Limit(1).Pluck("indexer", &names).Error
	if err != nil || len(names) == 0 {
		return "", err
	}
	return names[0], nil
}

type issueRegrabRow struct {
	IssueID   domain.IssueID `gorm:"primaryKey"`
	HandledAt time.Time
}

func (issueRegrabRow) TableName() string { return "acquisition_issue_regrabs" }

// claimRegrab records the issue's REGRAB and reports false when it already ran.
func claimRegrab(tx *gorm.DB, issue domain.IssueID, at time.Time) (bool, error) {
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&issueRegrabRow{IssueID: issue, HandledAt: at})
	return result.RowsAffected > 0, result.Error
}

// issueBlocked reports whether the release already has an ISSUE entry, so a second REGRAB adds none.
func issueBlocked(tx *gorm.DB, e api.BlocklistEntry) (bool, error) {
	var found int64
	err := tx.Model(&blocklistRow{}).
		Where("title_id = ? AND reason = ? AND info_hash = ? AND release_name = ? AND indexer = ?", e.Title, e.Reason, e.InfoHash, e.ReleaseName, e.Indexer).
		Count(&found).Error
	return found > 0, err
}
