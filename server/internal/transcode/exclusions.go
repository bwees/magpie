package transcode

import (
	"context"
	"errors"
	"slices"

	"magpie/internal/domain"

	"gorm.io/gorm"
)

var (
	ExclusionScopes = []ExclusionScope{ExcludeTitle, ExcludeRootFolder}

	ErrUnknownScope = errors.New("transcode: unknown exclusion scope")
)

// Exclusion keeps a title or a root folder out of transcoding.
type Exclusion struct {
	Scope ExclusionScope
	Ref   string
}

func (e Exclusion) row() (exclusionRow, error) {
	if !slices.Contains(ExclusionScopes, e.Scope) {
		return exclusionRow{}, ErrUnknownScope
	}
	ref := e.Ref
	if e.Scope == ExcludeRootFolder {
		ref = cleanFolder(ref)
	}
	return exclusionRow{Scope: e.Scope, Ref: ref}, nil
}

// Exclusions lists every exclusion, titles first.
func (s *JobService) Exclusions(ctx context.Context) ([]Exclusion, error) {
	var rows []exclusionRow
	if err := s.deps.DB.Read(ctx).Order("scope DESC, ref").Find(&rows).Error; err != nil {
		return nil, err
	}
	exclusions := make([]Exclusion, 0, len(rows))
	for _, row := range rows {
		exclusions = append(exclusions, Exclusion(row))
	}
	return exclusions, nil
}

// Exclude keeps a title or root folder out of transcoding: its queued jobs are cancelled and its files
// skipped as EXCLUDED. Jobs agents already hold finish.
func (s *JobService) Exclude(ctx context.Context, e Exclusion) error {
	row, err := e.row()
	if err != nil {
		return err
	}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return s.reevaluate(ctx, tx, row)
	})
}

// Include ends an exclusion and evaluates the files it kept out again.
func (s *JobService) Include(ctx context.Context, e Exclusion) error {
	row, err := e.row()
	if err != nil {
		return err
	}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("scope = ? AND ref = ?", row.Scope, row.Ref).Delete(&exclusionRow{}).Error; err != nil {
			return err
		}
		return s.reevaluate(ctx, tx, row)
	})
}

// reevaluate runs the policy on a title's files at once. A root folder may hold the whole library, so
// it sweeps the backlog instead.
func (s *JobService) reevaluate(ctx context.Context, tx *gorm.DB, row exclusionRow) error {
	if row.Scope == ExcludeRootFolder {
		return s.deps.Queue.EnqueueTx(tx, sweepTask{})
	}
	policy, err := policyKey.GetTx(tx)
	if err != nil {
		return err
	}
	var files []eligibleFileRow
	if err := tx.Where("title_id = ?", domain.TitleID(row.Ref)).Order("media_file_id").Find(&files).Error; err != nil {
		return err
	}
	for _, file := range files {
		if err := s.evaluate(ctx, tx, file, policy); err != nil {
			return err
		}
	}
	return nil
}
