package voicedialog

// 诊断辅助：把 QCloud TTS SDK 一手吐出的原始 PCM 直接写成 WAV 文件，方便用户用任意
// 播放器双击播放。这一层在 chunked channel / Pipeline framer / encoder / RTP 之前，
// 任何下游处理引入的伪音都不会污染此文件。用户对比这个文件和实际通话音频，
// 即可判定"电流音"是 QCloud 模型自身、还是我们的发送链路、还是接收端解码。
//
// 启用方式：环境变量 SIP_TTS_RAW_DUMP_DIR=/path/to/dir。每段一个文件：
//   <call_id>__<utterance_id>__<cloudSR>hz.wav

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type rawPCMDumper struct {
	f          *os.File
	path       string
	sampleRate int
	bytes      int64
}

func newRawPCMDumper(dir, callID, utteranceID string, sampleRate int) (*rawPCMDumper, error) {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	name := fmt.Sprintf("%s__%s__%dhz.wav", sanitizePathSeg(callID), sanitizePathSeg(utteranceID), sampleRate)
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	d := &rawPCMDumper{f: f, path: path, sampleRate: sampleRate}
	// Reserve space for 44-byte WAV header; sizes will be patched in Close().
	hdr := make([]byte, 44)
	if _, err := f.Write(hdr); err != nil {
		_ = f.Close()
		return nil, err
	}
	return d, nil
}

func (d *rawPCMDumper) Write(pcm []byte) error {
	if d == nil || d.f == nil || len(pcm) == 0 {
		return nil
	}
	n, err := d.f.Write(pcm)
	d.bytes += int64(n)
	return err
}

// Close patches the WAV header with the final data length and closes the file.
// The header layout is the standard 16-bit PCM mono WAVE format.
func (d *rawPCMDumper) Close() error {
	if d == nil || d.f == nil {
		return nil
	}
	defer func() {
		_ = d.f.Close()
		d.f = nil
	}()
	dataLen := uint32(d.bytes)
	riffLen := dataLen + 36
	hdr := make([]byte, 44)
	copy(hdr[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(hdr[4:8], riffLen)
	copy(hdr[8:12], []byte("WAVE"))
	copy(hdr[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(hdr[16:20], 16)                     // fmt chunk size
	binary.LittleEndian.PutUint16(hdr[20:22], 1)                      // PCM
	binary.LittleEndian.PutUint16(hdr[22:24], 1)                      // mono
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(d.sampleRate))   // sample rate
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(d.sampleRate*2)) // byte rate (mono*16bit)
	binary.LittleEndian.PutUint16(hdr[32:34], 2)                      // block align
	binary.LittleEndian.PutUint16(hdr[34:36], 16)                     // bits per sample
	copy(hdr[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(hdr[40:44], dataLen)
	if _, err := d.f.WriteAt(hdr, 0); err != nil {
		return err
	}
	return d.f.Sync()
}

func sanitizePathSeg(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	// Keep only filename-safe chars; replace others with '_'.
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '-', c == '_', c == '.':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	if len(b) == 0 {
		return "unknown"
	}
	if len(b) > 80 {
		b = b[:80]
	}
	return string(b)
}
