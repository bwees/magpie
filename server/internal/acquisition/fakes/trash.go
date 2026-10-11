package fakes

import (
	"context"
	"slices"
	"sync"

	"magpie/internal/acquisition"
	"magpie/internal/platform/scripted"
)

const (
	TrashGuidesCustomFormats   scripted.Method = "TrashGuides.CustomFormats"
	TrashGuidesQualityProfiles scripted.Method = "TrashGuides.QualityProfiles"
	TrashGuidesQualitySizes    scripted.Method = "TrashGuides.QualitySizes"
)

type trashGuide struct {
	formats  []acquisition.TrashCustomFormat
	profiles []acquisition.TrashQualityProfile
	sizes    []acquisition.TrashQualitySize
}

// TrashGuides serves the guide data set per app. An app with nothing set returns empty lists.
type TrashGuides struct {
	scripted.Script

	mu     sync.RWMutex
	guides map[acquisition.TrashApp]trashGuide
}

var _ acquisition.TrashGuides = (*TrashGuides)(nil)

func NewTrashGuides() *TrashGuides {
	return &TrashGuides{guides: map[acquisition.TrashApp]trashGuide{}}
}

func (g *TrashGuides) SetCustomFormats(app acquisition.TrashApp, formats ...acquisition.TrashCustomFormat) {
	g.update(app, func(guide *trashGuide) { guide.formats = formats })
}

func (g *TrashGuides) SetQualityProfiles(app acquisition.TrashApp, profiles ...acquisition.TrashQualityProfile) {
	g.update(app, func(guide *trashGuide) { guide.profiles = profiles })
}

func (g *TrashGuides) SetQualitySizes(app acquisition.TrashApp, sizes ...acquisition.TrashQualitySize) {
	g.update(app, func(guide *trashGuide) { guide.sizes = sizes })
}

func (g *TrashGuides) CustomFormats(ctx context.Context, app acquisition.TrashApp) ([]acquisition.TrashCustomFormat, error) {
	if err := g.Enter(ctx, TrashGuidesCustomFormats); err != nil {
		return nil, err
	}
	return slices.Clone(g.guide(app).formats), nil
}

func (g *TrashGuides) QualityProfiles(ctx context.Context, app acquisition.TrashApp) ([]acquisition.TrashQualityProfile, error) {
	if err := g.Enter(ctx, TrashGuidesQualityProfiles); err != nil {
		return nil, err
	}
	return slices.Clone(g.guide(app).profiles), nil
}

func (g *TrashGuides) QualitySizes(ctx context.Context, app acquisition.TrashApp) ([]acquisition.TrashQualitySize, error) {
	if err := g.Enter(ctx, TrashGuidesQualitySizes); err != nil {
		return nil, err
	}
	return slices.Clone(g.guide(app).sizes), nil
}

func (g *TrashGuides) guide(app acquisition.TrashApp) trashGuide {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.guides[app]
}

func (g *TrashGuides) update(app acquisition.TrashApp, fn func(*trashGuide)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	guide := g.guides[app]
	fn(&guide)
	g.guides[app] = guide
}
