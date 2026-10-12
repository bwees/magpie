package transcode

import (
	"context"
	"errors"
	"slices"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
)

// keptLanguages are the audio and subtitle languages a transcode copies. All keeps every stream, for a
// title Catalog no longer knows.
type keptLanguages struct {
	All       bool
	Languages []domain.Language
}

// undetermined is how Library tags a stream without a language; such streams are always kept.
const undetermined domain.Language = "und"

// languagesToKeep reads the title's wanted languages (its override or the global list) and its original
// language from Catalog.
func languagesToKeep(ctx context.Context, catalog catalogapi.Reader, title domain.TitleID) (keptLanguages, error) {
	t, err := catalog.Title(ctx, title)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return keptLanguages{All: true}, nil
	}
	if err != nil {
		return keptLanguages{}, err
	}
	wanted, err := catalog.WantedLanguages(ctx, title)
	if err != nil {
		return keptLanguages{}, err
	}
	keep := keptLanguages{Languages: []domain.Language{undetermined}}
	if t.OriginalLanguage != "" {
		keep.Languages = append(keep.Languages, t.OriginalLanguage)
	}
	for _, w := range wanted {
		keep.Languages = append(keep.Languages, w.Language)
	}
	return keep, nil
}

func (k keptLanguages) keeps(s libraryapi.Stream) bool {
	return k.All || slices.Contains(k.Languages, s.Language)
}

// keptTracks picks the audio and subtitle streams in the kept languages, in source order. A file that
// would lose every audio stream keeps them all.
func keptTracks(info libraryapi.MediaInfo, keep keptLanguages) (audio, subs []int) {
	audio, subs = []int{}, []int{}
	for _, s := range info.Streams {
		switch {
		case s.Kind == libraryapi.StreamAudio && keep.keeps(s):
			audio = append(audio, s.Index)
		case s.Kind == libraryapi.StreamSubtitle && keep.keeps(s):
			subs = append(subs, s.Index)
		}
	}
	if len(audio) == 0 {
		audio = streamsOf(info, libraryapi.StreamAudio)
	}
	return audio, subs
}
