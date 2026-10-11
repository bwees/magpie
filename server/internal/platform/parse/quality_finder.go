// Copyright (C) Magpie contributors.
// Portions ported from Sonarr (https://github.com/Sonarr/Sonarr) and
// Radarr (https://github.com/Radarr/Radarr), copyright their contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import "slices"

// sourceRank is a position in Sonarr's or Radarr's QualitySource enum. The finders
// fall back to the nearest quality whose source ranks at or above the wanted one.
type sourceRank int

// Sonarr's QualitySource enum.
const (
	sonarrUnknown sourceRank = iota
	sonarrTelevision
	sonarrTelevisionRaw
	sonarrWeb
	sonarrWebRip
	sonarrDVD
	sonarrBluray
	sonarrBlurayRaw
)

// Radarr's QualitySource enum.
const (
	radarrUnknown sourceRank = iota
	radarrCam
	radarrTelesync
	radarrTelecine
	radarrWorkprint
	radarrDVD
	radarrTV
	radarrWebDL
	radarrWebRip
	radarrBluray
)

type finderEntry struct {
	id         QualityID
	rank       sourceRank
	resolution Resolution
	modifier   QualityModifier
}

// sonarrQualities is Sonarr's Quality.All with Sonarr's sources and resolutions.
func sonarrQualities() []finderEntry {
	return []finderEntry{
		{QualityUnknown, sonarrUnknown, ResolutionUnknown, ModifierNone},
		{QualitySDTV, sonarrTelevision, Resolution480p, ModifierNone},
		{QualityDVD, sonarrDVD, Resolution480p, ModifierNone},
		{QualityWEBDL1080p, sonarrWeb, Resolution1080p, ModifierNone},
		{QualityHDTV720p, sonarrTelevision, Resolution720p, ModifierNone},
		{QualityWEBDL720p, sonarrWeb, Resolution720p, ModifierNone},
		{QualityBluray720p, sonarrBluray, Resolution720p, ModifierNone},
		{QualityBluray1080p, sonarrBluray, Resolution1080p, ModifierNone},
		{QualityWEBDL480p, sonarrWeb, Resolution480p, ModifierNone},
		{QualityHDTV1080p, sonarrTelevision, Resolution1080p, ModifierNone},
		{QualityRawHD, sonarrTelevisionRaw, Resolution1080p, ModifierNone},
		{QualityWEBRip480p, sonarrWebRip, Resolution480p, ModifierNone},
		{QualityBluray480p, sonarrBluray, Resolution480p, ModifierNone},
		{QualityWEBRip720p, sonarrWebRip, Resolution720p, ModifierNone},
		{QualityWEBRip1080p, sonarrWebRip, Resolution1080p, ModifierNone},
		{QualityHDTV2160p, sonarrTelevision, Resolution2160p, ModifierNone},
		{QualityWEBRip2160p, sonarrWebRip, Resolution2160p, ModifierNone},
		{QualityWEBDL2160p, sonarrWeb, Resolution2160p, ModifierNone},
		{QualityBluray2160p, sonarrBluray, Resolution2160p, ModifierNone},
		{QualityRemux1080p, sonarrBlurayRaw, Resolution1080p, ModifierNone},
		{QualityRemux2160p, sonarrBlurayRaw, Resolution2160p, ModifierNone},
		{QualityBluray576p, sonarrBluray, Resolution576p, ModifierNone},
	}
}

// radarrQualities is Radarr's Quality.All with Radarr's sources, resolutions and modifiers.
func radarrQualities() []finderEntry {
	return []finderEntry{
		{QualityUnknown, radarrUnknown, ResolutionUnknown, ModifierNone},
		{QualityWorkprint, radarrWorkprint, ResolutionUnknown, ModifierNone},
		{QualityCam, radarrCam, ResolutionUnknown, ModifierNone},
		{QualityTelesync, radarrTelesync, ResolutionUnknown, ModifierNone},
		{QualityTelecine, radarrTelecine, ResolutionUnknown, ModifierNone},
		{QualityDVDSCR, radarrDVD, Resolution480p, ModifierScreener},
		{QualityRegional, radarrDVD, Resolution480p, ModifierRegional},
		{QualitySDTV, radarrTV, Resolution480p, ModifierNone},
		{QualityDVD, radarrDVD, ResolutionUnknown, ModifierNone},
		{QualityDVDR, radarrDVD, Resolution480p, ModifierRemux},
		{QualityHDTV720p, radarrTV, Resolution720p, ModifierNone},
		{QualityHDTV1080p, radarrTV, Resolution1080p, ModifierNone},
		{QualityHDTV2160p, radarrTV, Resolution2160p, ModifierNone},
		{QualityWEBDL480p, radarrWebDL, Resolution480p, ModifierNone},
		{QualityWEBDL720p, radarrWebDL, Resolution720p, ModifierNone},
		{QualityWEBDL1080p, radarrWebDL, Resolution1080p, ModifierNone},
		{QualityWEBDL2160p, radarrWebDL, Resolution2160p, ModifierNone},
		{QualityWEBRip480p, radarrWebRip, Resolution480p, ModifierNone},
		{QualityWEBRip720p, radarrWebRip, Resolution720p, ModifierNone},
		{QualityWEBRip1080p, radarrWebRip, Resolution1080p, ModifierNone},
		{QualityWEBRip2160p, radarrWebRip, Resolution2160p, ModifierNone},
		{QualityBluray480p, radarrBluray, Resolution480p, ModifierNone},
		{QualityBluray576p, radarrBluray, Resolution576p, ModifierNone},
		{QualityBluray720p, radarrBluray, Resolution720p, ModifierNone},
		{QualityBluray1080p, radarrBluray, Resolution1080p, ModifierNone},
		{QualityBluray2160p, radarrBluray, Resolution2160p, ModifierNone},
		{QualityRemux1080p, radarrBluray, Resolution1080p, ModifierRemux},
		{QualityRemux2160p, radarrBluray, Resolution2160p, ModifierRemux},
		{QualityBRDisk, radarrBluray, Resolution1080p, ModifierBRDisk},
		{QualityRawHD, radarrTV, Resolution1080p, ModifierRawHD},
	}
}

func entryFor(entries []finderEntry, id QualityID) finderEntry {
	for _, e := range entries {
		if e.id == id {
			return e
		}
	}
	return entries[0]
}

// nearestByRank returns the first entry, in source order, ranked at or above rank.
func nearestByRank(entries []finderEntry, rank sourceRank) QualityID {
	slices.SortStableFunc(entries, func(a, b finderEntry) int { return int(a.rank - b.rank) })
	for _, e := range entries {
		if e.rank >= rank {
			return e.id
		}
	}
	return QualityUnknown
}

// sonarrFindQuality ports Sonarr's QualityFinder.FindBySourceAndResolution.
func sonarrFindQuality(source QualityID, remux bool, resolution Resolution) QualityID {
	all := sonarrQualities()
	rank := entryFor(all, source).rank
	if remux {
		rank = sonarrBlurayRaw
	}

	var atResolution []finderEntry
	for _, e := range all {
		if e.rank == rank && e.resolution == resolution {
			return e.id
		}
		if e.resolution == resolution {
			atResolution = append(atResolution, e)
		}
	}

	if resolution < Resolution720p {
		switch rank {
		case sonarrTelevision:
			return QualitySDTV
		case sonarrWeb:
			return QualityWEBDL480p
		case sonarrWebRip:
			return QualityWEBRip480p
		}
	}
	return nearestByRank(atResolution, rank)
}

// radarrFindQuality ports Radarr's QualityFinder.FindBySourceAndResolution.
func radarrFindQuality(source QualityID, remux bool, resolution Resolution) QualityID {
	all := radarrQualities()
	rank := entryFor(all, source).rank
	modifier := ModifierNone
	if remux {
		rank = radarrBluray
		modifier = ModifierRemux
	}

	var unknownResolution, atResolution []finderEntry
	for _, e := range all {
		if e.modifier != modifier {
			continue
		}
		if e.rank == rank && e.resolution == resolution {
			return e.id
		}
		if e.rank == rank && e.resolution == ResolutionUnknown && e.id != QualityUnknown {
			unknownResolution = append(unknownResolution, e)
		}
		if e.resolution == resolution {
			atResolution = append(atResolution, e)
		}
	}

	if len(unknownResolution) > 0 {
		return unknownResolution[0].id
	}
	return nearestByRank(atResolution, rank)
}
