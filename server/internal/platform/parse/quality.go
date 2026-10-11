// Copyright (C) Magpie contributors.
// Portions ported from Radarr (https://github.com/Radarr/Radarr), copyright its contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import "magpie/internal/domain"

// QualityID is Magpie's quality ID, which is Radarr's quality ID.
type QualityID int

const (
	QualityUnknown     QualityID = 0
	QualitySDTV        QualityID = 1
	QualityDVD         QualityID = 2
	QualityWEBDL1080p  QualityID = 3
	QualityHDTV720p    QualityID = 4
	QualityWEBDL720p   QualityID = 5
	QualityBluray720p  QualityID = 6
	QualityBluray1080p QualityID = 7
	QualityWEBDL480p   QualityID = 8
	QualityHDTV1080p   QualityID = 9
	QualityRawHD       QualityID = 10
	QualityWEBRip480p  QualityID = 12
	QualityWEBRip720p  QualityID = 14
	QualityWEBRip1080p QualityID = 15
	QualityHDTV2160p   QualityID = 16
	QualityWEBRip2160p QualityID = 17
	QualityWEBDL2160p  QualityID = 18
	QualityBluray2160p QualityID = 19
	QualityBluray480p  QualityID = 20
	QualityBluray576p  QualityID = 21
	QualityBRDisk      QualityID = 22
	QualityDVDR        QualityID = 23
	QualityWorkprint   QualityID = 24
	QualityCam         QualityID = 25
	QualityTelesync    QualityID = 26
	QualityTelecine    QualityID = 27
	QualityDVDSCR      QualityID = 28
	QualityRegional    QualityID = 29
	QualityRemux1080p  QualityID = 30
	QualityRemux2160p  QualityID = 31
)

// QualitySource is where a release was captured from.
type QualitySource string

const (
	SourceUnknown   QualitySource = "UNKNOWN"
	SourceCam       QualitySource = "CAM"
	SourceTelesync  QualitySource = "TELESYNC"
	SourceTelecine  QualitySource = "TELECINE"
	SourceWorkprint QualitySource = "WORKPRINT"
	SourceDVD       QualitySource = "DVD"
	SourceTV        QualitySource = "TV"
	SourceWebDL     QualitySource = "WEBDL"
	SourceWebRip    QualitySource = "WEBRIP"
	SourceBluray    QualitySource = "BLURAY"
)

// QualityModifier tells apart qualities that share a source and resolution.
type QualityModifier string

const (
	ModifierNone     QualityModifier = "NONE"
	ModifierRegional QualityModifier = "REGIONAL"
	ModifierScreener QualityModifier = "SCREENER"
	ModifierRawHD    QualityModifier = "RAWHD"
	ModifierBRDisk   QualityModifier = "BRDISK"
	ModifierRemux    QualityModifier = "REMUX"
)

// Resolution is a vertical resolution in lines.
type Resolution int

const (
	ResolutionUnknown Resolution = 0
	Resolution360p    Resolution = 360
	Resolution480p    Resolution = 480
	Resolution540p    Resolution = 540
	Resolution576p    Resolution = 576
	Resolution720p    Resolution = 720
	Resolution1080p   Resolution = 1080
	Resolution2160p   Resolution = 2160
)

// QualityDefinition is one entry of the quality catalogue.
type QualityDefinition struct {
	ID         QualityID
	Name       string
	Source     QualitySource
	Resolution Resolution
	Modifier   QualityModifier
}

// Quality returns the shared-kernel value for this definition at a revision version.
func (d QualityDefinition) Quality(revision int) domain.Quality {
	return domain.Quality{
		ID:         int(d.ID),
		Name:       d.Name,
		Source:     string(d.Source),
		Resolution: int(d.Resolution),
		Revision:   revision,
	}
}

// Qualities returns Radarr's 30 qualities, ordered by ID.
func Qualities() []QualityDefinition {
	return []QualityDefinition{
		{QualityUnknown, "Unknown", SourceUnknown, ResolutionUnknown, ModifierNone},
		{QualitySDTV, "SDTV", SourceTV, Resolution480p, ModifierNone},
		{QualityDVD, "DVD", SourceDVD, Resolution480p, ModifierNone},
		{QualityWEBDL1080p, "WEBDL-1080p", SourceWebDL, Resolution1080p, ModifierNone},
		{QualityHDTV720p, "HDTV-720p", SourceTV, Resolution720p, ModifierNone},
		{QualityWEBDL720p, "WEBDL-720p", SourceWebDL, Resolution720p, ModifierNone},
		{QualityBluray720p, "Bluray-720p", SourceBluray, Resolution720p, ModifierNone},
		{QualityBluray1080p, "Bluray-1080p", SourceBluray, Resolution1080p, ModifierNone},
		{QualityWEBDL480p, "WEBDL-480p", SourceWebDL, Resolution480p, ModifierNone},
		{QualityHDTV1080p, "HDTV-1080p", SourceTV, Resolution1080p, ModifierNone},
		{QualityRawHD, "Raw-HD", SourceTV, Resolution1080p, ModifierRawHD},
		{QualityWEBRip480p, "WEBRip-480p", SourceWebRip, Resolution480p, ModifierNone},
		{QualityWEBRip720p, "WEBRip-720p", SourceWebRip, Resolution720p, ModifierNone},
		{QualityWEBRip1080p, "WEBRip-1080p", SourceWebRip, Resolution1080p, ModifierNone},
		{QualityHDTV2160p, "HDTV-2160p", SourceTV, Resolution2160p, ModifierNone},
		{QualityWEBRip2160p, "WEBRip-2160p", SourceWebRip, Resolution2160p, ModifierNone},
		{QualityWEBDL2160p, "WEBDL-2160p", SourceWebDL, Resolution2160p, ModifierNone},
		{QualityBluray2160p, "Bluray-2160p", SourceBluray, Resolution2160p, ModifierNone},
		{QualityBluray480p, "Bluray-480p", SourceBluray, Resolution480p, ModifierNone},
		{QualityBluray576p, "Bluray-576p", SourceBluray, Resolution576p, ModifierNone},
		{QualityBRDisk, "BR-DISK", SourceBluray, Resolution1080p, ModifierBRDisk},
		{QualityDVDR, "DVD-R", SourceDVD, Resolution480p, ModifierRemux},
		{QualityWorkprint, "WORKPRINT", SourceWorkprint, ResolutionUnknown, ModifierNone},
		{QualityCam, "CAM", SourceCam, ResolutionUnknown, ModifierNone},
		{QualityTelesync, "TELESYNC", SourceTelesync, ResolutionUnknown, ModifierNone},
		{QualityTelecine, "TELECINE", SourceTelecine, ResolutionUnknown, ModifierNone},
		{QualityDVDSCR, "DVDSCR", SourceDVD, Resolution480p, ModifierScreener},
		{QualityRegional, "REGIONAL", SourceDVD, Resolution480p, ModifierRegional},
		{QualityRemux1080p, "Remux-1080p", SourceBluray, Resolution1080p, ModifierRemux},
		{QualityRemux2160p, "Remux-2160p", SourceBluray, Resolution2160p, ModifierRemux},
	}
}

// QualityByID looks up a definition by Magpie (Radarr) quality ID.
func QualityByID(id QualityID) (QualityDefinition, bool) {
	for _, d := range Qualities() {
		if d.ID == id {
			return d, true
		}
	}
	return QualityDefinition{}, false
}

func mustQuality(id QualityID) QualityDefinition {
	d, ok := QualityByID(id)
	if !ok {
		panic("parse: unknown quality ID")
	}
	return d
}

// Sonarr IDs that mean a different quality in Radarr. Every other ID is shared.
const (
	sonarrBluray480p       = 13
	sonarrBluray1080pRemux = 20
	sonarrBluray2160pRemux = 21
	sonarrBluray576p       = 22
)

// SonarrToMagpieQuality maps a Sonarr quality ID to Magpie's (Radarr's) ID.
func SonarrToMagpieQuality(sonarrID int) QualityID {
	switch sonarrID {
	case sonarrBluray480p:
		return QualityBluray480p
	case sonarrBluray576p:
		return QualityBluray576p
	case sonarrBluray1080pRemux:
		return QualityRemux1080p
	case sonarrBluray2160pRemux:
		return QualityRemux2160p
	}
	return QualityID(sonarrID)
}
