// Package ffprobe implements Library's MediaProber port with the ffprobe binary.
package ffprobe

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"magpie/internal/domain"
	"magpie/internal/library"
	"magpie/internal/library/api"
	"magpie/internal/platform/parse"
)

const (
	DefaultBinary = "ffprobe"

	// Like Sonarr: probe 50 MB, then 150 MB with a longer analysis when that wasn't enough.
	firstProbeSize   = "50000000"
	retryProbeSize   = "150000000"
	retryAnalyzeTime = "100000000" // microseconds

	codecTypeVideo    = "video"
	codecTypeAudio    = "audio"
	codecTypeSubtitle = "subtitle"

	transferPQ  = "smpte2084"
	transferHLG = "arib-std-b67"

	sideDataDolbyVision = "DOVI configuration record"
	sideDataHDR10Plus   = "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)"

	formatSeparator    = ","
	tenBitPixelMark    = "10"
	twelveBitPixelMark = "12"
	tenBits            = 10
	twelveBits         = 12
	bitsPerKilobit     = 1000
	bitrateTag         = "BPS"
)

const undetermined domain.Language = "und"

// imageSubtitleCodecs are bitmap subtitles; the rest are text.
var imageSubtitleCodecs = []string{"hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle", "xsub"}

var ErrNotInstalled = errors.New("ffprobe: not installed")

type Prober struct {
	binary string
}

var _ library.MediaProber = (*Prober)(nil)

func New() *Prober {
	return &Prober{binary: DefaultBinary}
}

// Probe reads a file's container, duration, bitrate, HDR format and streams. It fails with
// library.ErrProbeFailed when ffprobe can't read the file, and with ErrNotInstalled without ffprobe.
func (p *Prober) Probe(ctx context.Context, path string) (api.MediaInfo, error) {
	out, err := p.run(ctx, path, firstProbeSize)
	if err == nil && len(out.Streams) > 0 {
		return out.mediaInfo(), nil
	}
	if errors.Is(err, ErrNotInstalled) || ctx.Err() != nil {
		return api.MediaInfo{}, errors.Join(err, ctx.Err())
	}
	out, err = p.run(ctx, path, retryProbeSize, "-analyzeduration", retryAnalyzeTime)
	if err != nil {
		return api.MediaInfo{}, err
	}
	if len(out.Streams) == 0 {
		return api.MediaInfo{}, fmt.Errorf("%w: %s has no streams", library.ErrProbeFailed, path)
	}
	return out.mediaInfo(), nil
}

func (p *Prober) run(ctx context.Context, path, probeSize string, extra ...string) (probeOutput, error) {
	args := append([]string{"-v", "error", "-print_format", "json", "-show_format", "-show_streams", "-probesize", probeSize}, extra...)
	cmd := exec.CommandContext(ctx, p.binary, append(args, path)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return probeOutput{}, fmt.Errorf("%w: %w", ErrNotInstalled, err)
	}
	if err != nil {
		return probeOutput{}, fmt.Errorf("%w: %s: %w: %s", library.ErrProbeFailed, path, err, strings.TrimSpace(stderr.String()))
	}
	return decode(raw)
}

func decode(raw []byte) (probeOutput, error) {
	var out probeOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return probeOutput{}, fmt.Errorf("%w: decoding ffprobe output: %w", library.ErrProbeFailed, err)
	}
	return out, nil
}

type probeOutput struct {
	Format  probeFormat   `json:"format"`
	Streams []probeStream `json:"streams"`
}

type probeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`
}

type probeStream struct {
	Index            int               `json:"index"`
	CodecType        string            `json:"codec_type"`
	CodecName        string            `json:"codec_name"`
	Width            int               `json:"width"`
	Height           int               `json:"height"`
	PixFmt           string            `json:"pix_fmt"`
	BitsPerRawSample string            `json:"bits_per_raw_sample"`
	Channels         int               `json:"channels"`
	BitRate          string            `json:"bit_rate"`
	ColorTransfer    string            `json:"color_transfer"`
	StartTime        string            `json:"start_time"`
	Disposition      map[string]int    `json:"disposition"`
	Tags             map[string]string `json:"tags"`
	SideData         []struct {
		Type string `json:"side_data_type"`
	} `json:"side_data_list"`
}

func (o probeOutput) mediaInfo() api.MediaInfo {
	info := api.MediaInfo{
		Container:   strings.Split(o.Format.FormatName, formatSeparator)[0],
		Duration:    seconds(o.Format.Duration),
		BitrateKbps: kilobits(o.Format.BitRate),
		HDR:         api.HDRNone,
		Streams:     []api.Stream{},
	}
	for _, s := range o.Streams {
		kind, ok := streamKind(s.CodecType)
		if !ok {
			continue
		}
		stream := api.Stream{
			Index: s.Index, Kind: kind, Codec: s.CodecName, Language: language(s.Tags),
			Forced: s.Disposition["forced"] > 0, Default: s.Disposition["default"] > 0,
			Image:    kind == api.StreamSubtitle && slices.Contains(imageSubtitleCodecs, s.CodecName),
			Channels: s.Channels, BitDepth: s.bitDepth(), BitrateKbps: kilobits(cmp.Or(s.BitRate, s.Tags[bitrateTag])),
			StartTime: seconds(s.StartTime),
		}
		info.Streams = append(info.Streams, stream)
		if kind == api.StreamVideo && info.Width == 0 {
			info.Width, info.Height, info.HDR = s.Width, s.Height, s.hdr()
		}
	}
	return info
}

func streamKind(codecType string) (api.StreamKind, bool) {
	switch codecType {
	case codecTypeVideo:
		return api.StreamVideo, true
	case codecTypeAudio:
		return api.StreamAudio, true
	case codecTypeSubtitle:
		return api.StreamSubtitle, true
	}
	return "", false
}

func (s probeStream) hdr() api.HDRFormat {
	for _, side := range s.SideData {
		switch side.Type {
		case sideDataDolbyVision:
			return api.HDRDolbyVision
		case sideDataHDR10Plus:
			return api.HDR10Plus
		}
	}
	switch s.ColorTransfer {
	case transferPQ:
		return api.HDR10
	case transferHLG:
		return api.HDRHLG
	}
	return api.HDRNone
}

func (s probeStream) bitDepth() int {
	if bits, err := strconv.Atoi(s.BitsPerRawSample); err == nil && bits > 0 {
		return bits
	}
	switch {
	case strings.Contains(s.PixFmt, tenBitPixelMark):
		return tenBits
	case strings.Contains(s.PixFmt, twelveBitPixelMark):
		return twelveBits
	}
	return 0
}

func language(tags map[string]string) domain.Language {
	if code, ok := parse.LanguageCode(tags["language"]); ok {
		return code
	}
	return undetermined
}

func seconds(s string) time.Duration {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return time.Duration(f * float64(time.Second))
}

func kilobits(s string) int {
	bits, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return bits / bitsPerKilobit
}
