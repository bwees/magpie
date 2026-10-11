// Code generated from Radarr's parser test fixtures. DO NOT EDIT.
//
// Source: https://github.com/Radarr/Radarr/blob/f5ccd1ca7888b70d43fc22d0108154aa753deba8/src/NzbDrone.Core.Test/ParserTests/ExtendedQualityParserRegex.cs
// Copyright (C) Radarr contributors. Licensed under GPL-3.0-or-later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fixtures

var RadarrExtendedQualityParserRegex = File{
	Source: "Radarr/src/NzbDrone.Core.Test/ParserTests/ExtendedQualityParserRegex.cs",
	Cases: []Case{
		{Line: 16, Method: "should_parse_reality_from_title", Args: Args{"Movie.Title.HDTV.XviD-LOL", 0}},
		{Line: 17, Method: "should_parse_reality_from_title", Args: Args{"Movie.Title.Garnets.or.Gold.REAL.REAL.PROPER.HDTV.x264-W4F", 2}},
		{Line: 18, Method: "should_parse_reality_from_title", Args: Args{"Movie.Title.REAL.PROPER.720p.HDTV.x264-ORENJI-RP", 1}},
		{Line: 19, Method: "should_parse_reality_from_title", Args: Args{"Movie.Title.REAL.PROPER.HDTV.x264-KILLERS", 1}},
		{Line: 20, Method: "should_parse_reality_from_title", Args: Args{"Movie.Title.REAL.PROPER.720p.HDTV.x264-KILLERS", 1}},
		{Line: 21, Method: "should_parse_reality_from_title", Args: Args{"Movie.Title.New.Black.real.proper.720p.webrip.x264-2hd", 0}},
		{Line: 22, Method: "should_parse_reality_from_title", Args: Args{"Movie.Title.Super.Duper.Real.Proper.HDTV.x264-FTP", 0}},
		{Line: 23, Method: "should_parse_reality_from_title", Args: Args{"Movie.Title.PROPER.HDTV.x264-RiVER-RP", 0}},
		{Line: 24, Method: "should_parse_reality_from_title", Args: Args{"Movie.Title.PROPER.REAL.RERIP.1080p.BluRay.x264-TENEIGHTY", 1}},
		{Line: 25, Method: "should_parse_reality_from_title", Args: Args{"[MGS] - Movie.Title - Episode 02v2 - [D8B6C90D]", 0}},
		{Line: 26, Method: "should_parse_reality_from_title", Args: Args{"[Hatsuyuki] Movie Title - 07 [v2][848x480][23D8F455].avi", 0}},
		{Line: 27, Method: "should_parse_reality_from_title", Args: Args{"[DeadFish] Movie Title - 01v3 [720p][AAC]", 0}},
		{Line: 28, Method: "should_parse_reality_from_title", Args: Args{"[DeadFish] Movie Title Sword - 01v4 [720p][AAC]", 0}},
		{Line: 29, Method: "should_parse_reality_from_title", Args: Args{"The Real Movie.Titlewives of Some Place - S01E01 - Why are we doing this?", 0}},
		{Line: 35, Method: "should_parse_version_from_title", Args: Args{"Movie.Title.HDTV.XviD-LOL", 1}},
		{Line: 36, Method: "should_parse_version_from_title", Args: Args{"Movie.Title.Garnets.or.Gold.REAL.REAL.PROPER.HDTV.x264-W4F", 2}},
		{Line: 37, Method: "should_parse_version_from_title", Args: Args{"Movie.Title.REAL.PROPER.720p.HDTV.x264-ORENJI-RP", 2}},
		{Line: 38, Method: "should_parse_version_from_title", Args: Args{"Movie.Title.REAL.PROPER.HDTV.x264-KILLERS", 2}},
		{Line: 39, Method: "should_parse_version_from_title", Args: Args{"Movie.Title.REAL.PROPER.720p.HDTV.x264-KILLERS", 2}},
		{Line: 40, Method: "should_parse_version_from_title", Args: Args{"Movie.Title.New.Black.real.proper.720p.webrip.x264-2hd", 2}},
		{Line: 41, Method: "should_parse_version_from_title", Args: Args{"Movie.Title.Super.Duper.Real.Proper.HDTV.x264-FTP", 2}},
		{Line: 42, Method: "should_parse_version_from_title", Args: Args{"Movie.Title.PROPER.HDTV.x264-RiVER-RP", 2}},
		{Line: 43, Method: "should_parse_version_from_title", Args: Args{"Movie.Title.PROPER.REAL.RERIP.1080p.BluRay.x264-TENEIGHTY", 2}},
		{Line: 49, Method: "should_parse_ultrahd_from_title", Args: Args{"Movie 2016 2160p 4K UltraHD BluRay DTS-HD MA 7 1 x264-Whatevs", 19}},
		{Line: 50, Method: "should_parse_ultrahd_from_title", Args: Args{"Movie 2016 2160p 4K UltraHD DTS-HD MA 7 1 x264-Whatevs", 16}},
		{Line: 51, Method: "should_parse_ultrahd_from_title", Args: Args{"Movie 2016 4K 2160p UltraHD BluRay AAC2 0 HEVC x265", 19}},
		{Line: 52, Method: "should_parse_ultrahd_from_title", Args: Args{"The Movie 2015 2160p UHD BluRay DTS x264-Whatevs", 19}},
		{Line: 53, Method: "should_parse_ultrahd_from_title", Args: Args{"The Movie 2015 2160p UHD BluRay FLAC 7 1 x264-Whatevs", 19}},
		{Line: 54, Method: "should_parse_ultrahd_from_title", Args: Args{"The Movie 2015 2160p Ultra HD BluRay DTS-HD MA 7 1 x264-Whatevs", 19}},
		{Line: 55, Method: "should_parse_ultrahd_from_title", Args: Args{"Into the Movie 2016 2160p Netflix WEBRip DD5 1 x264-Whatevs", 18}},
	},
}
