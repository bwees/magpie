//go:build lintfixture

// Package transcodefixture must fail lint: Transcode never imports Subtitles' api.
// `mise run lint:boundaries` checks that depguard rejects it.
package transcodefixture

import subtitlesapi "magpie/internal/subtitles/api"

var _ = subtitlesapi.Messages
