// Code generated from Radarr's parser test fixtures. DO NOT EDIT.
//
// Source: https://github.com/Radarr/Radarr/blob/f5ccd1ca7888b70d43fc22d0108154aa753deba8/src/NzbDrone.Core.Test/ParserTests/UrlFixture.cs
// Copyright (C) Radarr contributors. Licensed under GPL-3.0-or-later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fixtures

var RadarrUrlFixture = File{
	Source: "Radarr/src/NzbDrone.Core.Test/ParserTests/UrlFixture.cs",
	Cases: []Case{
		{Line: 11, Method: "should_not_parse_url_in_name", Args: Args{"[www.test.com] - Movie.Title.2023.720p.HDTV.X264-DIMENSION", "Movie Title"}},
		{Line: 12, Method: "should_not_parse_url_in_name", Args: Args{"test.net - Movie.Title.2023.720p.HDTV.X264-DIMENSION", "Movie Title"}},
		{Line: 13, Method: "should_not_parse_url_in_name", Args: Args{"[www.test-hyphen.com] - Movie.Title.2023.720p.HDTV.X264-DIMENSION", "Movie Title"}},
		{Line: 14, Method: "should_not_parse_url_in_name", Args: Args{"www.test123.org - Movie.Title.2023.720p.HDTV.X264-DIMENSION", "Movie Title"}},
		{Line: 15, Method: "should_not_parse_url_in_name", Args: Args{"[test.co.uk] - Movie.Title.2023.720p.HDTV.X264-DIMENSION", "Movie Title"}},
		{Line: 16, Method: "should_not_parse_url_in_name", Args: Args{"www.test-hyphen.net.au - Movie.Title.2023.720p.HDTV.X264-DIMENSION", "Movie Title"}},
		{Line: 17, Method: "should_not_parse_url_in_name", Args: Args{"[www.test123.co.nz] - Movie.Title.2023.720p.HDTV.X264-DIMENSION", "Movie Title"}},
		{Line: 18, Method: "should_not_parse_url_in_name", Args: Args{"test-hyphen123.org.au - Movie.Title.2023.720p.HDTV.X264-DIMENSION", "Movie Title"}},
		{Line: 19, Method: "should_not_parse_url_in_name", Args: Args{"[www.test123.de] - Mad Movie Title 2023 [Bluray720p]", "Mad Movie Title"}},
		{Line: 20, Method: "should_not_parse_url_in_name", Args: Args{"www.test-hyphen.de - Mad Movie Title 2023 [Bluray1080p]", "Mad Movie Title"}},
		{Line: 21, Method: "should_not_parse_url_in_name", Args: Args{"www.test123.co.za - The Movie Title Bros. (2023)", "The Movie Title Bros."}},
		{Line: 22, Method: "should_not_parse_url_in_name", Args: Args{"[www.test-hyphen.ca] - Movie Title (2023)", "Movie Title"}},
		{Line: 23, Method: "should_not_parse_url_in_name", Args: Args{"test123.ca - Movie Time 2023 720p HDTV x264 CRON", "Movie Time"}},
		{Line: 24, Method: "should_not_parse_url_in_name", Args: Args{"[www.test-hyphen123.co.za] - Movie Title 2023", "Movie Title"}},
		{Line: 25, Method: "should_not_parse_url_in_name", Args: Args{"(movieawake.com) Movie Title 2023 [720p] [English Subbed]", "Movie Title"}},
		{Line: 32, Method: "should_not_parse_url_in_group", Args: Args{"Movie.2023.English.HDTV.XviD-LOL[www.abb.com]", "LOL"}},
		{Line: 33, Method: "should_not_parse_url_in_group", Args: Args{"Movie Title 2023 English HDTV XviD LOL[www.academy.org]", nil}},
		{Line: 34, Method: "should_not_parse_url_in_group", Args: Args{"Movie Title Now 2023 DVDRip XviD RUNNER[www.aetna.net]", nil}},
		{Line: 35, Method: "should_not_parse_url_in_group", Args: Args{"Movie.Title.2023.DVDRip.XviD-RUNNER[www.alfaromeo.io]", "RUNNER"}},
		{Line: 36, Method: "should_not_parse_url_in_group", Args: Args{"Movie.Title.2023.English.HDTV.XviD-LOL[www.abbott.gov]", "LOL"}},
		{Line: 37, Method: "should_not_parse_url_in_group", Args: Args{"Movie Title 2023 English HDTV XviD LOL[www.actor.org]", nil}},
		{Line: 38, Method: "should_not_parse_url_in_group", Args: Args{"Movie Title Future 2023 DVDRip XviD RUNNER[www.allstate.net]", nil}},
	},
}
