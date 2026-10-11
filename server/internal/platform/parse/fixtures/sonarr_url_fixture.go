// Code generated from Sonarr's parser test fixtures. DO NOT EDIT.
//
// Source: https://github.com/Sonarr/Sonarr/blob/9d0920b470b033106ca2913572970324244e8fb0/src/NzbDrone.Core.Test/ParserTests/UrlFixture.cs
// Copyright (C) Sonarr contributors. Licensed under GPL-3.0-or-later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fixtures

var SonarrUrlFixture = File{
	Source: "Sonarr/src/NzbDrone.Core.Test/ParserTests/UrlFixture.cs",
	Cases: []Case{
		{Line: 11, Method: "should_not_parse_url_in_name", Args: Args{"[www.test.com] - Series.S03E14.720p.HDTV.X264-DIMENSION", "Series"}},
		{Line: 12, Method: "should_not_parse_url_in_name", Args: Args{"test.net - Series.S03E14.720p.HDTV.X264-DIMENSION", "Series"}},
		{Line: 13, Method: "should_not_parse_url_in_name", Args: Args{"[www.test-hyphen.com] - Series.S03E14.720p.HDTV.X264-DIMENSION", "Series"}},
		{Line: 14, Method: "should_not_parse_url_in_name", Args: Args{"www.test123.org - Series.S03E14.720p.HDTV.X264-DIMENSION", "Series"}},
		{Line: 15, Method: "should_not_parse_url_in_name", Args: Args{"[test.co.uk] - Series.S03E14.720p.HDTV.X264-DIMENSION", "Series"}},
		{Line: 16, Method: "should_not_parse_url_in_name", Args: Args{"www.test-hyphen.net.au - Series.S03E14.720p.HDTV.X264-DIMENSION", "Series"}},
		{Line: 17, Method: "should_not_parse_url_in_name", Args: Args{"[www.test123.co.nz] - Series.S03E14.720p.HDTV.X264-DIMENSION", "Series"}},
		{Line: 18, Method: "should_not_parse_url_in_name", Args: Args{"test-hyphen123.org.au - Series.S03E14.720p.HDTV.X264-DIMENSION", "Series"}},
		{Line: 19, Method: "should_not_parse_url_in_name", Args: Args{"[www.test123.de] - Mad Series - Season 1 [Bluray720p]", "Mad Series"}},
		{Line: 20, Method: "should_not_parse_url_in_name", Args: Args{"www.test-hyphen.de - Mad Series - Season 1 [Bluray1080p]", "Mad Series"}},
		{Line: 21, Method: "should_not_parse_url_in_name", Args: Args{"[test-hyphen123.co.za] - The Daily Series - 2023-05-26", "The Daily Series"}},
		{Line: 22, Method: "should_not_parse_url_in_name", Args: Args{"www.test123.co.za - The Series Bros. (2006) - S01E01", "The Series Bros. (2006)"}},
		{Line: 23, Method: "should_not_parse_url_in_name", Args: Args{"[www.test-hyphen.ca] - Series (2011) S01", "Series (2011)"}},
		{Line: 24, Method: "should_not_parse_url_in_name", Args: Args{"test123.ca - Series Time S02 720p HDTV x264 CRON", "Series Time"}},
		{Line: 25, Method: "should_not_parse_url_in_name", Args: Args{"[www.test-hyphen123.co.za] - Series Title S01E01", "Series Title"}},
		{Line: 26, Method: "should_not_parse_url_in_name", Args: Args{"(seriesawake.com) Series Super - 57 [720p] [English Subbed]", "Series Super"}},
		{Line: 34, Method: "should_not_parse_url_in_group", Args: Args{"Series.2009.S01E14.English.HDTV.XviD-LOL[www.abb.com]", "LOL"}},
		{Line: 35, Method: "should_not_parse_url_in_group", Args: Args{"Series 2009 S01E14 English HDTV XviD LOL[www.academy.org]", nil}},
		{Line: 36, Method: "should_not_parse_url_in_group", Args: Args{"Series Now S05 EXTRAS DVDRip XviD RUNNER[www.aetna.net]", nil}},
		{Line: 37, Method: "should_not_parse_url_in_group", Args: Args{"Series.Title.S01.EXTRAS.DVDRip.XviD-RUNNER[www.alfaromeo.io]", "RUNNER"}},
		{Line: 38, Method: "should_not_parse_url_in_group", Args: Args{"2020.Series.2011.12.02.PDTV.XviD-C4TV[rarbg.to]", "C4TV"}},
		{Line: 39, Method: "should_not_parse_url_in_group", Args: Args{"Series.Title.S01E14.English.HDTV.XviD-LOL[www.abbott.gov]", "LOL"}},
		{Line: 40, Method: "should_not_parse_url_in_group", Args: Args{"Series 2020 S01E14 English HDTV XviD LOL[www.actor.org]", nil}},
		{Line: 41, Method: "should_not_parse_url_in_group", Args: Args{"Series Live S05 EXTRAS DVDRip XviD RUNNER[www.agency.net]", nil}},
		{Line: 42, Method: "should_not_parse_url_in_group", Args: Args{"Series.Title.S02.EXTRAS.DVDRip.XviD-RUNNER[www.airbus.io]", "RUNNER"}},
		{Line: 43, Method: "should_not_parse_url_in_group", Args: Args{"2021.Series.2012.12.02.PDTV.XviD-C4TV[rarbg.to]", "C4TV"}},
		{Line: 44, Method: "should_not_parse_url_in_group", Args: Args{"Series.2020.S01E14.English.HDTV.XviD-LOL[www.afl.com]", "LOL"}},
		{Line: 45, Method: "should_not_parse_url_in_group", Args: Args{"Series 2021 S01E14 English HDTV XviD LOL[www.adult.org]", nil}},
		{Line: 46, Method: "should_not_parse_url_in_group", Args: Args{"Series Future S05 EXTRAS DVDRip XviD RUNNER[www.allstate.net]", nil}},
		{Line: 47, Method: "should_not_parse_url_in_group", Args: Args{"Series.Title.S03.EXTRAS.DVDRip.XviD-RUNNER[www.ally.io]", "RUNNER"}},
		{Line: 48, Method: "should_not_parse_url_in_group", Args: Args{"2022.Series.2013.12.02.PDTV.XviD-C4TV[rarbg.to]", "C4TV"}},
	},
}
