package encode

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// ffprobe codec_type values.
const (
	TypeVideo      = "video"
	TypeAudio      = "audio"
	TypeSubtitle   = "subtitle"
	TypeAttachment = "attachment"
)

const (
	eightBit  = 8
	tenBit    = 10
	twelveBit = 12

	tenBitMark    = "10"
	twelveBitMark = "12"

	transferPQ = "smpte2084"
	// sideDataMastering is how ffprobe names HDR10's static mastering metadata on a frame.
	sideDataMastering = "Mastering display metadata"
)

// Stream is one stream as ffprobe reports it.
type Stream struct {
	Index          int    `json:"index"`
	Type           string `json:"codec_type"`
	Codec          string `json:"codec_name"`
	PixFmt         string `json:"pix_fmt"`
	BitsPerSample  string `json:"bits_per_raw_sample"`
	FrameRate      string `json:"r_frame_rate"`
	AvgFrameRate   string `json:"avg_frame_rate"`
	ColorTransfer  string `json:"color_transfer"`
	FramesInStream string `json:"nb_read_packets"`
	Disposition    struct {
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
}

// BitDepth reads the depth from the pixel format, falling back to bits_per_raw_sample.
func (s Stream) BitDepth() int {
	switch {
	case strings.Contains(s.PixFmt, twelveBitMark):
		return twelveBit
	case strings.Contains(s.PixFmt, tenBitMark):
		return tenBit
	}
	if bits, err := strconv.Atoi(s.BitsPerSample); err == nil && bits > 0 {
		return bits
	}
	return eightBit
}

// ConstantRate reports whether the stream's frame rate is constant, so frame counts compare.
func (s Stream) ConstantRate() bool {
	return s.FrameRate != "" && s.FrameRate == s.AvgFrameRate
}

// Media is what ffprobe reports for a file.
type Media struct {
	Duration float64
	Streams  []Stream
}

// Video is the first video stream that isn't cover art.
func (m Media) Video() (Stream, bool) {
	for _, s := range m.Streams {
		if s.Type == TypeVideo && s.Disposition.AttachedPic == 0 {
			return s, true
		}
	}
	return Stream{}, false
}

// Stream returns the stream with a source index.
func (m Media) Stream(index int) (Stream, bool) {
	for _, s := range m.Streams {
		if s.Index == index {
			return s, true
		}
	}
	return Stream{}, false
}

// HDR10 reports whether the video is PQ, the transfer HDR10 uses.
func (m Media) HDR10() bool {
	video, ok := m.Video()
	return ok && video.ColorTransfer == transferPQ
}

// Input is a source ffmpeg reads: a local path, or the server URL with the options that authenticate
// and reconnect it.
type Input struct {
	Path string
	Args []string
}

// reconnectDelayMax caps ffmpeg's wait between reconnects of an HTTP input, in seconds.
const reconnectDelayMax = "30"

// HTTPInput reads a URL as direct read needs: with the agent's auth headers, reconnecting at the byte
// offset after a network error.
func HTTPInput(url string, headers map[string]string) Input {
	var lines strings.Builder
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		lines.WriteString(name + ": " + headers[name] + "\r\n")
	}
	return Input{Path: url, Args: []string{
		"-headers", lines.String(), "-reconnect", "1", "-reconnect_on_network_error", "1",
		"-reconnect_delay_max", reconnectDelayMax,
	}}
}

func (in Input) args() []string {
	return append(append([]string{}, in.Args...), "-i", in.Path)
}

type probeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []Stream `json:"streams"`
}

// Probe reads an input's duration and streams.
func (e *Encoder) Probe(ctx context.Context, in Input) (Media, error) {
	args := append([]string{"-v", "error", "-print_format", "json", "-show_format", "-show_streams"}, in.Args...)
	stdout, stderr, err := e.Runner.Run(ctx, Command{Name: e.FFprobe, Args: append(args, in.Path)})
	if err != nil {
		return Media{}, fmt.Errorf("ffprobe %s: %w: %s", in.Path, err, firstLine(stderr))
	}
	var out probeOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		return Media{}, fmt.Errorf("decoding ffprobe output: %w", err)
	}
	duration, err := strconv.ParseFloat(out.Format.Duration, 64)
	if err != nil {
		return Media{}, fmt.Errorf("ffprobe %s: no duration", in.Path)
	}
	return Media{Duration: duration, Streams: out.Streams}, nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
