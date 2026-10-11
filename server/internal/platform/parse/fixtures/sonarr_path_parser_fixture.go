// Code generated from Sonarr's parser test fixtures. DO NOT EDIT.
//
// Source: https://github.com/Sonarr/Sonarr/blob/9d0920b470b033106ca2913572970324244e8fb0/src/NzbDrone.Core.Test/ParserTests/PathParserFixture.cs
// Copyright (C) Sonarr contributors. Licensed under GPL-3.0-or-later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fixtures

var SonarrPathParserFixture = File{
	Source: "Sonarr/src/NzbDrone.Core.Test/ParserTests/PathParserFixture.cs",
	Cases: []Case{
		{Line: 11, Method: "should_parse_from_path", Args: Args{"z:\\tv shows\\series title (2003)\\Season 3\\S03E05 - Title.mkv", 3, 5}},
		{Line: 12, Method: "should_parse_from_path", Args: Args{"z:\\tv showsseries title\\Season 16\\S16E03 - The Title.mkv", 16, 3}},
		{Line: 13, Method: "should_parse_from_path", Args: Args{"z:\\tv shows\\series title\\Specials\\S00E16 - Dear Title - SD TV.avi", 0, 16}},
		{Line: 14, Method: "should_parse_from_path", Args: Args{"D:\\shares\\TV Shows\\series title\\Season 2\\S02E21 - 94 Title - 720p TV.mkv", 2, 21}},
		{Line: 15, Method: "should_parse_from_path", Args: Args{"D:\\shares\\TV Shows\\Series (2003)\\Season 2\\S02E21.avi", 2, 21}},
		{Line: 16, Method: "should_parse_from_path", Args: Args{"C:/Test/TV/Series.4x05.HDTV.XviD-LOL", 4, 5}},
		{Line: 17, Method: "should_parse_from_path", Args: Args{"P:\\TV Shows\\Series\\Season 6\\S06E13 - 5 to 9 - 720p BluRay.mkv", 6, 13}},
		{Line: 18, Method: "should_parse_from_path", Args: Args{"S:\\TV Drop\\Series - 10x11 - Title [SDTV]\\1011 - Title.avi", 10, 11}},
		{Line: 19, Method: "should_parse_from_path", Args: Args{"/TV Drop/Series - 10x11 - Title [SDTV]/1011 - Title.avi", 10, 11}},
		{Line: 20, Method: "should_parse_from_path", Args: Args{"S:\\TV Drop\\Series Title - 10x12 - 24 Hours of Development [SDTV]\\1012 - Hours of Development.avi", 10, 12}},
		{Line: 21, Method: "should_parse_from_path", Args: Args{"/TV Drop/Series Title - 10x12 - 24 Hours of Development [SDTV]/1012 - Hours of Development.avi", 10, 12}},
		{Line: 22, Method: "should_parse_from_path", Args: Args{"S:\\TV Drop\\Series Title - 10x12 - 24 Hours of Development [SDTV]\\Hours of Development.avi", 10, 12}},
		{Line: 23, Method: "should_parse_from_path", Args: Args{"/TV Drop/Series Title - 10x12 - 24 Hours of Development [SDTV]/Hours of Development.avi", 10, 12}},
		{Line: 24, Method: "should_parse_from_path", Args: Args{"E:\\Downloads\\tv\\Series.Title.S01E01.720p.HDTV\\ajifajjjeaeaeqwer_eppj.avi", 1, 1}},
		{Line: 25, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Unsorted\\Series.Title.S01E01.720p.HDTV\\tbbt101.avi", 1, 1}},
		{Line: 26, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Unsorted\\Series.Title.S02E19.720p.BluRay.x264-SiNNERS-RP\\ba27283b17c00d01193eacc02a8ba98eeb523a76.mkv", 2, 19}},
		{Line: 27, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Unsorted\\Series.Title.S02E18.720p.BluRay.x264-SiNNERS-RP\\45a55debe3856da318cc35882ad07e43cd32fd15.mkv", 2, 18}},
		{Line: 28, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series\\Season 01\\01 Pilot (1080p HD).mkv", 1, 1}},
		{Line: 29, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series\\Season 01\\1 Pilot (1080p HD).mkv", 1, 1}},
		{Line: 30, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series\\Season 1\\02 Honor Thy Father (1080p HD).m4v", 1, 2}},
		{Line: 31, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series\\Season 1\\2 Honor Thy Developer (1080p HD).m4v", 1, 2}},
		{Line: 32, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series\\Season 2 - Total Series Action\\01. Total Series Action - Episode 1 - Monster Cash.mkv", 2, 1}},
		{Line: 33, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series\\Season 2\\01. Total Series Action - Episode 1 - Monster Cash.mkv", 2, 1}},
		{Line: 34, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series\\Season 1\\02.04.24 - S01E01 - The Rabbit Hole", 1, 1}},
		{Line: 35, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series\\Season 1\\8 Series Rules - S01E01 - Pilot", 1, 1}},
		{Line: 36, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series Title\\Series Title S02 01.mkv", 2, 1}},
		{Line: 37, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series Title\\Series Title S2 1.mkv", 2, 1}},
		{Line: 38, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series Title\\Series Title S2 10.mkv", 2, 10}},
		{Line: 39, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series Title\\S02 01.mkv", 2, 1}},
		{Line: 40, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series Title\\Series Title S02 10.mkv", 2, 10}},
		{Line: 41, Method: "should_parse_from_path", Args: Args{"C:\\Test\\Series Title S01 02\\Series Title S02 01 1080p WEB-DL.mkv", 2, 1}},
		{Line: 57, Method: "should_parse_multi_episode_from_path", Args: Args{"01-03\\The Series Title (2010) - 1x01-02-03 - Episode Title HDTV-720p Proper", "The Series Title (2010)", 1, []int{1, 2, 3}}},
		{Line: 58, Method: "should_parse_multi_episode_from_path", Args: Args{"Season 2\\E05-06 - Episode Title HDTV-720p Proper", "", 2, []int{5, 6}}},
	},
}
