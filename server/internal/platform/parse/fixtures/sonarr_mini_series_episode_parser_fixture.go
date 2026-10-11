// Code generated from Sonarr's parser test fixtures. DO NOT EDIT.
//
// Source: https://github.com/Sonarr/Sonarr/blob/9d0920b470b033106ca2913572970324244e8fb0/src/NzbDrone.Core.Test/ParserTests/MiniSeriesEpisodeParserFixture.cs
// Copyright (C) Sonarr contributors. Licensed under GPL-3.0-or-later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fixtures

var SonarrMiniSeriesEpisodeParserFixture = File{
	Source: "Sonarr/src/NzbDrone.Core.Test/ParserTests/MiniSeriesEpisodeParserFixture.cs",
	Cases: []Case{
		{Line: 11, Method: "should_parse_mini_series_episode", Args: Args{"The.Big.Series.Leader.Part.2.DSR.XviD-SYS", "The Big Series Leader", 2}},
		{Line: 12, Method: "should_parse_mini_series_episode", Args: Args{"kill-roy-was-here-e07-720p", "kill-roy-was-here", 7}},
		{Line: 13, Method: "should_parse_mini_series_episode", Args: Args{"Series and Show 2012 Part 1 REPACK 720p HDTV x264 2HD", "Series and Show 2012", 1}},
		{Line: 14, Method: "should_parse_mini_series_episode", Args: Args{"Series Show.2016.E04.Power.720p.WEB-DL.DD5.1.H.264-MARS", "Series Show 2016", 4}},
		{Line: 30, Method: "should_parse_korean_series_episode", Args: Args{"It's a Series Title.E56.190121.720p-NEXT.mp4", "It's a Series Title", 56, "2019-01-21"}},
		{Line: 31, Method: "should_parse_korean_series_episode", Args: Args{"My Only Series Title.E37.190120.1080p-NEXT.mp4", "My Only Series Title", 37, "2019-01-20"}},
		{Line: 32, Method: "should_parse_korean_series_episode", Args: Args{"Series.E191.190121.720p-NEXT.mp4", "Series", 191, "2019-01-21"}},
		{Line: 33, Method: "should_parse_korean_series_episode", Args: Args{"The Series Title Challenge.E932.190120.720p-NEXT.mp4", "The Series Title Challenge", 932, "2019-01-20"}},
		{Line: 34, Method: "should_parse_korean_series_episode", Args: Args{"백번의 추억.E12.251019.1080p-F1RST", "백번의 추억", 12, "2025-01-19"}},
		{Line: 35, Method: "should_parse_korean_series_episode", Args: Args{"나는 SOLO <나는 솔로>.E223.251015.1080p-F1RST", "나는 SOLO 나는 솔로", 223, "2025-10-15"}},
		{Line: 36, Method: "should_parse_korean_series_episode", Args: Args{"나는 SOLO 그 후, 사랑은 계속된다(나솔사계).E135.251016.1080p-F1RST", "나는 SOLO 그 후, 사랑은 계속된다(나솔사계)", 135, "2025-10-16"}},
	},
}
