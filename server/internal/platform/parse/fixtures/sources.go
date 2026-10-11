// Copyright (C) Sonarr and Radarr contributors. Licensed under GPL-3.0-or-later.
// SPDX-License-Identifier: GPL-3.0-or-later
//
// Hand-ported [TestCaseSource] arrays, which the generator can't read:
//   Sonarr 9d0920b470b033106ca2913572970324244e8fb0 src/NzbDrone.Core.Test/ParserTests/HashedReleaseFixture.cs
//   Sonarr 9d0920b470b033106ca2913572970324244e8fb0 src/NzbDrone.Core.Test/ParserTests/QualityParserFixture.cs
//   Radarr f5ccd1ca7888b70d43fc22d0108154aa753deba8 src/NzbDrone.Core.Test/ParserTests/HashedReleaseFixture.cs
//   Radarr f5ccd1ca7888b70d43fc22d0108154aa753deba8 src/NzbDrone.Core.Test/ParserTests/QualityParserFixture.cs

package fixtures

// HashedRelease is a path whose file name is a hash, so the parser must fall back to the folder.
// Quality is the C# Quality property name; Group is empty for C# null.
type HashedRelease struct {
	Path    string
	Title   string
	Quality string
	Group   string
}

// SonarrHashedReleases is HashedReleaseFixture.HashedReleaseParserCases, paths before AsOsAgnostic().
var SonarrHashedReleases = []HashedRelease{
	{`C:\Test\Some.Hashed.Release.S01E01.720p.WEB-DL.AAC2.0.H.264-Mercury\0e895c37245186812cb08aab1529cf8ee389dd05.mkv`, "Some Hashed Release", "WEBDL720p", "Mercury"},
	{`C:\Test\0e895c37245186812cb08aab1529cf8ee389dd05\Some.Hashed.Release.S01E01.720p.WEB-DL.AAC2.0.H.264-Mercury.mkv`, "Some Hashed Release", "WEBDL720p", "Mercury"},
	{`C:\Test\Fake.Dir.S01E01-Test\yrucreM-462.H.0.2CAA.LD-BEW.p027.10E10S.esaeleR.dehsaH.emoS.mkv`, "Some Hashed Release", "WEBDL720p", "Mercury"},
	{`C:\Test\Fake.Dir.S01E01-Test\yrucreM-LN 1.5DD LD-BEW P0801 10E10S esaeleR dehsaH emoS.mkv`, "Some Hashed Release", "WEBDL1080p", "Mercury"},
	{`C:\Test\Title.S01E10.DVDRip.XviD-SONARR\AHFMZXGHEWD660.mkv`, "Title", "DVD", "SONARR"},
	{`C:\Test\Show Title.S02E12.1080p.BluRay.x264-SONARR\Backup_72023S02-12.mkv`, "Show Title", "Bluray1080p", "SONARR"},
	{`C:\Test\Title S04E08 Chupacabra 720p WEB-DL DD5 1 H 264-ECI\123.mkv`, "Title", "WEBDL720p", "ECI"},
	{`C:\Test\Title S04E08 Chupacabra 720p WEB-DL DD5 1 H 264-ECI\abc.mkv`, "Title", "WEBDL720p", "ECI"},
	{`C:\Test\Title S04E08 Chupacabra 720p WEB-DL DD5 1 H 264-ECI\b00bs.mkv`, "Title", "WEBDL720p", "ECI"},
	{`C:\Test\The.Show.Title.S02E23.720p.HDTV.x264-NZBgeek/cgajsofuejsa501.mkv`, "The Show Title", "HDTV720p", "NZBgeek"},
	{`C:\Test\Show.Title.S03E04.1080p.WEB-DL.DD5.1.H264-RARBG\170424_26.mkv`, "Show Title", "WEBDL1080p", "RARBG"},
	{`C:\Test\XxQVHK4GJMP3n2dLpmhW\XxQVHK4GJMP3n2dLpmhW\MKV\010E70S.nuF.fo.snoS.mkv`, "Sons of Fun", "HDTV720p", ""},
	{`50E50S.denorD.mkv`, "Droned", "HDTV720p", ""},
	{`C:\Test\XxQVHK4GJMP3n2dLpmhW\XxQVHK4GJMP3n2dLpmhW\50E50S.denorD.mkv`, "Droned", "HDTV720p", ""},
	{`C:\Test\Series.Title.S02E24.720p.HDTV.H.264\abc.xyz.af6021c37f7852.mkv`, "Series Title", "HDTV720p", ""},
	{`C:\Test\tv\Series.Title.S04E03.Episode.Name.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb\cwnOJYks5E2WP7zGuzPkdkRK3JkWw0.mkv`, "Series Title", "WEBDL1080p", "NTb"},
	{`C:\Test\tv\Series.Title.2017.S04E07.Episode.Name.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb/s2e2fYzx5xJhuBjHn5ZXE07Ebi.mkv`, "Series Title 2017", "WEBDL1080p", "NTb"},
	{`C:\Test\tv\Series.Title.S04E03.Episode.Name.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb\zM0vnVU1SoV4uwTihfzTHkYmJPBemgN3MqXG1fY.mkv`, "Series Title", "WEBDL1080p", "NTb"},
	{`C:\Test\tv\Series.Title.2019.S02E02.The.Episode.of.Names.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb/46XuzmawYQeUBwNdH2Hw2996.mkv`, "Series Title 2019", "WEBDL1080p", "NTb"},
	{`C:\Test\Fake.Dir.S01E01-Test\11E-20S seires.mkv`, "series", "HDTV720p", ""},
}

// RadarrHashedReleases is HashedReleaseFixture.HashedReleaseParserCases, paths before AsOsAgnostic().
var RadarrHashedReleases = []HashedRelease{
	{`C:\Test\Some.Hashed.Release.2018.720p.WEB-DL.AAC2.0.H.264-Mercury\0e895c37245186812cb08aab1529cf8ee389dd05.mkv`, "Some Hashed Release", "WEBDL720p", "Mercury"},
	{`C:\Test\0e895c37245186812cb08aab1529cf8ee389dd05\Some.Hashed.Release.2018.720p.WEB-DL.AAC2.0.H.264-Mercury.mkv`, "Some Hashed Release", "WEBDL720p", "Mercury"},
	{`C:\Test\Movie.2018.DVDRip.XviD-RADARR\AHFMZXGHEWD660.mkv`, "Movie", "DVD", "RADARR"},
	{`C:\Test\Movie.2018.1080p.BluRay.x264-RADARR\Backup_72023S02-12.mkv`, "Movie", "Bluray1080p", "RADARR"},
	{`C:\Test\Movie.2018.1080p.BluRay.x264\Backup_72023S02-12.mkv`, "Movie", "Bluray1080p", ""},
	{`C:\Test\Movie 2018 720p WEB-DL DD5 1 H 264-ECI\123.mkv`, "Movie", "WEBDL720p", "ECI"},
	{`C:\Test\Movie 2018 720p WEB-DL DD5 1 H 264-ECI\abc.mkv`, "Movie", "WEBDL720p", "ECI"},
	{`C:\Test\Movie 2018 720p WEB-DL DD5 1 H 264-ECI\b00bs.mkv`, "Movie", "WEBDL720p", "ECI"},
	{`C:\Test\Movie.Title.2018.720p.HDTV.x264-NZBgeek/cgajsofuejsa501.mkv`, "Movie Title", "HDTV720p", "NZBgeek"},
	{`C:\Test\Movie.2018.1080p.WEB-DL.DD5.1.H264-RARBG\170424_26.mkv`, "Movie", "WEBDL1080p", "RARBG"},
	{`C:\Test\Movie.Title.2018.720p.HDTV.H.264\abc.xyz.af6021c37f7852.mkv`, "Movie Title", "HDTV720p", ""},
}

// SonarrSelfQualities is QualityParserFixture.SelfQualityParserCases: each quality's own name must parse back to it.
// Name is Sonarr's display name; Quality is the C# property name.
var SonarrSelfQualities = []struct{ Name, Quality string }{
	{"SDTV", "SDTV"},
	{"DVD", "DVD"},
	{"WEBDL-480p", "WEBDL480p"},
	{"Bluray-480p", "Bluray480p"},
	{"Bluray-576p", "Bluray576p"},
	{"HDTV-720p", "HDTV720p"},
	{"HDTV-1080p", "HDTV1080p"},
	{"HDTV-2160p", "HDTV2160p"},
	{"WEBDL-720p", "WEBDL720p"},
	{"WEBDL-1080p", "WEBDL1080p"},
	{"WEBDL-2160p", "WEBDL2160p"},
	{"Bluray-720p", "Bluray720p"},
	{"Bluray-1080p", "Bluray1080p"},
	{"Bluray-2160p", "Bluray2160p"},
	{"Bluray-1080p Remux", "Bluray1080pRemux"},
	{"Bluray-2160p Remux", "Bluray2160pRemux"},
}

// SonarrOtherSourceQualities is QualityParserFixture.OtherSourceQualityParserCases.
var SonarrOtherSourceQualities = []struct{ Text, Quality string }{
	{"SD TV", "SDTV"},
	{"SD DVD", "DVD"},
	{"480p WEB-DL", "WEBDL480p"},
	{"HD TV", "HDTV720p"},
	{"1080p HD TV", "HDTV1080p"},
	{"2160p HD TV", "HDTV2160p"},
	{"720p WEB-DL", "WEBDL720p"},
	{"1080p WEB-DL", "WEBDL1080p"},
	{"2160p WEB-DL", "WEBDL2160p"},
	{"720p BluRay", "Bluray720p"},
	{"1080p BluRay", "Bluray1080p"},
	{"2160p BluRay", "Bluray2160p"},
	{"1080p Remux", "Bluray1080pRemux"},
	{"2160p Remux", "Bluray2160pRemux"},
}

// RadarrOtherSourceQualities is QualityParserFixture.OtherSourceQualityParserCases.
// Source, Resolution and Modifier are the C# enum member names.
var RadarrOtherSourceQualities = []struct{ Text, Source, Resolution, Modifier string }{
	{"SD TV", "TV", "R480p", "NONE"},
	{"SD DVD", "DVD", "Unknown", "NONE"},
	{"480p WEB-DL", "WEBDL", "R480p", "NONE"},
	{"HD TV", "TV", "R720p", "NONE"},
	{"1080p HD TV", "TV", "R1080p", "NONE"},
	{"2160p HD TV", "TV", "R2160p", "NONE"},
	{"720p WEB-DL", "WEBDL", "R720p", "NONE"},
	{"1080p WEB-DL", "WEBDL", "R1080p", "NONE"},
	{"2160p WEB-DL", "WEBDL", "R2160p", "NONE"},
	{"720p BluRay", "BLURAY", "R720p", "NONE"},
	{"1080p BluRay", "BLURAY", "R1080p", "NONE"},
	{"2160p BluRay", "BLURAY", "R2160p", "NONE"},
	{"1080p Remux", "BLURAY", "R1080p", "REMUX"},
	{"2160p Remux", "BLURAY", "R2160p", "REMUX"},
}
