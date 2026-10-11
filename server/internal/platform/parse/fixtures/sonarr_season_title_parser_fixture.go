// Code generated from Sonarr's parser test fixtures. DO NOT EDIT.
//
// Source: https://github.com/Sonarr/Sonarr/blob/9d0920b470b033106ca2913572970324244e8fb0/src/NzbDrone.Core.Test/ParserTests/SeasonTitleParserFixture.cs
// Copyright (C) Sonarr contributors. Licensed under GPL-3.0-or-later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fixtures

var SonarrSeasonTitleParserFixture = File{
	Source: "Sonarr/src/NzbDrone.Core.Test/ParserTests/SeasonTitleParserFixture.cs",
	Cases: []Case{
		{Line: 10, Method: "should_return_the_title_when_there_is_no_season_or_episode", Args: Args{"[Group] Series Title Curse (BS11 1280x720 x264 AAC)", "Series Title Curse"}},
		{Line: 11, Method: "should_return_the_title_when_there_is_no_season_or_episode", Args: Args{"[Group] Series Title Curse (BDRip 1920x1080 x264 FLAC) [ABCD1234]", "Series Title Curse"}},
		{Line: 12, Method: "should_return_the_title_when_there_is_no_season_or_episode", Args: Args{"[Group] Series.Title - Curse (BD, 720p)", "Series.Title - Curse"}},
		{Line: 13, Method: "should_return_the_title_when_there_is_no_season_or_episode", Args: Args{"Series Title Curse [Bluray.1080p.HEVC.AAC.ITA.FLAC.JAP.Sub.Ita]", "Series Title Curse"}},
		{Line: 14, Method: "should_return_the_title_when_there_is_no_season_or_episode", Args: Args{"[Group] Series Title: Curse [Dual Audio 10bit 1080p][HEVC-x265]", "Series Title: Curse"}},
		{Line: 28, Method: "should_populate_the_fields_the_decision_engine_reads", Args: Args{"[Group] Series Title Curse (BD 1280x720 x264 AAC)"}},
		{Line: 29, Method: "should_populate_the_fields_the_decision_engine_reads", Args: Args{"[Group] Series Title: Curse [Dual Audio 10bit 1080p][HEVC-x265]"}},
		{Line: 39, Method: "should_not_return_a_title_when_stripping_leaves_metadata", Args: Args{"[Group][Series Title: Curse][BDRip][1080P_x265(10bit)-FLAC][ALL]"}},
		{Line: 40, Method: "should_not_return_a_title_when_stripping_leaves_metadata", Args: Args{"[Group][Series Title Curse][1080p][x264]"}},
		{Line: 41, Method: "should_not_return_a_title_when_stripping_leaves_metadata", Args: Args{"[Group] Series Title Curse () (1080p)"}},
		{Line: 47, Method: "should_not_return_a_title_for_an_extra", Args: Args{"[Group] Series Title Curse (OVA)"}},
		{Line: 48, Method: "should_not_return_a_title_for_an_extra", Args: Args{"[Group] Series Title Curse [OVA]"}},
		{Line: 49, Method: "should_not_return_a_title_for_an_extra", Args: Args{"[Group] Series Title Curse (Special)"}},
		{Line: 50, Method: "should_not_return_a_title_for_an_extra", Args: Args{"[Group] Series Title Curse (NCOP)"}},
		{Line: 51, Method: "should_not_return_a_title_for_an_extra", Args: Args{"[Group] Series Title Curse (NCED)"}},
		{Line: 57, Method: "should_not_flag_a_release_the_regexes_can_parse", Args: Args{"Series Title S02E05 1080p WEB-DL"}},
		{Line: 58, Method: "should_not_flag_a_release_the_regexes_can_parse", Args: Args{"[Group] Series Title - 05 (1080p)"}},
		{Line: 59, Method: "should_not_flag_a_release_the_regexes_can_parse", Args: Args{"Series Title S02 1080p WEB-DL"}},
		{Line: 65, Method: "should_leave_parse_title_rejecting_what_it_rejects_today", Args: Args{"THIS SHOULD NEVER PARSE"}},
		{Line: 66, Method: "should_leave_parse_title_rejecting_what_it_rejects_today", Args: Args{"Vrq6e1Aba3U amCjuEgV5R2QvdsLEGYF3YQAQkw8"}},
		{Line: 67, Method: "should_leave_parse_title_rejecting_what_it_rejects_today", Args: Args{"[Group] Series Title Curse (BS11 1280x720 x264 AAC)"}},
	},
}
