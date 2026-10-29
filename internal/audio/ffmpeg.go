package audio

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
)

var (
	ffmpegOnce  sync.Once
	ffmpegPath  string
	ffprobePath string
)

func lookupFFmpeg() {
	ffmpegOnce.Do(func() {
		ffmpegPath, _ = exec.LookPath("ffmpeg")
		ffprobePath, _ = exec.LookPath("ffprobe")
	})
}

// FFmpegAvailable reports whether ffmpeg was found in PATH.
func FFmpegAvailable() bool {
	lookupFFmpeg()
	return ffmpegPath != ""
}

type probeResult struct {
	Streams []struct {
		CodecName  string `json:"codec_name"`
		SampleRate string `json:"sample_rate"`
		Channels   int    `json:"channels"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

// openFFmpeg opens a file that is decoded by an ffmpeg child process emitting
// raw 16 bit stereo PCM at SampleRate.
func openFFmpeg(path string) (*Source, error) {
	src := &Source{
		Path:       path,
		Codec:      strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), ".")),
		SampleRate: int(SampleRate),
		Channels:   2,
		format:     beep.Format{SampleRate: SampleRate, NumChannels: 2, Precision: 2},
	}

	if ffprobePath != "" {
		out, err := exec.Command(ffprobePath, "-v", "error", "-select_streams", "a:0",
			"-show_entries", "stream=codec_name,sample_rate,channels:format=duration,bit_rate",
			"-of", "json", path).Output()
		if err != nil {
			return nil, fmt.Errorf("probe %s: %w", filepath.Base(path), err)
		}
		var pr probeResult
		if err := json.Unmarshal(out, &pr); err == nil {
			if len(pr.Streams) == 0 {
				return nil, fmt.Errorf("%s: no audio stream", filepath.Base(path))
			}
			st := pr.Streams[0]
			if st.CodecName != "" {
				src.Codec = strings.ToUpper(st.CodecName)
			}
			if sr, err := strconv.Atoi(st.SampleRate); err == nil {
				src.SampleRate = sr
			}
			if st.Channels > 0 {
				src.Channels = st.Channels
			}
			if d, err := strconv.ParseFloat(pr.Format.Duration, 64); err == nil {
				src.Length = time.Duration(d * float64(time.Second))
			}
			if br, err := strconv.Atoi(pr.Format.BitRate); err == nil {
				src.Bitrate = br / 1000
			}
		}
	}

	s := &ffmpegStream{path: path, length: SampleRate.N(src.Length)}
	if err := s.start(0); err != nil {
		return nil, err
	}
	src.stream = s
	return src, nil
}

type ffmpegStream struct {
	path   string
	length int // in frames, 0 when unknown
	pos    int
	cmd    *exec.Cmd
	out    io.ReadCloser
	r      *bufio.Reader
	buf    []byte
	err    error
}

func (s *ffmpegStream) start(at int) error {
	s.kill()
	args := []string{"-nostdin", "-v", "error"}
	if at > 0 {
		args = append(args, "-ss", strconv.FormatFloat(SampleRate.D(at).Seconds(), 'f', 3, 64))
	}
	args = append(args, "-i", s.path, "-vn", "-f", "s16le", "-acodec", "pcm_s16le",
		"-ac", "2", "-ar", strconv.Itoa(int(SampleRate)), "-")
	cmd := exec.Command(ffmpegPath, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	s.cmd, s.out, s.r = cmd, out, bufio.NewReaderSize(out, 1<<16)
	s.pos = at
	s.err = nil
	return nil
}

func (s *ffmpegStream) kill() {
	if s.cmd == nil {
		return
	}
	cmd := s.cmd
	s.cmd, s.out, s.r = nil, nil, nil
	_ = cmd.Process.Kill()
	go cmd.Wait() // reap without blocking the audio thread
}

func (s *ffmpegStream) Stream(samples [][2]float64) (n int, ok bool) {
	if s.r == nil {
		return 0, false
	}
	need := len(samples) * 4
	if cap(s.buf) < need {
		s.buf = make([]byte, need)
	}
	buf := s.buf[:need]
	got, err := io.ReadFull(s.r, buf)
	got -= got % 4
	for i := 0; i < got/4; i++ {
		l := int16(uint16(buf[i*4]) | uint16(buf[i*4+1])<<8)
		r := int16(uint16(buf[i*4+2]) | uint16(buf[i*4+3])<<8)
		samples[i][0] = float64(l) / 32768
		samples[i][1] = float64(r) / 32768
	}
	n = got / 4
	s.pos += n
	if err != nil {
		if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			s.err = err
		}
		s.kill()
		return n, n > 0
	}
	return n, true
}

func (s *ffmpegStream) Err() error { return s.err }

func (s *ffmpegStream) Len() int {
	if s.length > 0 {
		return s.length
	}
	return s.pos
}

func (s *ffmpegStream) Position() int { return s.pos }

func (s *ffmpegStream) Seek(p int) error {
	if p < 0 {
		p = 0
	}
	return s.start(p)
}

func (s *ffmpegStream) Close() error {
	s.kill()
	return nil
}
