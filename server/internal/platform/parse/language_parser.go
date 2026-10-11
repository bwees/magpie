// Copyright (C) Magpie contributors.
// Portions ported from Sonarr (https://github.com/Sonarr/Sonarr) and
// Radarr (https://github.com/Radarr/Radarr), copyright their contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"slices"
	"strings"

	"magpie/internal/domain"
)

const (
	langEnglish          domain.Language = "en"
	langFrench           domain.Language = "fr"
	langSpanish          domain.Language = "es"
	langGerman           domain.Language = "de"
	langItalian          domain.Language = "it"
	langDanish           domain.Language = "da"
	langDutch            domain.Language = "nl"
	langJapanese         domain.Language = "ja"
	langIcelandic        domain.Language = "is"
	langChinese          domain.Language = "zh"
	langRussian          domain.Language = "ru"
	langPolish           domain.Language = "pl"
	langVietnamese       domain.Language = "vi"
	langSwedish          domain.Language = "sv"
	langNorwegian        domain.Language = "no"
	langFinnish          domain.Language = "fi"
	langTurkish          domain.Language = "tr"
	langPortuguese       domain.Language = "pt"
	langFlemish          domain.Language = "nl-BE"
	langGreek            domain.Language = "el"
	langKorean           domain.Language = "ko"
	langHungarian        domain.Language = "hu"
	langHebrew           domain.Language = "he"
	langLithuanian       domain.Language = "lt"
	langCzech            domain.Language = "cs"
	langArabic           domain.Language = "ar"
	langHindi            domain.Language = "hi"
	langBulgarian        domain.Language = "bg"
	langMalayalam        domain.Language = "ml"
	langUkrainian        domain.Language = "uk"
	langSlovak           domain.Language = "sk"
	langThai             domain.Language = "th"
	langPortugueseBrazil domain.Language = "pt-BR"
	langSpanishLatino    domain.Language = "es-MX"
	langRomanian         domain.Language = "ro"
	langLatvian          domain.Language = "lv"
	langPersian          domain.Language = "fa"
	langCatalan          domain.Language = "ca"
	langTamil            domain.Language = "ta"
	langAzerbaijani      domain.Language = "az"
	langUzbek            domain.Language = "uz"
	langUrdu             domain.Language = "ur"
	langRomansh          domain.Language = "rm"
	langGeorgian         domain.Language = "ka"
	langIrish            domain.Language = "ga"
	langBengali          domain.Language = "bn"
	langTelugu           domain.Language = "te"
	langKannada          domain.Language = "kn"
	langAlbanian         domain.Language = "sq"
	langAfrikaans        domain.Language = "af"
	langMarathi          domain.Language = "mr"
	langTagalog          domain.Language = "tl"
	langMongolian        domain.Language = "mn"

	// langOriginal marks "the title's original language" while parsing; it never leaves the package.
	langOriginal domain.Language = "ORIGINAL"
)

// languages is what a release name says about its audio languages.
type languages struct {
	list     []domain.Language
	original bool
}

type languageWord struct {
	words    []string
	language domain.Language
}

type languageGroup struct {
	group    string
	language domain.Language
}

const seriesLanguagesOnly = `(?<english>\b(?:ing|eng)\b)|(?<italian>\b(?:ita|italian)\b)|(?<german>(?:swiss)?german\b|videomann|ger[. ]dub|\bger\b)|(?<flemish>flemish)|(?<greek>greek)|(?<french>(?:_|\b)(?:FR|VF|VF2|VFF|VFI|VFQ|TRUEFRENCH|FRENCH|FRE|FRA)(?:_|\b))|(?<russian>\b(?:rus|ru)\b)|(?<hungarian>\b(?:HUNDUB|HUN)\b)|(?<hebrew>\bHebDub\b)|(?<polish>\b(?:PL\W?DUB|DUB\W?PL|LEK\W?PL|PL\W?LEK)\b)|(?<chinese>\[(?:CH[ST]|BIG5|GB)\]|简|繁|字幕|国语音轨[.+])|(?<bulgarian>\bbgaudio\b)|(?<spanish>\b(?:español|castellano|esp|spa(?!\(Latino\)))\b)|(?<ukrainian>\b(?:\dx?)?(?:ukr))|(?<thai>\b(?:THAI)\b)|(?<romanian>\b(?:RoDubbed|ROMANIAN)\b)|(?<catalan>[-,. ]cat[. ](?:DD|subs)|\b(?:catalan|catalán)\b)|(?<latvian>\b(?:lat|lav|lv)\b)|(?<turkish>\b(?:tur)\b)|(?<urdu>\burdu\b)|(?<romansh>\b(?:romansh|rumantsch|romansch)\b)|(?<georgian>\b(?:geo|ka|kat|georgian)\b)|(?<irish>\b(?:irish|gaeilge)\b)|(?<japanese>\(JA\)|JAP|JPN)|(?<portuguese>[_. ]por[_. ])|(?<original>\b(?:orig|original)\b)`

var (
	seriesLanguageCleanRegex = mustPattern(`.*?[_. ](S\d{2}(?:E\d{2,4})*[_. ].*)`, ignoreCase).gated(seasonTokenGate)
	seriesLanguageRegex      = mustPattern(`(?:`+seriesLanguagesOnly+`)(?!(?:[-_. ](?:`+seriesLanguagesOnly+`))*[-_. ]subs?)`, ignoreCase).gated(wordGate(true, []string{
		"ing", "eng", "ita", "italian", "ger", "fr", "vf", "vf2", "vff", "vfi", "vfq", "truefrench",
		"french", "fre", "fra", "rus", "ru", "hundub", "hun", "hebdub", "castellano", "esp", "spa",
		"thai", "rodubbed", "romanian", "cat", "catalan", "lat", "lav", "lv", "tur", "urdu", "romansh",
		"rumantsch", "romansch", "geo", "ka", "kat", "georgian", "irish", "gaeilge", "por", "orig", "original",
	}, "german", "videomann", "flemish", "greek", "dub", "lek", "[ch", "[big5", "[gb", "bgaudio", "ukr", "(ja)", "jap", "jpn"))
	seriesCaseSensitiveLanguageRegex = mustPattern(`(?:(?i)(?<!SUB[\W|_|^]))(?:(?<lithuanian>\bLT\b)|(?<czech>\bCZ\b)|(?<polish>\bPL\b)|(?<bulgarian>\bBG\b)|(?<slovak>\bSK\b)|(?<german>\bDE\b))(?:(?i)(?![\W|_|^]SUB))`, caseSensitive).gated(wordGate(false, []string{"LT", "CZ", "PL", "BG", "SK", "DE"}))

	movieLanguageRegex = mustPattern(`(?:\W|_|^)(?<english>\beng\b)|
		(?<italian>\b(?:ita|italian)\b)|
		(?<german>(?:swiss)?german\b|videomann|ger[. ]dub|\bger\b)|
		(?<flemish>flemish)|
		(?<bulgarian>bgaudio)|
		(?<romanian>rodubbed)|
		(?<brazilian>\b(dublado|pt-BR)\b)|
		(?<greek>greek)|
		(?<french>\b(?:FR|VO|VF|VFF|VFQ|VFI|VF2|TRUEFRENCH|FRENCH|FRE|FRA)\b)|
		(?<russian>\b(?:rus|ru)\b)|
		(?<hungarian>\b(?:HUNDUB|HUN)\b)|
		(?<hebrew>\b(?:HebDub|HebDubbed)\b)|
		(?<polish>\b(?:PL\W?DUB|DUB\W?PL|LEK\W?PL|PL\W?LEK)\b)|
		(?<chinese>\[(?:CH[ST]|BIG5|GB)\]|简|繁|字幕)|
		(?<ukrainian>(?:(?:\dx)?UKR))|
		(?<spanish>\b(?:español|castellano)\b)|
		(?<catalan>\b(?:catalan?|catalán|català)\b)|
		(?<latvian>\b(?:lat|lav|lv)\b)|
		(?<telugu>\btel\b)|
		(?<vietnamese>\bVIE\b)|
		(?<japanese>\bJAP\b)|
		(?<korean>\bKOR\b)|
		(?<urdu>\burdu\b)|
		(?<romansh>\b(?:romansh|rumantsch|romansch)\b)|
		(?<mongolian>\b(?:mongolian|khalkha)\b)|
		(?<georgian>\b(?:georgian|geo|ka|kat)\b)|
		(?<original>\b(?:orig|original)\b)`, ignoreCase|ignoreWhitespace).gated(wordGate(true, []string{
		"eng", "ita", "italian", "ger", "dublado", "fr", "vo", "vf", "vff", "vfq", "vfi", "vf2",
		"truefrench", "french", "fre", "fra", "rus", "ru", "hundub", "hun", "hebdub", "hebdubbed",
		"castellano", "catala", "catalan", "lat", "lav", "lv", "tel", "vie", "jap", "kor", "urdu",
		"romansh", "rumantsch", "romansch", "mongolian", "khalkha", "georgian", "geo", "ka", "kat",
		"orig", "original",
	}, "german", "videomann", "flemish", "bgaudio", "rodubbed", "pt-br", "greek", "dub", "lek", "[ch", "[big5", "[gb", "ukr"))
	movieCaseSensitiveLanguageRegex = mustPattern(`(?:(?i)(?<!SUB[\W|_|^]))(?:(?<english>\bEN\b)|
		(?<lithuanian>\bLT\b)|
		(?<czech>\bCZ\b)|
		(?<polish>\bPL\b)|
		(?<bulgarian>\bBG\b)|
		(?<slovak>\bSK\b)|
		(?<german>\bDE\b)|
		(?<spanish>\b(?<!DTS[._ -])ES\b))(?:(?i)(?![\W|_|^]SUB))`, ignoreWhitespace).gated(wordGate(false, []string{"EN", "LT", "CZ", "PL", "BG", "SK", "DE", "ES"}))

	germanDualLanguageRegex  = mustPattern(`(?<!WEB[-_. ]?)\bDL\b`, ignoreCase)
	germanMultiLanguageRegex = mustPattern(`\bML\b`, ignoreCase)
)

func seriesLanguageWords() []languageWord {
	return []languageWord{
		{[]string{"spanish"}, langSpanish},
		{[]string{"danish"}, langDanish},
		{[]string{"dutch"}, langDutch},
		{[]string{"japanese"}, langJapanese},
		{[]string{"icelandic"}, langIcelandic},
		{[]string{"mandarin", "cantonese", "chinese"}, langChinese},
		{[]string{"korean"}, langKorean},
		{[]string{"russian"}, langRussian},
		{[]string{"polish"}, langPolish},
		{[]string{"vietnamese"}, langVietnamese},
		{[]string{"swedish"}, langSwedish},
		{[]string{"norwegian"}, langNorwegian},
		{[]string{"finnish"}, langFinnish},
		{[]string{"turkish"}, langTurkish},
		{[]string{"portuguese"}, langPortuguese},
		{[]string{"hungarian"}, langHungarian},
		{[]string{"hebrew"}, langHebrew},
		{[]string{"arabic"}, langArabic},
		{[]string{"hindi"}, langHindi},
		{[]string{"malayalam"}, langMalayalam},
		{[]string{"ukrainian"}, langUkrainian},
		{[]string{"bulgarian"}, langBulgarian},
		{[]string{"slovak"}, langSlovak},
		{[]string{"brazilian", "dublado"}, langPortugueseBrazil},
		{[]string{"latino"}, langSpanishLatino},
		{[]string{"latvian"}, langLatvian},
		{[]string{"azerbaijani", "azerbaijan"}, langAzerbaijani},
		{[]string{"uzbek", "uzbekistan"}, langUzbek},
	}
}

func seriesCaseSensitiveGroups() []languageGroup {
	return []languageGroup{
		{"lithuanian", langLithuanian},
		{"czech", langCzech},
		{"polish", langPolish},
		{"bulgarian", langBulgarian},
		{"slovak", langSlovak},
		{"german", langGerman},
	}
}

func seriesLanguageGroups() []languageGroup {
	return []languageGroup{
		{"english", langEnglish},
		{"italian", langItalian},
		{"german", langGerman},
		{"flemish", langFlemish},
		{"greek", langGreek},
		{"french", langFrench},
		{"russian", langRussian},
		{"hungarian", langHungarian},
		{"hebrew", langHebrew},
		{"polish", langPolish},
		{"chinese", langChinese},
		{"bulgarian", langBulgarian},
		{"ukrainian", langUkrainian},
		{"spanish", langSpanish},
		{"thai", langThai},
		{"romanian", langRomanian},
		{"catalan", langCatalan},
		{"latvian", langLatvian},
		{"turkish", langTurkish},
		{"urdu", langUrdu},
		{"romansh", langRomansh},
		{"georgian", langGeorgian},
		{"irish", langIrish},
		{"japanese", langJapanese},
		{"portuguese", langPortuguese},
		{"original", langOriginal},
	}
}

func movieLanguageWords() []languageWord {
	return []languageWord{
		{[]string{"english"}, langEnglish},
		{[]string{"spanish"}, langSpanish},
		{[]string{"danish"}, langDanish},
		{[]string{"dutch"}, langDutch},
		{[]string{"japanese"}, langJapanese},
		{[]string{"icelandic"}, langIcelandic},
		{[]string{"mandarin", "cantonese", "chinese"}, langChinese},
		{[]string{"korean"}, langKorean},
		{[]string{"russian"}, langRussian},
		{[]string{"romanian"}, langRomanian},
		{[]string{"hindi"}, langHindi},
		{[]string{"arabic"}, langArabic},
		{[]string{"thai"}, langThai},
		{[]string{"bulgarian"}, langBulgarian},
		{[]string{"polish"}, langPolish},
		{[]string{"vietnamese"}, langVietnamese},
		{[]string{"swedish"}, langSwedish},
		{[]string{"norwegian"}, langNorwegian},
		{[]string{"finnish"}, langFinnish},
		{[]string{"turkish"}, langTurkish},
		{[]string{"portuguese"}, langPortuguese},
		{[]string{"brazilian"}, langPortugueseBrazil},
		{[]string{"hungarian"}, langHungarian},
		{[]string{"hebrew"}, langHebrew},
		{[]string{"ukrainian"}, langUkrainian},
		{[]string{"persian"}, langPersian},
		{[]string{"bengali"}, langBengali},
		{[]string{"slovak"}, langSlovak},
		{[]string{"latvian"}, langLatvian},
		{[]string{"latino"}, langSpanishLatino},
		{[]string{"tamil"}, langTamil},
		{[]string{"telugu"}, langTelugu},
		{[]string{"malayalam"}, langMalayalam},
		{[]string{"kannada"}, langKannada},
		{[]string{"albanian"}, langAlbanian},
		{[]string{"afrikaans"}, langAfrikaans},
		{[]string{"marathi"}, langMarathi},
		{[]string{"tagalog"}, langTagalog},
	}
}

func movieCaseSensitiveGroups() []languageGroup {
	return []languageGroup{
		{"english", langEnglish},
		{"lithuanian", langLithuanian},
		{"czech", langCzech},
		{"polish", langPolish},
		{"bulgarian", langBulgarian},
		{"slovak", langSlovak},
		{"spanish", langSpanish},
		{"german", langGerman},
	}
}

func movieLanguageGroups() []languageGroup {
	return []languageGroup{
		{"english", langEnglish},
		{"italian", langItalian},
		{"german", langGerman},
		{"flemish", langFlemish},
		{"greek", langGreek},
		{"french", langFrench},
		{"russian", langRussian},
		{"bulgarian", langBulgarian},
		{"brazilian", langPortugueseBrazil},
		{"hungarian", langHungarian},
		{"hebrew", langHebrew},
		{"polish", langPolish},
		{"chinese", langChinese},
		{"spanish", langSpanish},
		{"catalan", langCatalan},
		{"ukrainian", langUkrainian},
		{"latvian", langLatvian},
		{"romanian", langRomanian},
		{"telugu", langTelugu},
		{"vietnamese", langVietnamese},
		{"japanese", langJapanese},
		{"korean", langKorean},
		{"urdu", langUrdu},
		{"romansh", langRomansh},
		{"mongolian", langMongolian},
		{"georgian", langGeorgian},
		{"original", langOriginal},
	}
}

func appendWordLanguages(found []domain.Language, lowerTitle string, words []languageWord) []domain.Language {
	for _, w := range words {
		for _, word := range w.words {
			if strings.Contains(lowerTitle, word) {
				found = append(found, w.language)
				break
			}
		}
	}
	return found
}

func appendGroupLanguages(found []domain.Language, m *match, groups []languageGroup) []domain.Language {
	for _, g := range groups {
		if m.success(g.group) {
			found = append(found, g.language)
		}
	}
	return found
}

// parseSeriesLanguages ports Sonarr's LanguageParser.ParseLanguages.
func parseSeriesLanguages(title string) languages {
	seriesLanguageCleanRegex.tryReplace(&title, "$1")
	lowerTitle := strings.ToLower(title)

	found := appendWordLanguages(nil, lowerTitle, seriesLanguageWords())
	if m := seriesCaseSensitiveLanguageRegex.match(title); m != nil {
		found = appendGroupLanguages(found, m, seriesCaseSensitiveGroups())
	}
	for _, m := range seriesLanguageRegex.matches(title) {
		found = appendGroupLanguages(found, m, seriesLanguageGroups())
	}
	if strings.Contains(lowerTitle, "english") {
		found = append(found, langEnglish)
	}
	return finishLanguages(found, title)
}

// parseMovieLanguages ports Radarr's LanguageParser.ParseLanguages.
func parseMovieLanguages(title string) languages {
	lowerTitle := strings.ToLower(title)

	found := appendWordLanguages(nil, lowerTitle, movieLanguageWords())
	for _, m := range movieCaseSensitiveLanguageRegex.matches(title) {
		found = appendGroupLanguages(found, m, movieCaseSensitiveGroups())
	}
	for _, m := range movieLanguageRegex.matches(title) {
		found = appendGroupLanguages(found, m, movieLanguageGroups())
	}
	return finishLanguages(found, title)
}

// finishLanguages applies the German DL/ML rule and removes duplicates, keeping first-seen order.
func finishLanguages(found []domain.Language, title string) languages {
	if len(found) == 1 && found[0] == langGerman {
		switch {
		case germanDualLanguageRegex.isMatch(title):
			found = append(found, langOriginal)
		case germanMultiLanguageRegex.isMatch(title):
			found = append(found, langOriginal, langEnglish)
		}
	}

	var result languages
	for _, l := range found {
		if l == langOriginal {
			result.original = true
			continue
		}
		if !slices.Contains(result.list, l) {
			result.list = append(result.list, l)
		}
	}
	return result
}
