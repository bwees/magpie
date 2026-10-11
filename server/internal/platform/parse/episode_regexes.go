// Copyright (C) Magpie contributors.
// Portions ported from Sonarr (https://github.com/Sonarr/Sonarr), copyright its contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

// episodeTitleRegexes is Sonarr's ReportTitleRegex list. Order matters: the first match wins.
// Anime patterns stay in place so the patterns after them see the same names as in Sonarr.
var episodeTitleRegexes = []episodeRegex{
	// Daily episode with year in series title and air time after date (Plex DVR format)
	{needsParen, mustPattern(`^^(?<title>.+?\((?<titleyear>\d{4})\))[-_. ]+(?<airyear>19[4-9]\d|20\d\d)(?<sep>[-_]?)(?<airmonth>0\d|1[0-2])\k<sep>(?<airday>[0-2]\d|3[01])[-_. ]\d{2}[-_. ]\d{2}[-_. ]\d{2}`, ignoreCase)},

	// Daily episodes without title (2018-10-12, 20181012)
	{always, mustPattern(`^(?<airyear>19[6-9]\d|20\d\d)(?<sep>[-_]?)(?<airmonth>0\d|1[0-2])\k<sep>(?<airday>[0-2]\d|3[01])(?!\d)`, ignoreCase)},

	// Multi-Part episodes without a title (S01E05.S01E06)
	{always, mustPattern(`^(?:\W*S(?<season>(?<!\d+)(?:\d{1,2}|\d{4})(?!\d+))(?:e{1,2}(?<episode>\d{1,3}(?!\d+)))+){2,}`, ignoreCase)},

	// Multi-Part episodes without a title (1x05.1x06)
	{always, mustPattern(`^(?:\W*(?<season>(?<!\d+)(?:\d{1,2}|\d{4})(?!\d+))(?:x{1,2}(?<episode>\d{1,3}(?!\d+)))+){2,}`, ignoreCase)},

	// Episodes without a title, Multi (S01E04E05, 1x04x05, etc)
	{always, mustPattern(`^(?:S?(?<season>(?<!\d+)(?:\d{1,2}|\d{4})(?!\d+))(?:(?:[-_]|[ex]){1,2}(?<episode>\d{2,3}(?!\d+))){2,})`, ignoreCase)},

	// Split episodes (S01E05a, S01E05b, etc)
	{needsSplitEpisode, mustPattern(`^(?<title>.+?)(?:S?(?<season>(?<!\d+)(?:\d{1,2}|\d{4})(?!\d+))(?:(?:[-_ ]?[ex])(?<episode>\d{2,3}(?!\d+))(?<splitepisode>[a-d])(?:[ _.])))`, ignoreCase)},

	// Episodes without a title, Single (S01E05, 1x05)
	{always, mustPattern(`^(?:S?(?<season>(?<!\d+)(?:\d{1,2}|\d{4})(?!\d+))(?:(?:[-_ ]?[ex])(?<episode>\d{2,3}(?!\d+))))`, ignoreCase)},

	// Anime - [SubGroup] Title Absolute (Season+Episode)
	{bracketStart, mustPattern(`^(?:\[(?<subgroup>.+?)\](?:_|-|\s|\.)?)(?<title>.+?)[-_. ]+(?<absoluteepisode>(?<!\d+)\d{2,3}(\.\d{1,2})?(?!\d+))(?:[-_. ])+\((?:S(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:(?:[ex]|\W[ex]){1,2}(?<episode>\d{2}(?!\d+))))(?:v\d+)?(?:\)(?!\d+)).*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - [SubGroup] Title Absolute - Season+Episode
	{bracketStart, mustPattern(`^(?:\[(?<subgroup>.+?)\](?:_|-|\s|\.)?)(?<title>.+?)[-_. ]+(?<absoluteepisode>(?<!\d+)\d{2,3}(\.\d{1,2})?(?!\d+))(?:[-_. ](?<![()\[!]))+(?:S(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:(?:[ex]|\W[ex]){1,2}(?<episode>\d{2}(?!\d+))))(?:v\d+)?(?:[_. ](?!\d+)).*?(?<hash>[(\[][0-9A-F]{8}[)\]])$`, ignoreCase)},

	// Anime - [SubGroup] Title Season+Episode
	{bracketStart, mustPattern(`^(?:\[(?<subgroup>.+?)\](?:_|-|\s|\.)?)(?<title>.+?)(?:[-_\W](?<![()\[!]))+(?:S?(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:(?:[ex]|\W[ex]){1,2}(?<episode>\d{2}(?!\d+)))+)(?:v\d+)?(?:[_. ](?!\d+)).*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - [SubGroup] Title Episode Absolute Episode Number ([SubGroup] Series Title Episode 01)
	{bracketStart, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)(?<title>.+?)[-_. ]+?(?:Episode)(?:[-_. ]+(?<absoluteepisode>(?<!\d+)\d{2,3}(\.\d{1,2})?(?!\d+)))+.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - [SubGroup] Title Absolute Episode Number + Season+Episode
	{bracketStart, mustPattern(`^(?:\[(?<subgroup>.+?)\](?:_|-|\s|\.)?)(?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+(?<absoluteepisode>\d{2,3}(\.\d{1,2})?))+(?:_|-|\s|\.)+(?:S?(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:(?:\-|[ex]|\W[ex]){1,2}(?<episode>\d{2}(?!\d+)))+).*?(?<hash>[(\[][0-9A-F]{8}[)\]])?(?:$|\.)`, ignoreCase)},

	// Anime - [SubGroup] Title Multi-episode + optional Absolute Episode Number
	{bracketStart, mustPattern(`^(?:\[(?<subgroup>.+?)\](?:_|-|\s|\.)?)(?<title>.+?)(?:[-_\W](?<![()\[!]))+(?:S?(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:(?:\-|[ex]|\W[ex]){1,2}(?<episode>\d{2}(?!\d+))){2,})(?:(?:_|-|\s|\.)+(?<absoluteepisode>(?<!\d+)\d{2,3}(\.\d{1,2})?(?!\d+|\-[a-z])))*.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - [SubGroup] Title Single episode + Absolute Episode Number
	{bracketStart, mustPattern(`^(?:\[(?<subgroup>.+?)\](?:_|-|\s|\.)?)(?<title>.+?)(?:[-_\W](?<![()\[!]))+(?:S?(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:(?:\-|[ex]|\W[ex]){1,2}(?<episode>\d{2}(?!\d+))))(?:(?:_|-|\s|\.)+(?<absoluteepisode>(?<!\d+)\d{2,3}(\.\d{1,2})?(?!\d+|\-[a-z])))+.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - [SubGroup] Title with trailing number Absolute Episode Number - Batch separated with tilde
	{bracketStart, mustPattern(`^\[(?<subgroup>.+?)\][-_. ]?(?<title>.+?[^-]+?)(?:(?<![-_. ]|\b[0]\d+) - )[-_. ]?(?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+))\s?~\s?(?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+))(?:[-_. ]+(?<special>special|ova|ovd))?.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?(?:$|\.mkv)`, ignoreCase)},

	// Anime - [SubGroup] Title with season number in brackets Absolute Episode Number
	{bracketStart, mustPattern(`^\[(?<subgroup>.+?)\][-_. ]?(?<title>[^-]+?)[_. ]+?\(Season[_. ](?<season>\d+)\)[-_. ]+?(?:[-_. ]?(?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+)))+(?:[-_. ]+(?<special>special|ova|ovd))?.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?(?:$|\.mkv)`, ignoreCase)},

	// Anime - [SubGroup] Title with trailing 3-digit number and sub title - Absolute Episode Number
	{bracketStart, mustPattern(`^\[(?<subgroup>[^\]]+?)\][-_. ]?(?<title>[^]]+?)(?:[-_. ]{3}?(?<absoluteepisode>\d{2}(\.\d{1,2})?(?!-?\d+|-[a-z]+)))+(?:[-_. ]+(?<special>special|ova|ovd))?.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?(?:$|\.mkv)`, ignoreCase)},

	// Anime - [SubGroup] Multi-season pack
	{bracketStart, mustPattern(`^\[(?<subgroup>.+?)\][-_. ]?(?<title>.+?)[-_. ]+S(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))(?:(?:-S?|[-_. ]{1,3}S)(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))){1,}`, ignoreCase)},

	// Anime - [SubGroup] Partial season pack
	{bracketStart, mustPattern(`^\[(?<subgroup>.+?)\][-_. ]?(?<title>.+?)(?:\W+S(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))\W+(?:(?:(?:Part|Vol)\W?|(?<!\d+\W+)e|p)(?<seasonpart>\d{1,2}(?!\d+)))+)`, ignoreCase)},

	// Anime - [SubGroup] Title with trailing number Absolute Episode Number
	{bracketStart, mustPattern(`^\[(?<subgroup>.+?)\][-_. ]?(?<title>[^-]+?)(?:(?<![-_. ]|\b[0]\d+) - )(?:[-_. ]?(?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+)))+(?:[-_. ]+(?<special>special|ova|ovd))?.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?(?:$|\.mkv)`, ignoreCase)},

	// Anime - [SubGroup] Title with trailing number S## (Full season)
	{bracketStart, mustPattern(`^\[(?<subgroup>.+?)\][-_. ]?(?<title>.+?)[-_. ]+(?:S(?<season>(?<!\d+)(?:\d{1,2}|\d{4})(?![ex]?\d+))).+?(?:$|\.mkv)`, ignoreCase)},

	// Anime - [SubGroup] Title with trailing number Absolute Episode Number
	{bracketStart, mustPattern(`^\[(?<subgroup>.+?)\][-_. ]?(?<title>[^-]+?)(?:(?<![-_. ]|\b[0]\d+)[_ ]+)(?:[-_. ]?(?<absoluteepisode>\d{3}(\.\d{1,2})?(?!\d+|-[a-z]+)))+(?:[-_. ]+(?<special>special|ova|ovd))?.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?(?:$|\.mkv)`, ignoreCase)},

	// Anime - [SubGroup] Title - Absolute Episode Number
	{bracketStart, mustPattern(`^\[(?<subgroup>.+?)\][-_. ]?(?<title>.+?)(?:(?<!\b[0]\d+))(?:[. ]-[. ](?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+|[-])))+(?:[-_. ]+(?<special>special|ova|ovd))?.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?(?:$|\.mkv)`, ignoreCase)},

	// Anime - [SubGroup] Title Absolute Episode Number - Absolute Episode Number (batches without full separator between title and absolute episode numbers)
	{bracketStart, mustPattern(`^\[(?<subgroup>.+?)\][-_. ]?(?<title>.+?)(?:(?<!\b[0]\d+))(?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+|[-]))[. ]-[. ](?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+|[-]))(?:[-_. ]+(?<special>special|ova|ovd))?.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?(?:$|\.mkv)`, ignoreCase)},

	// Anime - [SubGroup] Title (Season only)
	{needsParenOrSquare, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ])?(?<title>.+?)[-_. ]+?[\[(](?:S|Season|Saison|Series|Stagione|Sezon)[-_. ]?(?<season>\d{1,2}(?![-_. ]?\d+))(?:[-_. )\]]|$)+(?<extras>EXTRAS|SUBPACK)?(?!\\)`, ignoreCase)},

	// Anime - [SubGroup] Title Absolute Episode Number
	{bracketStart, mustPattern(`^\[(?<subgroup>.+?)\][-_. ]?(?<title>.+?)[-_. ]+\(?(?:[-_. ]?#?(?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+|-[a-z]+)))+\)?(?:[-_. ]+(?<special>special|ova|ovd))?.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?(?:$|\.mkv)`, ignoreCase)},

	// Multi-episode Repeated (S01E05 - S01E06)
	{needsTwoSeasonMarkers, mustPattern(`^(?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+S(?<season>(?<!\d+)(?:\d{1,2}|\d{4})(?!\d+))(?:(?:e|[-_. ]e){1,2}(?<episode>\d{1,3}(?!\d+)))+){2,}`, ignoreCase)},

	// Multi-episode Repeated (1x05 - 1x06)
	{needsTwoCrossMarkers, mustPattern(`^(?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+(?<season>(?<!\d+)(?:\d{1,2}|\d{4})(?!\d+))(?:x{1,2}(?<episode>\d{1,3}(?!\d+)))+){2,}`, ignoreCase)},

	// Single episodes with a title (S01E05, 1x05, etc) followed by ".5 [SP]"
	{needsSpecialSuffix, mustPattern(`^(?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+S?(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))(?:[ex]|\W[ex]|_){1,2}(?<episode>\d{2,3}(?!\d+|(?:[ex]|\W[ex]|_|-){1,2}\d+))(?<special>\.5[ .]\[SP\]))`, ignoreCase)},

	// Single episodes with a title (S01E05, 1x05, etc) and trailing info in slashes
	{needsBracket, mustPattern(`^(?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+S?(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))(?:[ex]|\W[ex]|_){1,2}(?<episode>\d{2,3}(?!\d+|(?:[ex]|\W[ex]|_|-){1,2}\d+))).+?(?:\[.+?\])(?!\\)`, ignoreCase)},

	// Anime - Title Season EpisodeNumber + Absolute Episode Number [SubGroup]
	{needsBracket, mustPattern(`^(?<title>.+?)(?:[-_\W](?<![()\[!]))+(?:S?(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:(?:[ex]|\W[ex]|-){1,2}(?<episode>(?<!\d+)\d{2}(?!\d+)))+)[-_. (]+?(?:[-_. ]?(?<absoluteepisode>(?<!\d+)\d{3}(\.\d{1,2})?(?!\d+|[pi])))+.+?\[(?<subgroup>.+?)\](?:$|\.mkv)`, ignoreCase)},

	// Multi-Episode with a title (S01E05E06, S01E05-06, S01E05 E06, etc) and trailing info in slashes
	{needsBracket, mustPattern(`^(?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+S?(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))(?:[ex]|\W[ex]|_){1,2}(?<episode>\d{2,3}(?!\d+))(?:(?:\-|[ex]|\W[ex]|_){1,2}(?<episode>\d{2,3}(?!\d+)))+).+?(?:\[.+?\])(?!\\)`, ignoreCase)},

	// Anime - Title Absolute Episode Number [SubGroup] [Hash]? (Series Title Episode 99-100 [RlsGroup] [ABCD1234])
	{needsEpisodeAndSq, mustPattern(`^(?<title>.+?)[-_. ]Episode(?:[-_. ]+(?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+)))+(?:.+?)\[(?<subgroup>.+?)\].*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - Title Absolute Episode Number [SubGroup] [Hash]
	{needsBracket, mustPattern(`^(?<title>.+?)(?:(?:_|-|\s|\.)+(?<absoluteepisode>\d{3}(\.\d{1,2})(?!\d+)))+(?:.+?)\[(?<subgroup>.+?)\].*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - Title Absolute Episode Number (Year) [SubGroup]
	{needsParenAndSq, mustPattern(`^(?<title>.+?)[-_. ]+(?<absoluteepisode>(?<!\d+)\d{2}(?!\d+))[-_. ](\(\d{4}\))[-_. ]\[(?<subgroup>.+?)\]`, ignoreCase)},

	// Anime - Title with trailing number, Absolute Episode Number and hash
	{needsHashAfterDash, mustPattern(`^(?<title>[^-]+?)(?:(?<![-_. ]|\b[0]\d+) - )(?:[-_. ]?(?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+)))+(?:[-_. ]+(?<special>special|ova|ovd))?.*?(?<hash>[(\[][0-9A-F]{8}[)\]])(?:$|\.mkv)`, ignoreCase)},

	// Anime - Title Absolute Episode Number [Hash]
	{needsParenOrSquare, mustPattern(`^(?<title>.+?)(?:(?:_|-|\s|\.)+(?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+)))+(?:[-_. ]+(?<special>special|ova|ovd))?[-_. ]+.*?(?<hash>[(\[][0-9A-F]{8}[)\]])$`, ignoreCase)},

	// Episodes with airdate AND season/episode number, capture season/episode only
	{needsDate, mustPattern(`^(?<title>.+?)?\W*(?<airdate>\d{4}\W+[0-1][0-9]\W+[0-3][0-9])(?!\W+[0-3][0-9])[-_. ](?:s?(?<season>(?<!\d+)(?:\d{1,2})(?!\d+)))(?:[ex](?<episode>(?<!\d+)(?:\d{1,3})(?!\d+)))`, ignoreCase)},

	// Episodes with airdate AND season/episode number
	{needsDate, mustPattern(`^(?<title>.+?)?\W*(?<airyear>\d{4})\W+(?<airmonth>[0-1][0-9])\W+(?<airday>[0-3][0-9])(?!\W+[0-3][0-9]).+?(?:s?(?<season>(?<!\d+)(?:\d{1,2})(?!\d+)))(?:[ex](?<episode>(?<!\d+)(?:\d{1,3})(?!\d+)))`, ignoreCase)},

	// Episodes with absolute episode number AND airdate (TJET Wrestling)
	{needsDateAndBracket, mustPattern(`^(?<title>.+?)?[-_. ](?:e\d{2,3}(?!\d+))[-_. ](?<airyear>\d{4})\W+(?<airmonth>[0-1][0-9])\W+(?<airday>[0-3][0-9])(?!\W+[0-3][0-9]).+?(?:\[[a-z]+\])`, ignoreCase)},

	// Single or multi episode releases with multiple titles, then season and episode numbers after the last title. (Title1 / Title2 / ... / S1E1-2 of 6)
	{needsSlash, mustPattern(`^((?<title>.*?)[ ._]\/[ ._])+\(?S(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:\W|_)?E?[ ._]?(?<episode>(?<!\d+)\d{1,2}(?!\d+))(?:-(?<episode>(?<!\d+)\d{1,2}(?!\d+)))?(?:[ ._]of[ ._](?<episodecount>\d{1,2}))?\)?[ ._][\(\[]`, ignoreCase)},

	// Multi-episode with title (S01E99-100, S01E05-06)
	{needsSeasonEpisodeDash, mustPattern(`^(?<title>.+?)(?:[-_\W](?<![()\[!]))+S(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))E(?<episode>\d{2,3}(?!\d+))(?:-(?<episode>\d{2,3}(?!\d+)))+(?:[-_. ]|$)`, ignoreCase)},

	// Multi-episode with title (S01E05-06, S01E05-6)
	{needsSeasonEpisodeDash, mustPattern(`^(?<title>.+?)(?:[-_\W](?<![()\[!]))+S(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))E(?<episode>\d{1,2}(?!\d+))(?:-(?<episode>\d{1,2}(?!\d+)))+(?:[-_. ]|$)`, ignoreCase)},

	// Episodes with a Chinese title and English title, Single episodes (S01E05, 1x05, etc) & Multi-episode (S01E05E06, S01E05-06, S01E05 E06, etc)
	{needsNonASCII, mustPattern(`^(?<title>[\u4E00-\u9FCC]+)[_. ](?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+S?(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))(?:[ex]|\W[ex]){1,2}(?<episode>\d{2,3}(?!\d+))(?:(?:\-|[ex]|\W[ex]|_){1,2}(?<episode>\d{2,3}(?!\d+)))*)(?:[-_. ]|$)`, ignoreCase)},

	// Episodes with a title, Single episodes (S01E05, 1x05, etc) & Multi-episode (S01E05E06, S01E05-06, S01E05 E06, etc)
	{needsSeasonEpisodeMarker, mustPattern(`^(?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+S?(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))(?:[ex]|\W[ex]){1,2}(?<episode>\d{2,3}(?!\d+))(?:(?:\-|[ex]|\W[ex]|_){1,2}(?<episode>\d{2,3}(?!\d+)))*)(?:[-_. ]|$)`, ignoreCase)},

	// Episodes with a title, 4 digit season number, Single episodes (S2016E05, etc) & Multi-episode (S2016E05E06, S2016E05-06, S2016E05 E06, etc)
	{needsFourDigitSeason, mustPattern(`^(?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+S(?<season>(?<!\d+)(?:\d{4})(?!\d+))(?:e|\We|_){1,2}(?<episode>\d{2,4}(?!\d+))(?:(?:\-|e|\We|_){1,2}(?<episode>\d{2,3}(?!\d+)))*)\W?(?!\\)`, ignoreCase)},

	// Episodes with a title, 4 digit season number, Single episodes (2016x05, etc) & Multi-episode (2016x05x06, 2016x05-06, 2016x05 x06, etc)
	{needsFourDigitCross, mustPattern(`^(?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+(?<season>(?<!\d+)(?:\d{4})(?!\d+))(?:x|\Wx){1,2}(?<episode>\d{2,4}(?!\d+))(?:(?:\-|x|\Wx|_){1,2}(?<episode>\d{2,3}(?!\d+)))*)\W?(?!\\)`, ignoreCase)},

	// Episodes with a title, Single episodes (s01.05)
	{needsDottedEpisode, mustPattern(`^(?<title>.+?)(?:[-_\W](?<![()\[!]))+S(?<season>(?<!\d+)(?:\d{2})(?!\d+))(?:\.)(?<episode>\d{2,3}(?!\d+))(?:[-_. ]|$)`, ignoreCase)},

	// Multi-season pack
	{needsMultiSeason, mustPattern(`^(?<title>.+?)(Complete Series)?[-_. ]+(?:S|(?:Season|Saison|Series|Stagione)[_. ])(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))(?:(?:[-_. ]{1}|[-_. ]{3})(?:S|(?:Season|Saison|Series|Stagione)[_. ])?(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))){1,}`, ignoreCase)},

	// Partial season pack
	{needsPartialSeason, mustPattern(`^(?<title>.+?)(?:\W+S(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))\W+(?:(?:(?:Part|Vol)\W?|(?<!\d+\W+)e|p)(?<seasonpart>\d{1,2}(?!\d+)))+)`, ignoreCase)},

	// Anime - 4 digit absolute episode number in batch (1017-1088) or 1017-1088
	{bracketStart, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)(?<title>.+?)[-_. ]+?\(?(?<absoluteepisode>\d{4}(\.\d{1,2})?(?!\d+))[-](?<absoluteepisode>\d{4}(\.\d{1,2})?(?!\d+))\)?`, ignoreCase)},

	// Anime - 4 digit absolute episode number
	{bracketStart, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)(?<title>.+?)[-_. ]+?(?<absoluteepisode>\d{4}(\.\d{1,2})?(?!\d+))`, ignoreCase)},

	// Mini-Series, treated as season 1, multi episodes are labelled as E1-E2
	{needsTwoEpisodeMarkers, mustPattern(`^(?<title>.+?)(?:[-._ ][e])(?<episode>\d{2,3}(?!\d+))(?:(?:\-?[e])(?<episode>\d{2,3}(?!\d+)))+`, ignoreCase)},

	// 5 digit episode number with a title
	{needsFiveDigits, mustPattern(`^(?:(?<title>.+?)(?:_|-|\s|\.)+)(?:S?(?<season>(?<!\d+)\d{1,2}(?!\d+)))(?:(?:\-|[ex]|\W[ex]|_){1,2}(?<episode>(?<!\d+)\d{5}(?!\d+)))`, ignoreCase)},

	// 5 digit multi-episode with a title
	{needsFiveDigitsAndEp, mustPattern(`^(?:(?<title>.+?)(?:_|-|\s|\.)+)(?:S?(?<season>(?<!\d+)\d{1,2}(?!\d+)))(?:(?:[-_. ]{1,3}ep){1,2}(?<episode>(?<!\d+)\d{5}(?!\d+)))+`, ignoreCase)},

	// Episodes with airdate and part (2018.04.28.Part.2)
	{needsSeparatedDateAndPart, mustPattern(`^(?<title>.+?)?\W*(?<airyear>\d{4})[-_. ]+(?<airmonth>[0-1][0-9])[-_. ]+(?<airday>[0-3][0-9])(?![-_. ]+[0-3][0-9])[-_. ]+Part[-_. ]?(?<part>[1-9])`, ignoreCase)},

	// Episodes with airdate (2018.04.28)
	{needsSeparatedDate, mustPattern(`^(?<title>.+?)?\W*(?<airyear>\d{4})[-_. ]+(?<airmonth>[0-1][0-9])[-_. ]+(?<airday>[0-3][0-9])(?![-_. ]+[0-3][0-9])`, ignoreCase)},

	// Anime - Title 4-digit Absolute Episode Number [SubGroup]
	{needsBracket, mustPattern(`^(?<title>.+?)[-_. ]+(?<absoluteepisode>(?<!\d+)\d{4}(?!\d+))[-_. ]\[(?<subgroup>.+?)\]`, ignoreCase)},

	// Mini-Series with year in title, treated as season 1, episodes are labelled as Part01, Part 01, Part.1
	{needsFourDigits, mustPattern(`^(?<title>.+?\d{4})(?:\W+(?:(?:Part\W?|e)(?<episode>\d{1,2}(?!\d+)))+)`, ignoreCase)},

	// 3-digit season number, Title.S001E01 or Title.S001E01-E02
	{needsThreeDigitSeason, mustPattern(`^(?<title>.+?)[-_. ]+S(?<season>(?<!\d+)\d{3}(?!\d+))E(?<episode>(?<!\d+)\d{2}(?!\d+))(?:-[ex](?<episode>(?<!\d+)\d{2}(?!\d+)))?([_. ]|$)(?!\\)`, ignoreCase)},

	// Mini-Series, treated as season 1, episodes are labelled as XofY
	{needsOfCount, mustPattern(`^(?<title>.+?)(?:\W+(?:(?<episode>(?<!\d+)\d{1,2}(?!\d+))of\d+)+)`, ignoreCase)},

	// Supports Season 01 Episode 03
	{needsSeasonEpisode, mustPattern(`(?:.*(?:\"|^))(?<title>.*?)(?:[-_\W](?<![()\[]))+(?:\W?Season\W?)(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:\W|_)+(?:Episode\W)(?:[-_. ]?(?<episode>(?<!\d+)\d{1,2}(?!\d+)))+`, ignoreCase)},

	// Multi-episode with episodes in square brackets (Series Title [S01E11E12] or Series Title [S01E11-12])
	{needsSeasonBracket, mustPattern(`(?:.*(?:^))(?<title>.*?)[-._ ]+\[S(?<season>(?<!\d+)\d{2}(?!\d+))(?:[E-]{1,2}(?<episode>(?<!\d+)\d{2}(?!\d+)))+\]`, ignoreCase)},

	// Multi-episode with episodes in brackets (Series Title (S01E11E12) or Series Title (S01E11-12) or Series Title (S01E1-3 of 12))
	{needsSeasonParen, mustPattern(`(?:.*(?:^))(?<title>.*?)[-._ ]+\(S(?<season>(?<!\d+)\d{2}(?!\d+))(?:[E-]{1,2}(?<episode>(?<!\d+)\d{1,2}(?!\d+)))+([._ ]+of[._ ]+[\dx]+)?\)`, ignoreCase)},

	// Multi-episode release with no space between series title and season (S01E11E12)
	{needsCompactSeasonEpisode, mustPattern(`(?:.*(?:^))(?<title>.*?)S(?<season>(?<!\d+)\d{2}(?!\d+))(?:E(?<episode>(?<!\d+)\d{2}(?!\d+)))+`, ignoreCase)},

	// Multi-episode with single episode numbers (S6.E1-E2, S6.E1E2, S6E1E2, etc)
	{needsSeasonThenEpisode, mustPattern(`^(?<title>.+?)[-_. ]S(?<season>(?<!\d+)(?:\d{1,2}|\d{4})(?!\d+))(?:[-_. ]?[ex]?(?<episode>(?<!\d+)\d{1,2}(?!\d+)))+`, ignoreCase)},

	// Single or multi episode releases with multiple titles, each followed by season and episode numbers in brackets
	{needsSeasonParen, mustPattern(`^(?<title>.*?)[ ._]\(S(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:\W|_)?E?[ ._]?(?<episode>(?<!\d+)\d{1,2}(?!\d+))(?:-(?<episode>(?<!\d+)\d{1,2}(?!\d+)))?\)(?:[ ._]\/[ ._])(?<title>.*?)[ ._]\(`, ignoreCase)},

	// Multi-episode with episodes in brackets (Series Title (S01E11E12) or Series Title (S1E11-12) or Series Title (S1E1-3 of 12))
	{needsSeasonParen, mustPattern(`(?:.*(?:^))(?<title>.*?)[-._ ]+\(S(?<season>(?<!\d+)\d{1}(?!\d+))(?:[E-]{1,2}(?<episode>(?<!\d+)\d{1,2}(?!\d+)))+([._ ]+of[._ ]+[\dx]+)?\)`, ignoreCase)},

	// Single episode season or episode S1E1 or S1-E1 or S1.Ep1 or S01.Ep.01
	{needsSeasonThenE, mustPattern(`(?:.*(?:\"|^))(?<title>.*?)(?:\W?|_)S(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:\W|_)?Ep?[ ._]?(?<episode>(?<!\d+)\d{1,2}(?!\d+))`, ignoreCase)},

	// 3 digit season S010E05
	{needsThreeDigitSeasonThenE, mustPattern(`(?:.*(?:\"|^))(?<title>.*?)(?:\W?|_)S(?<season>(?<!\d+)\d{3}(?!\d+))(?:\W|_)?E(?<episode>(?<!\d+)\d{1,2}(?!\d+))`, ignoreCase)},

	// Separated season and episode numbers S01 - E01
	{needsSpacedDash, mustPattern(`^(?<title>.+?)(?:_|-|\s|\.)+S(?<season>\d{2}(?!\d+))(\W-\W)E(?<episode>(?<!\d+)\d{2}(?!\d+))(?!\\)`, ignoreCase)},

	// Season and episode numbers in square brackets (single and multi-episode)
	{needsBracket, mustPattern(`^(?<title>.+?)?(?:[-_\W](?<![()\[!]))+\[(?:s)?(?<season>(?<!\d+)\d{1,2})(?:(?:[ex])(?<episode>\d{2}))(?:(?:[-ex]){1,2}(?<episode>\d{2}))*\].+?(?:\.|$)`, ignoreCase)},

	// Anime - Title with season number - Absolute Episode Number (Title S01 - EP14)
	{needsSeparatorRun, mustPattern(`^(?<title>.+?S\d{1,2})[-_. ]{3,}(?:EP)?(?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+|[-]))`, ignoreCase)},

	// Anime - French titles with single episode numbers, with or without leading sub group ([RlsGroup] Title - Episode 1)
	{needsEpisodeWord, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)?(?<title>.+?)[-_. ]+?(?:Episode[-_. ]+?)(?<absoluteepisode>\d{1}(\.\d{1,2})?(?!\d+))`, ignoreCase)},

	// Anime - Absolute episode number in square brackets
	{bracketStart, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)(?<title>.+?)[-_. ]+?\[(?<absoluteepisode>\d{2,3}(\.\d{1,2})?(?!\d+))\]`, ignoreCase)},

	// Japanese variety shows with leading date
	{always, mustPattern(`^(?<airyear>\d{2})(?<airmonth>[0-1][0-9])(?<airday>[0-3][0-9])(?![-_. ]+[0-3][0-9])[-_. ](?<title>.+?)[-_. ](?:Season[-_. ]?(?<season>\d{1,2})[-_. ])?(?:ep|#)(?<episode>\d{2,3})`, ignoreCase)},

	// Season only releases followed by year
	{needsSeasonYear, mustPattern(`^(?<title>.+?)[-_. ]+?(?:S|Season|Saison|Series|Stagione)[-_. ]?(?<season>\d{1,2}(?=[-_. ]\d{4}[-_. ]+))(?<extras>EXTRAS|SUBPACK)?(?!\\)`, ignoreCase)},

	// Season only releases
	{always, mustPattern(`^(?<title>.+?)[-_. ]+?(?:S|Season|Saison|Series|Stagione)[-_. ]?(?<season>\d{1,2}(?![-_. ]?\d+))(?:[-_. ]|$)+(?<extras>EXTRAS|SUBPACK)?(?!\\)`, ignoreCase)},

	// 4 digit season only releases
	{needsFourDigits, mustPattern(`^(?<title>.+?)[-_. ]+?(?:S|Season|Saison|Series|Stagione)[-_. ]?(?<season>\d{4}(?![-_. ]?\d+))(\W+|_|$)(?<extras>EXTRAS|SUBPACK)?(?!\\)`, ignoreCase)},

	// Spanish tracker releases
	{needsSpanishCap, mustPattern(`^(?<title>.+?)(?:(?:[-_. ]+?Temporada.+?|\[.+?\])\[Cap)(?:[-_. ]+(?<season>(?<!\d+)\d{1,2})(?<episode>(?<!e|x)(?:[1-9][0-9]|[0][1-9])))+(?:\])`, ignoreCase)},

	// Supports 103/113 naming
	{needsThreeDigitCode, mustPattern(`^(?<title>.+?)?(?:(?:[_.-](?<![()\[!]))+(?<season>(?<!\d+)[1-9])(?<episode>[1-9][0-9]|[0][1-9])(?![a-z]|\d+))+(?:[_.]|$)`, ignoreCase)},

	// 4 digit episode number, without a title, single (S01E05, 1x05) and multi (S01E04E05, 1x04x05, etc)
	{always, mustPattern(`^(?:S?(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:(?:\-|[ex]|\W[ex]|_){1,2}(?<episode>\d{4}(?!\d+|i|p)))+)(\W+|_|$)(?!\\)`, ignoreCase)},

	// 4 digit episode number, with a title, single (S01E05, 1x05, etc) and multi (S01E05E06, S01E05-06, S01E05 E06, etc)
	{needsFourDigitEpisode, mustPattern(`^(?<title>.+?)(?:(?:[-_\W](?<![()\[!]|\d{1,2}-))+S?(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:(?:\-|[ex]|\W[ex]|_){1,2}(?<episode>\d{4}(?!\d+|i|p)))+)\W?(?!\\)`, ignoreCase)},

	// Turkish tracker releases (01 BLM, 3. Blm, 04.Bolum, etc)
	{needsTurkishWord, mustPattern(`^(?<title>.+?)[_. ](?<absoluteepisode>\d{1,4})(?:[_. ]+)(?:BLM|B[oö]l[uü]m)`, ignoreCase)},

	// Episodes with airdate (04.28.2018)
	{needsAmbiguousDate, mustPattern(`^(?<title>.+?)?\W*(?<ambiguousairmonth>[0-1][0-9])[-_. ]+(?<ambiguousairday>[0-3][0-9])[-_. ]+(?<airyear>\d{4})(?!\d+)`, ignoreCase)},

	// Episodes with airdate (28.04.2018)
	{needsAmbiguousDate, mustPattern(`^(?<title>.+?)?\W*(?<ambiguousairday>[0-3][0-9])[-_. ]+(?<ambiguousairmonth>[0-1][0-9])[-_. ]+(?<airyear>\d{4})(?!\d+)`, ignoreCase)},

	// Episodes with airdate (20180428)
	{needsEightDigits, mustPattern(`^(?<title>.+?)?\W*(?<!\d+)(?<airyear>\d{4})(?<airmonth>[0-1][0-9])(?<airday>[0-3][0-9])(?!\d+)`, ignoreCase)},

	// Anime - Title Absolute Episode Number (E195 or E1206)
	{needsLongEpisodeMarker, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)?(?<title>.+?)(?:(?:_|-|\s|\.)+(?:e|ep)(?<absoluteepisode>(\d{3}|\d{4})(\.\d{1,2})?))+[-_. ].*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Daily episodes that use short month format instead of number
	{needsShortMonth, mustPattern(`^(?<title>.+?)[-_. ]+(?<airday>[1-2]\d|3[01]|[1-9])(?:th|st|rd)[-_. ](?<shortairmonth>jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[-_. ](?<airyear>19[4-9]\d|20\d\d)`, ignoreCase)},

	// Supports 1103/1113 naming
	{needsFourDigitCode, mustPattern(`^(?<title>.+?)?(?:(?:[-_. ](?<![()\[!]))*(?<!\d{1,2}-)(?<season>(?<!\d+|\(|\[|e|x)\d{2})(?<episode>(?<!e|x)(?:[1-9][0-9]|[0][1-9])(?!p|i|\d+|\)|\]|\W\d+|\W(?:e|ep|x)\d+)))+([-_. ]+|$)(?!\\)`, ignoreCase)},

	// Dutch/Flemish release titles
	{needsDutchWord, mustPattern(`^(?<title>.+?)[-_. ](?:Se\.(?<season>(?<!\d+)(?:\d{1,2}|\d{4})(?!\d+))(?:(?:[-_ ]?afl\.)(?<episode>\d{1,3}(?!\d+))(?:(?:[-]|[-_ ]en[-_ ])(?<episode>\d{1,3}(?!\d+)))*))`, ignoreCase)},

	// Episodes with single digit episode number (S01E1, S01E5E6, etc)
	{needsShortEpisodeMarker, mustPattern(`^(?<title>.*?)(?:(?:[-_\W](?<![()\[!]))+S?(?<season>(?<!\d+)\d{1,2}(?!\d+))(?:(?:\-|[ex]){1,2}(?<episode>\d{1}))+)+(\W+|_|$)(?!\\)`, ignoreCase)},

	// iTunes Season 1\05 Title (Quality).ext
	{always, mustPattern(`^(?:Season(?:_|-|\s|\.)(?<season>(?<!\d+)\d{1,2}(?!\d+)))(?:_|-|\s|\.)(?<episode>(?<!\d+)\d{1,2}(?!\d+))`, ignoreCase)},

	// iTunes 1-05 Title (Quality).ext
	{always, mustPattern(`^(?:(?<season>(?<!\d+)(?:\d{1,2})(?!\d+))(?:-(?<episode>\d{2,3}(?!\d+))))`, ignoreCase)},

	// Mini-Series, treated as season 1, episodes are labelled as Part01, Part 01, Part.1
	{needsPartOrEpisodeMarker, mustPattern(`^(?<title>.+?)(?:\W+(?:(?:(?<!\()Part\W?|(?<!\d+\W+)e)(?<episode>\d{1,2}(?!\d+|\))))+)`, ignoreCase)},

	// Mini-Series, treated as season 1, episodes are labelled as Part One/Two/Three/...Nine, Part.One, Part_One
	{needsPartWord, mustPattern(`^(?<title>.+?)(?:\W+(?:Part[-._ ](?<episode>One|Two|Three|Four|Five|Six|Seven|Eight|Nine)(?>[-._ ])))`, ignoreCase)},

	// Anime Range - Title Absolute Episode Number (ep01-12)
	{needsEpisodeRunMarker, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)?(?<title>.+?)(?:_|\s|\.)+(?:e|ep)(?<absoluteepisode>\d{2,3}(\.\d{1,2})?)-(?<absoluteepisode>(?<!\d+)\d{1,2}(\.\d{1,2})?(?!\d+|-)).*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - Title Absolute Episode Number (e66)
	{needsEpisodeRunMarker, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)?(?<title>.+?)(?:(?:_|-|\s|\.)+(?:e|ep)(?<absoluteepisode>\d{2,4}(\.\d{1,2})?))+[-_. ].*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - Title Episode Absolute Episode Number (Series Title Episode 01)
	{needsEpisodeWord, mustPattern(`^(?<title>.+?)[-_. ](?:Episode)(?:[-_. ]+(?<absoluteepisode>(?<!\d+)\d{2,3}(\.\d{1,2})?(?!\d+)))+.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime Range - Title Absolute Episode Number (1 or 2 digit absolute episode numbers in a range, 1-10)
	{needsDigitRange, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)?(?<title>.+?)[_. ]+(?<absoluteepisode>(?<!\d+)\d{1,2}(\.\d{1,2})?(?!\d+))-(?<absoluteepisode>(?<!\d+)\d{1,2}(\.\d{1,2})?(?!\d+|-)).*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - Title Episode/Episodio Absolute Episode Number
	{needsEpisodioWord, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)?(?<title>.+?)[-_. ]+(?:Episode|Episodio)(?:[-_. ]+(?<absoluteepisode>(?<!\d+)\d{2,4}(\.\d{1,2})?(?!\d+|[ip])))+.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - Title [Absolute Episode Number] from AniLibriaTV
	{needsBracket, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)?(?<title>.+?)(?:[-_. ]\[)(?:(?:-?)(?<absoluteepisode>(?<!\d+)\d{2,3}(\.\d{1,2})?(?!\d+|[ip])))+(?:\][-_. ]).*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Simple season releases in the format 'Series (year) - Temporada season'
	{needsTemporadaWord, mustPattern(`^(?<title>.+?)[_. ]-[_. ](?:Temporada)[-_. ]?(?<season>\d{1,2}(?![_. ]?\d+))(?:[_. ]|$)+(?<extras>EXTRAS|SUBPACK)?(?!\\)`, ignoreCase)},

	// Anime - Title Absolute Episode Number
	{needsSeparatedAbsolute, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)?(?<title>.+?)(?:[-_. ]+(?<absoluteepisode>(?<!\d+)\d{2,4}(\.\d{1,2})?(?!\d+|[ip])))+.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Anime - Title {Absolute Episode Number}
	{needsBoundedAbsolute, mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)?(?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+(?<absoluteepisode>(?<!\d+)\d{2,3}(\.\d{1,2})?(?!\d+|[ip])))+.*?(?<hash>[(\[][0-9A-F]{8}[)\]])?$`, ignoreCase)},

	// Extant, terrible multi-episode naming (extant.10708.hdtv-lol.mp4)
	{needsExtantNumber, mustPattern(`^(?<title>.+?)[-_. ](?<season>[0]?\d?)(?:(?<episode>\d{2}){2}(?!\d+))[-_. ]`, ignoreCase)},

	// Episodes without a title, Single episode numbers (S1E1, 1x1)
	{always, mustPattern(`^(?:S?(?<season>(?<!\d+)(?:\d{1,2}|\d{4})(?!\d+))(?:(?:[-_ ]?[ex])(?<episode>\d{1}(?!\d+))))`, ignoreCase)},

	// Anime OVA special
	{bracketStart, mustPattern(`^\[(?<subgroup>.+?)\][-_. ]?(?<title>.+?)(?:[-_. ]+(?<special>special|ova|ovd|ncop|nced)).*?(?<hash>[(\[][0-9A-F]{8}[)\]])?(?:$|\.mkv)`, ignoreCase)},

	// Episodes with a title, 3 digit season number, Single episodes (S001E05, 1x05, etc) & Multi-episode (S001E05E06, S001E05-06, S001E05 E06, etc)
	{needsThreeDigitSeasonMarker, mustPattern(`^(?<title>.+?)(?:(?:[-_\W](?<![()\[!]))+S?(?<season>(?<!\d+)(?:\d{3})(?!\d+))(?:[ex]|\W[ex]){1,2}(?<episode>\d{2,3}(?!\d+))(?:(?:\-|[ex]|\W[ex]|_){1,2}(?<episode>\d{2,3}(?!\d+)))*)(?:[-_. ]|$)`, ignoreCase)},
}
