//go:build nowhispercpp

package whisper

import (
	"context"
	"fmt"
)

// WhisperCPPConfig controls the on-device whisper.cpp backend.
type WhisperCPPConfig struct {
	ModelPath      string
	Threads        int
	DefaultOptions Options
}

// WhisperCPPTranscriber wraps a whisper.cpp model for local transcription.
type WhisperCPPTranscriber struct {
	defaultOptions Options
	defaultThreads int
}

// NewWhisperCPPTranscriber loads a whisper.cpp model for on-device speech recognition.
func NewWhisperCPPTranscriber(cfg WhisperCPPConfig) (*WhisperCPPTranscriber, error) {
	return nil, fmt.Errorf("whisper.cpp support is disabled in this build")
}

// Close releases resources held by the whisper.cpp model.
func (t *WhisperCPPTranscriber) Close() {
	// No-op
}

// TranscribePCM runs whisper.cpp against raw PCM samples.
func (t *WhisperCPPTranscriber) TranscribePCM(ctx context.Context, samples []float32, sampleRate int, opts Options) (*Result, error) {
	return nil, fmt.Errorf("whisper.cpp support is disabled in this build")
}

// TranscribeSilk decodes SILK payloads before invoking whisper.cpp.
func (t *WhisperCPPTranscriber) TranscribeSilk(ctx context.Context, silkData []byte, opts Options) (*Result, error) {
	return nil, fmt.Errorf("whisper.cpp support is disabled in this build")
}
