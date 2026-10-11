// Code generated from Radarr's parser test fixtures. DO NOT EDIT.
//
// Source: https://github.com/Radarr/Radarr/blob/f5ccd1ca7888b70d43fc22d0108154aa753deba8/src/NzbDrone.Core.Test/ParserTests/AnimeVersionFixture.cs
// Copyright (C) Radarr contributors. Licensed under GPL-3.0-or-later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fixtures

var RadarrAnimeVersionFixture = File{
	Source: "Radarr/src/NzbDrone.Core.Test/ParserTests/AnimeVersionFixture.cs",
	Cases: []Case{
		{Line: 11, Method: "should_be_able_to_parse_repack", Args: Args{"Anime Title - 2018 - (BD 1080p HEVC FLAC) [Dual Audio] [Group]", 1}},
		{Line: 12, Method: "should_be_able_to_parse_repack", Args: Args{"Anime Title - 2018v2 - (BD 1080p HEVC FLAC) [Dual Audio] [Group]", 2}},
		{Line: 13, Method: "should_be_able_to_parse_repack", Args: Args{"Anime Title - 2018 v2 - (BD 1080p HEVC FLAC) [Dual Audio] [Group]", 2}},
		{Line: 14, Method: "should_be_able_to_parse_repack", Args: Args{"[SubsPlease] Anime Title - 01 (1080p) [B1F227CF]", 1}},
		{Line: 15, Method: "should_be_able_to_parse_repack", Args: Args{"[SubsPlease] Anime Title - 01v2 (1080p) [B1F227CF]", 2}},
		{Line: 16, Method: "should_be_able_to_parse_repack", Args: Args{"[SubsPlease] Anime Title - 01 v2 (1080p) [B1F227CF]", 2}},
	},
}
