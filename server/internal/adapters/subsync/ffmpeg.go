package subsync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

const DefaultFFmpegBinary = "ffmpeg"

// FFmpeg decodes audio by running the ffmpeg binary. The zero value runs ffmpeg from PATH.
type FFmpeg struct {
	Binary string
}

func (f FFmpeg) DecodeAudio(ctx context.Context, videoPath string) (io.ReadCloser, error) {
	binary := f.Binary
	if binary == "" {
		binary = DefaultFFmpegBinary
	}
	cmd := exec.CommandContext(ctx, binary, ffmpegArgs(videoPath)...)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", binary, err)
	}
	return &ffmpegStream{ReadCloser: stdout, cmd: cmd, stderr: stderr}, nil
}

func ffmpegArgs(videoPath string) []string {
	return []string{
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-i", videoPath,
		"-map", "0:a:0",
		"-ac", "1",
		"-ar", strconv.Itoa(SampleRate),
		"-f", "s16le", "-acodec", "pcm_s16le",
		"pipe:1",
	}
}

type ffmpegStream struct {
	io.ReadCloser
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func (s *ffmpegStream) Close() error {
	_ = s.ReadCloser.Close()
	if err := s.cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(s.stderr.String()))
	}
	return nil
}
