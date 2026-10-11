// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"testing"

	"magpie/internal/domain"
)

func typicalReleases() []struct {
	name string
	kind domain.MediaKind
} {
	return []struct {
		name string
		kind domain.MediaKind
	}{
		{"Series.Title.S01E05.720p.HDTV.x264-GRP", domain.MediaKindSeries},
		{"Series.Title.S03E01E02.1080p.WEB-DL.DD5.1.H.264-NTb", domain.MediaKindSeries},
		{"Series.Title.S02.2160p.AMZN.WEB-DL.DDP5.1.HDR.HEVC-GRP", domain.MediaKindSeries},
		{"Movie.Title.2019.1080p.BluRay.x264-SPARKS", domain.MediaKindMovie},
		{"Movie.Title.2021.2160p.UHD.BluRay.REMUX.HDR.HEVC.Atmos-EPSiLON", domain.MediaKindMovie},
	}
}

func BenchmarkParseRelease(b *testing.B) {
	for _, c := range typicalReleases() {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := ParseRelease(c.name, c.kind); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
