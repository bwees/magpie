// Code generated from Sonarr's parser test fixtures. DO NOT EDIT.
//
// Source: https://github.com/Sonarr/Sonarr/blob/9d0920b470b033106ca2913572970324244e8fb0/src/NzbDrone.Core.Test/ParserTests/ExtendedQualityParserRegex.cs
// Copyright (C) Sonarr contributors. Licensed under GPL-3.0-or-later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fixtures

var SonarrExtendedQualityParserRegex = File{
	Source: "Sonarr/src/NzbDrone.Core.Test/ParserTests/ExtendedQualityParserRegex.cs",
	Cases: []Case{
		{Line: 11, Method: "should_parse_reality_from_title", Args: Args{"Series.Title.S04E05.HDTV.XviD-LOL", 0}},
		{Line: 12, Method: "should_parse_reality_from_title", Args: Args{"Series.Title.S04E05.Garnets.or.Gold.REAL.REAL.PROPER.HDTV.x264-W4F", 2}},
		{Line: 13, Method: "should_parse_reality_from_title", Args: Args{"Series.Title.S03E17.REAL.PROPER.720p.HDTV.x264-ORENJI-RP", 1}},
		{Line: 14, Method: "should_parse_reality_from_title", Args: Args{"Series.Title.S05E09.REAL.PROPER.HDTV.x264-KILLERS", 1}},
		{Line: 15, Method: "should_parse_reality_from_title", Args: Args{"Series.Title.S14E01.REAL.PROPER.720p.HDTV.x264-KILLERS", 1}},
		{Line: 16, Method: "should_parse_reality_from_title", Args: Args{"Series.Title.s02e06.real.proper.720p.webrip.x264-2hd", 0}},
		{Line: 17, Method: "should_parse_reality_from_title", Args: Args{"Series.Title.S21E07.Super.Duper.Real.Proper.HDTV.x264-FTP", 0}},
		{Line: 18, Method: "should_parse_reality_from_title", Args: Args{"Series.Title.S21E07.PROPER.HDTV.x264-RiVER-RP", 0}},
		{Line: 19, Method: "should_parse_reality_from_title", Args: Args{"Series.Title.S07E11.PROPER.REAL.RERIP.1080p.BluRay.x264-TENEIGHTY", 1}},
		{Line: 20, Method: "should_parse_reality_from_title", Args: Args{"[MGS] - Series Title - Episode 02v2 - [D8B6C90D]", 0}},
		{Line: 21, Method: "should_parse_reality_from_title", Args: Args{"[Hatsuyuki] Series Title - 07 [v2][848x480][23D8F455].avi", 0}},
		{Line: 22, Method: "should_parse_reality_from_title", Args: Args{"[DeadFish] Series Title - 01v3 [720p][AAC]", 0}},
		{Line: 23, Method: "should_parse_reality_from_title", Args: Args{"[DeadFish] Series Title - 01v4 [720p][AAC]", 0}},
		{Line: 24, Method: "should_parse_reality_from_title", Args: Args{"The Real Housewives of Some Place - S01E01 - Why are we doing this?", 0}},
		{Line: 30, Method: "should_parse_version_from_title", Args: Args{"Series.Title.S04E05.HDTV.XviD-LOL", 1}},
		{Line: 31, Method: "should_parse_version_from_title", Args: Args{"Series.Title.S04E05.Garnets.or.Gold.REAL.REAL.PROPER.HDTV.x264-W4F", 2}},
		{Line: 32, Method: "should_parse_version_from_title", Args: Args{"Series.Title.S03E17.REAL.PROPER.720p.HDTV.x264-ORENJI-RP", 2}},
		{Line: 33, Method: "should_parse_version_from_title", Args: Args{"Series.Title.S05E09.REAL.PROPER.HDTV.x264-KILLERS", 2}},
		{Line: 34, Method: "should_parse_version_from_title", Args: Args{"Series.Title.S14E01.REAL.PROPER.720p.HDTV.x264-KILLERS", 2}},
		{Line: 35, Method: "should_parse_version_from_title", Args: Args{"Series.Title.Is.the.New.Black.s02e06.real.proper.720p.webrip.x264-2hd", 2}},
		{Line: 36, Method: "should_parse_version_from_title", Args: Args{"Series.Title.S21E07.Super.Duper.Real.Proper.HDTV.x264-FTP", 2}},
		{Line: 37, Method: "should_parse_version_from_title", Args: Args{"Series.Title.S21E07.PROPER.HDTV.x264-RiVER-RP", 2}},
		{Line: 38, Method: "should_parse_version_from_title", Args: Args{"Series.Title.S07E11.PROPER.REAL.RERIP.1080p.BluRay.x264-TENEIGHTY", 2}},
		{Line: 39, Method: "should_parse_version_from_title", Args: Args{"[MGS] - Series Title - Episode 02v2 - [D8B6C90D]", 2}},
		{Line: 40, Method: "should_parse_version_from_title", Args: Args{"[Hatsuyuki] Series Title - 07 [v2][848x480][23D8F455].avi", 2}},
		{Line: 41, Method: "should_parse_version_from_title", Args: Args{"[DeadFish] Series Title - 01v3 [720p][AAC]", 3}},
		{Line: 42, Method: "should_parse_version_from_title", Args: Args{"[DeadFish]Series Title - 01v4 [720p][AAC]", 4}},
		{Line: 43, Method: "should_parse_version_from_title", Args: Args{"[Vivid-Asenshi] Series Title - 04v2 [266EE983]", 2}},
		{Line: 44, Method: "should_parse_version_from_title", Args: Args{"[Vivid-Asenshi] Series Title - 03v2 [66A05817]", 2}},
		{Line: 45, Method: "should_parse_version_from_title", Args: Args{"[Vivid-Asenshi] Series Title - 02v2 [1F67AB55]", 2}},
	},
}
