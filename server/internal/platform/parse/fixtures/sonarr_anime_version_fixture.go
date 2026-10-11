// Code generated from Sonarr's parser test fixtures. DO NOT EDIT.
//
// Source: https://github.com/Sonarr/Sonarr/blob/9d0920b470b033106ca2913572970324244e8fb0/src/NzbDrone.Core.Test/ParserTests/AnimeVersionFixture.cs
// Copyright (C) Sonarr contributors. Licensed under GPL-3.0-or-later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fixtures

var SonarrAnimeVersionFixture = File{
	Source: "Sonarr/src/NzbDrone.Core.Test/ParserTests/AnimeVersionFixture.cs",
	Cases: []Case{
		{Line: 11, Method: "should_be_able_to_parse_repack", Args: Args{"Anime Title - S01E01 - (BD 1080p HEVC FLAC) [Dual Audio] [Group]", 1}},
		{Line: 12, Method: "should_be_able_to_parse_repack", Args: Args{"Anime Title - S01E01v2 - (BD 1080p HEVC FLAC) [Dual Audio] [Group]", 2}},
		{Line: 13, Method: "should_be_able_to_parse_repack", Args: Args{"Anime Title - S01E01 v2 - (BD 1080p HEVC FLAC) [Dual Audio] [Group]", 2}},
		{Line: 14, Method: "should_be_able_to_parse_repack", Args: Args{"[SubsPlease] Anime Title - 01 (1080p) [B1F227CF]", 1}},
		{Line: 15, Method: "should_be_able_to_parse_repack", Args: Args{"[SubsPlease] Anime Title - 01v2 (1080p) [B1F227CF]", 2}},
		{Line: 16, Method: "should_be_able_to_parse_repack", Args: Args{"[SubsPlease] Anime Title - 01 v2 (1080p) [B1F227CF]", 2}},
	},
}
