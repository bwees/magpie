package parse

import (
	"testing"

	"magpie/internal/domain"
)

func TestLanguageCodeReadsCodesAndNames(t *testing.T) {
	for in, want := range map[string]domain.Language{
		"en": "en", "eng": "en", "ger": "de", "deu": "de", "FRE": "fr", "English": "en", "german": "de",
	} {
		if got, ok := LanguageCode(in); !ok || got != want {
			t.Errorf("LanguageCode(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "the", "dts", "x264", "grp", "web", "movie"} {
		if got, ok := LanguageCode(in); ok {
			t.Errorf("LanguageCode(%q) = %q, want no language", in, got)
		}
	}
}

func TestParseSubtitleLanguage(t *testing.T) {
	for path, want := range map[string]SubtitleLanguage{
		"Movie.en.srt":                   {Language: "en"},
		"Movie.eng.forced.srt":           {Language: "en", Forced: true},
		"Movie.en.sdh.srt":               {Language: "en", SDH: true},
		"Movie.en.hi.srt":                {Language: "en", SDH: true},
		"Movie.hi.srt":                   {Language: "hi"},
		"Movie.pt-BR.srt":                {Language: "pt"},
		"Subs/Show.S01E01/2_English.srt": {Language: "en"},
		"Movie.French.Forced.ass":        {Language: "fr", Forced: true},
	} {
		if got, ok := ParseSubtitleLanguage(path); !ok || got != want {
			t.Errorf("ParseSubtitleLanguage(%q) = %+v, %v; want %+v", path, got, ok, want)
		}
	}
	for _, path := range []string{"Movie.srt", "Show.S01E01.1080p.WEB.x264-GRP.srt", "forced.srt"} {
		if got, ok := ParseSubtitleLanguage(path); ok {
			t.Errorf("ParseSubtitleLanguage(%q) = %+v, want no language", path, got)
		}
	}
}
