Vendored from github.com/ebitengine/oto/v3 v3.4.1 (Apache-2.0, see LICENSE).
Local changes (marked `CLIAMP LOCAL PATCH`), re-apply when bumping oto:
- driver_unix.go `deviceCandidates` returns only the "pipewire" PCM when a PipeWire socket exists (or default/plug:default for PulseAudio), so cliamp never opens
  raw ALSA hardware devices that PipeWire needs.
- driver_unix.go `newContext` uses at least a 250 ms ALSA buffer when a sound server socket exists; smaller buffers fail with EIO when PipeWire's quantum is large.
- context.go adds `CreatedContextErr`, so cliamp can detect a failed driver through beep's speaker.
