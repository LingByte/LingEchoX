package session

// AudioSource represents different audio input sources
type AudioSource int

const (
	AudioSourceFile AudioSource = iota
	AudioSourceMicrophone
	AudioSourceMixed
)

// SetAudioSource sets the audio input source
func (c *Client) SetAudioSource(source AudioSource, audioFile string) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.audioSource = source
	if audioFile != "" {
		c.audioFile = audioFile
	}
}
