// Copyright (C) Magpie contributors.
// Portions ported from Sonarr (https://github.com/Sonarr/Sonarr) and
// Radarr (https://github.com/Radarr/Radarr), copyright their contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"strconv"
	"strings"
)

// The fixtures compare titles after Sonarr's CleanSeriesTitle or Radarr's CleanMovieTitle.
// Title matching belongs to the callers, so these ports live with the tests.

var (
	percentRegex          = mustPattern(`(?<=\b\d+)%`, caseSensitive)
	seriesNormalizeRegex  = mustPattern(`((?:\b|_)(?<!^)([aà](?!$)|an|the|and|or|of)(?!$)(?:\b|_))|\W|_`, ignoreCase)
	movieNormalizeRegex   = mustPattern(`((?:\b|_)(?<!^|[^a-zA-Z0-9_']\w[^a-zA-Z0-9_'])([aà](?!$|[^a-zA-Z0-9_']\w[^a-zA-Z0-9_'])|an|the|and|or|of)(?!$)(?:\b|_))|\W|_`, ignoreCase)
	germanUmlautsReplacer = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "Ä", "Ae", "Ö", "Oe", "Ü", "Ue", "ß", "ss")
	// diacriticsReplacer covers the decomposable lowercase Latin letters in the fixtures.
	diacriticsReplacer = strings.NewReplacer(
		"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a",
		"ç", "c", "è", "e", "é", "e", "ê", "e", "ë", "e",
		"ì", "i", "í", "i", "î", "i", "ï", "i", "ñ", "n",
		"ò", "o", "ó", "o", "ô", "o", "õ", "o", "ö", "o",
		"ù", "u", "ú", "u", "û", "u", "ü", "u", "ý", "y", "ÿ", "y",
	)
)

func isLongInteger(s string) bool {
	_, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return err == nil
}

func cleanSeriesTitle(title string) string {
	if strings.TrimSpace(title) == "" || isLongInteger(title) {
		return title
	}
	title = percentRegex.replace(title, "percent")
	return diacriticsReplacer.Replace(strings.ToLower(seriesNormalizeRegex.replace(title, "")))
}

func cleanMovieTitle(title string) string {
	if strings.TrimSpace(title) == "" || isLongInteger(title) {
		return title
	}
	return diacriticsReplacer.Replace(germanUmlautsReplacer.Replace(strings.ToLower(movieNormalizeRegex.replace(title, ""))))
}
