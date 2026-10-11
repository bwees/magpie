// Copyright (C) Magpie contributors.
// Portions ported from Sonarr (https://github.com/Sonarr/Sonarr) and
// Radarr (https://github.com/Radarr/Radarr), copyright their contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"strconv"
	"strings"

	"magpie/internal/domain"
)

type releaseGroupRules struct {
	kind  domain.MediaKind
	group pattern
	// Sonarr checks the bracketed exception list before the exact one; Radarr the other way round.
	exceptions []pattern
	clean      []pattern
	// Radarr's parser has no pre-substitutions.
	preSubstitutions []preSubstitution
}

var (
	invalidReleaseGroupRegex = mustPattern(`^([se]\d+|[0-9a-f]{8})$`, ignoreCase)
	animeReleaseGroupRegex   = mustPattern(`^(?:\[(?<subgroup>(?!\s).+?(?<!\s))\](?:_|-|\s|\.)?)`, ignoreCase)
)

var seriesReleaseGroupRules = releaseGroupRules{
	kind:  domain.MediaKindSeries,
	group: mustPattern(`-(?<releasegroup>[A-Za-zÀ-ÖØ-öø-ÿ0-9]+(?<part2>-[A-Za-zÀ-ÖØ-öø-ÿ0-9]+)?(?!.+?(?:HDTV|SDTV|480p|576p|720p|1080p|2160p)))(?<!(?:HDTV|SDTV|WEB-DL|Blu-Ray|480p|576p|720p|1080p|2160p|DTS-HD|DTS-X|DTS-MA|DTS-ES|-ES|-EN|-CAT|-GER|-FRA|-FRE|-ITA|\d{1,2}-bit|[ ._]\d{4}-\d{2}|-\d{2})(?:\k<part2>)?)(?:\b|[-._ ]|$)|[-._ ]\[(?<releasegroup>[A-Za-zÀ-ÖØ-öø-ÿ0-9]+)\]$`, ignoreCase),
	exceptions: []pattern{
		mustPattern(`(?<=[._ \[])(?<releasegroup>(Joy|ImE|UTR|t3nzin|Anime Time|Project Angel|Hakata Ramen|HONE|Vyndros|SEV|Garshasp|Kappa|Natty|RCVR|SAMPA|YOGI|r00t|EDGE2020|Celdra)(?=\]|\)))`, ignoreCase).gated(exactAny("]", ")")),
		mustPattern(`(?:(?<releasegroup>Fight-BB|VARYG|E\.N\.D|KRaLiMaRKo|BluDragon|DarQ|KCRT|BEN[_. ]THE[_. ]MEN|TAoE|QxR|Vialle)\b)`, ignoreCase).gated(foldAny("fight-bb", "varyg", "e.n.d", "kralimarko", "bludragon", "darq", "kcrt", "ben", "taoe", "qxr", "vialle")),
	},
	// Sonarr's CleanReleaseGroupRegex is `^(PREFIX)|(SUFFIX)+$`. The prefix branch can only
	// match at the start and the suffix branch only at the end, so replacing the prefix and
	// then the suffix gives the same result while letting each regex skip most positions.
	clean: []pattern{
		mustPattern(`^(.*?[-._ ](S\d+E\d+)[-._ ])`, ignoreCase).gated(seasonEpisodeGate),
		mustPattern(`(-(RP|1|NZBGeek|\[N-Z-B\]|Obfuscated|Scrambled|sample|Pre|postbot|xpost|Rakuv[a-z0-9]*|WhiteRev|BUYMORE|AsRequested|AlternativeToRequested|GEROV|Z0iDS3N|Chamele0n|4P|4Planet|AlteZachen|RePACKPOST))+$`, ignoreCase).gated(cleanSuffixGate),
	},
	preSubstitutions: seriesPreSubstitutions,
}

var movieReleaseGroupRules = releaseGroupRules{
	kind:  domain.MediaKindMovie,
	group: mustPattern(`-(?<releasegroup>[a-z0-9]+(?<part2>-[a-z0-9]+)?(?!.+?(?:480p|576p|720p|1080p|2160p)))(?<!(?:WEB-(DL|Rip)|Blu-Ray|480p|576p|720p|1080p|2160p|DTS-HD|DTS-X|DTS-MA|DTS-ES|-ES|-EN|-CAT|-ENG|-JAP|-GER|-FRA|-FRE|-ITA|-HDRip|\d{1,2}-bit|[ ._]\d{4}-\d{2}|-\d{2}|tmdb(id)?-(?<tmdbid>\d+)|(?<imdbid>tt\d{7,8}))(?:\k<part2>)?)(?:\b|[-._ ]|$)|[-._ ]\[(?<releasegroup>[a-z0-9]+)\]$`, ignoreCase),
	exceptions: []pattern{
		mustPattern(`\b(?<releasegroup>KRaLiMaRKo|E\.N\.D|D\-Z0N3|Koten_Gars|BluDragon|ZØNEHD|HQMUX|VARYG|YIFY|YTS(.(MX|LT|AG))?|TMd|Eml HDTeam|LMain|DarQ|BEN THE MEN|TAoE|QxR|126811)\b`, ignoreCase).gated(foldAny("kralimarko", "e.n.d", "d-z0n3", "koten_gars", "bludragon", "hqmux", "varyg", "yify", "yts", "tmd", "eml hdteam", "lmain", "darq", "ben the men", "taoe", "qxr", "126811")),
		mustPattern(`(?<=[._ \[])(?<releasegroup>(Silence|afm72|Panda|Ghost|MONOLITH|Tigole|Joy|ImE|UTR|t3nzin|Anime Time|Project Angel|Hakata Ramen|HONE|GiLG|Vyndros|SEV|Garshasp|Kappa|Natty|RCVR|SAMPA|YOGI|r00t|EDGE2020|RZeroX|FreetheFish|Anna|Bandi|Qman|theincognito|HDO|DusIctv|DHD|CtrlHD|-ZR-|ADC|XZVN|RH|Kametsu|Celdra)(?=\]|\)))`, ignoreCase).gated(exactAny("]", ")")),
	},
	clean: []pattern{
		mustPattern(`(-(RP|1|NZBGeek|Obfuscated|Obfuscation|Scrambled|sample|Pre|postbot|xpost|Rakuv[a-z0-9]*|WhiteRev|BUYMORE|AsRequested|AlternativeToRequested|GEROV|Z0iDS3N|Chamele0n|4P|4Planet|AlteZachen|RePACKPOST))+$`, ignoreCase).gated(cleanSuffixGate),
	},
}

// parseReleaseGroup ports ReleaseGroupParser.ParseReleaseGroup. An empty result means no group.
func parseReleaseGroup(title string, rules releaseGroupRules) string {
	title = removeFileExtension(strings.TrimSpace(title), rules.kind)
	for _, sub := range rules.preSubstitutions {
		if sub.regex.tryReplace(&title, sub.replacement) {
			break
		}
	}

	title = websitePrefixRegex.replace(title, "")
	title = cleanTorrentSuffixRegex.replace(title, "")

	if m := animeReleaseGroupRegex.match(title); m != nil {
		return m.value("subgroup")
	}

	for _, clean := range rules.clean {
		title = clean.replace(title, "")
	}

	for _, exception := range rules.exceptions {
		if m := exception.lastMatch(title); m != nil {
			return m.value("releasegroup")
		}
	}

	m := rules.group.lastMatch(title)
	if m == nil {
		return ""
	}
	group := m.value("releasegroup")
	if _, err := strconv.Atoi(group); err == nil {
		return ""
	}
	if invalidReleaseGroupRegex.isMatch(group) {
		return ""
	}
	return group
}
