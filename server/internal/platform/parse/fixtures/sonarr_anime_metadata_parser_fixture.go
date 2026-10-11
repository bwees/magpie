// Code generated from Sonarr's parser test fixtures. DO NOT EDIT.
//
// Source: https://github.com/Sonarr/Sonarr/blob/9d0920b470b033106ca2913572970324244e8fb0/src/NzbDrone.Core.Test/ParserTests/AnimeMetadataParserFixture.cs
// Copyright (C) Sonarr contributors. Licensed under GPL-3.0-or-later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fixtures

var SonarrAnimeMetadataParserFixture = File{
	Source: "Sonarr/src/NzbDrone.Core.Test/ParserTests/AnimeMetadataParserFixture.cs",
	Cases: []Case{
		{Line: 10, Method: "should_parse_releasegroup_and_hash", Args: Args{"[SubDESU]_Show_Title_DxD_07_(1280x720_x264-AAC)_[6B7FD717]", "SubDESU", "6B7FD717"}},
		{Line: 11, Method: "should_parse_releasegroup_and_hash", Args: Args{"[Chihiro]_Show_Title!!_-_06_[848x480_H.264_AAC][859EEAFA]", "Chihiro", "859EEAFA"}},
		{Line: 12, Method: "should_parse_releasegroup_and_hash", Args: Args{"[Underwater]_Show_Title_-_12_(720p)_[5C7BC4F9]", "Underwater", "5C7BC4F9"}},
		{Line: 13, Method: "should_parse_releasegroup_and_hash", Args: Args{"[HorribleSubs]_Show_Title_-_33_[720p]", "HorribleSubs", ""}},
		{Line: 14, Method: "should_parse_releasegroup_and_hash", Args: Args{"[HorribleSubs] Show-Title - 13 [1080p].mkv", "HorribleSubs", ""}},
		{Line: 15, Method: "should_parse_releasegroup_and_hash", Args: Args{"[Doremi].Show.Title.5.Go.Go!.31.[1280x720].[C65D4B1F].mkv", "Doremi", "C65D4B1F"}},
		{Line: 16, Method: "should_parse_releasegroup_and_hash", Args: Args{"[Doremi].Show.Title.5.Go.Go!.31[1280x720].[C65D4B1F]", "Doremi", "C65D4B1F"}},
		{Line: 17, Method: "should_parse_releasegroup_and_hash", Args: Args{"[Doremi].Show.Title.5.Go.Go!.31.[1280x720].mkv", "Doremi", ""}},
		{Line: 18, Method: "should_parse_releasegroup_and_hash", Args: Args{"[K-F] Series Title 214", "K-F", ""}},
		{Line: 19, Method: "should_parse_releasegroup_and_hash", Args: Args{"[K-F] Series Title S10E14 214", "K-F", ""}},
		{Line: 20, Method: "should_parse_releasegroup_and_hash", Args: Args{"[K-F] Series Title 10x14 214", "K-F", ""}},
		{Line: 21, Method: "should_parse_releasegroup_and_hash", Args: Args{"[K-F] Series Title 214 10x14", "K-F", ""}},
		{Line: 22, Method: "should_parse_releasegroup_and_hash", Args: Args{"Series Title - 031 - The Resolution to Kill [Lunar].avi", "Lunar", ""}},
		{Line: 23, Method: "should_parse_releasegroup_and_hash", Args: Args{"[ACX]Series Title 01 Episode Name [Kosaka] [9C57891E].mkv", "ACX", "9C57891E"}},
		{Line: 24, Method: "should_parse_releasegroup_and_hash", Args: Args{"[S-T-D] Series Title! - 06 (1280x720 10bit AAC) [59B3F2EA].mkv", "S-T-D", "59B3F2EA"}},
		{Line: 27, Method: "should_parse_releasegroup_and_hash", Args: Args{"[SubDESU]_Show_Title_DxD_07_(1280x720_x264-AAC)_(6B7FD717)", "SubDESU", "6B7FD717"}},
		{Line: 28, Method: "should_parse_releasegroup_and_hash", Args: Args{"[Chihiro]_Show_Title!!_-_06_[848x480_H.264_AAC](859EEAFA)", "Chihiro", "859EEAFA"}},
		{Line: 29, Method: "should_parse_releasegroup_and_hash", Args: Args{"[Underwater]_Show_Title_-_12_(720p)_(5C7BC4F9)", "Underwater", "5C7BC4F9"}},
		{Line: 30, Method: "should_parse_releasegroup_and_hash", Args: Args{"[HorribleSubs]_Show_Title_-_33_[720p]", "HorribleSubs", ""}},
		{Line: 31, Method: "should_parse_releasegroup_and_hash", Args: Args{"[HorribleSubs] Show-Title - 13 [1080p].mkv", "HorribleSubs", ""}},
		{Line: 32, Method: "should_parse_releasegroup_and_hash", Args: Args{"[Doremi].Show.Title.5.Go.Go!.31.[1280x720].(C65D4B1F).mkv", "Doremi", "C65D4B1F"}},
		{Line: 33, Method: "should_parse_releasegroup_and_hash", Args: Args{"[Doremi].Show.Title.5.Go.Go!.31[1280x720].(C65D4B1F)", "Doremi", "C65D4B1F"}},
		{Line: 34, Method: "should_parse_releasegroup_and_hash", Args: Args{"[Doremi].Show.Title.5.Go.Go!.31.[1280x720].mkv", "Doremi", ""}},
		{Line: 35, Method: "should_parse_releasegroup_and_hash", Args: Args{"[K-F] Series Title 214", "K-F", ""}},
		{Line: 36, Method: "should_parse_releasegroup_and_hash", Args: Args{"[K-F] Series Title S10E14 214", "K-F", ""}},
		{Line: 37, Method: "should_parse_releasegroup_and_hash", Args: Args{"[K-F] Series Title 10x14 214", "K-F", ""}},
		{Line: 38, Method: "should_parse_releasegroup_and_hash", Args: Args{"[K-F] Series Title 214 10x14", "K-F", ""}},
		{Line: 39, Method: "should_parse_releasegroup_and_hash", Args: Args{"Series Title - 031 - The Resolution to Kill [Lunar].avi", "Lunar", ""}},
		{Line: 40, Method: "should_parse_releasegroup_and_hash", Args: Args{"[ACX]Series Title 01 Episode Name [Kosaka] (9C57891E).mkv", "ACX", "9C57891E"}},
		{Line: 41, Method: "should_parse_releasegroup_and_hash", Args: Args{"[S-T-D] Series Title! - 06 (1280x720 10bit AAC) (59B3F2EA).mkv", "S-T-D", "59B3F2EA"}},
		{Line: 50, Method: "should_parse_release_hashes_with_10b_or_8b", Args: Args{"[DHD] Series Title! - 08 (1280x720 10bit AAC) [8B00F2EA].mkv", "8B00F2EA"}},
		{Line: 51, Method: "should_parse_release_hashes_with_10b_or_8b", Args: Args{"[DHD] Series Title! - 10 (1280x720 10bit AAC) [10BBF2EA].mkv", "10BBF2EA"}},
		{Line: 52, Method: "should_parse_release_hashes_with_10b_or_8b", Args: Args{"[DHD] Series Title! - 08 (1280x720 10bit AAC) [008BF28B].mkv", "008BF28B"}},
		{Line: 53, Method: "should_parse_release_hashes_with_10b_or_8b", Args: Args{"[DHD] Series Title! - 10 (1280x720 10bit AAC) [000BF10B].mkv", "000BF10B"}},
		{Line: 54, Method: "should_parse_release_hashes_with_10b_or_8b", Args: Args{"[DHD] Series Title! - 08 (1280x720 8bit AAC) [8B8BF2EA].mkv", "8B8BF2EA"}},
		{Line: 55, Method: "should_parse_release_hashes_with_10b_or_8b", Args: Args{"[DHD] Series Title! - 10 (1280x720 8bit AAC) [10B10BEA].mkv", "10B10BEA"}},
	},
}
