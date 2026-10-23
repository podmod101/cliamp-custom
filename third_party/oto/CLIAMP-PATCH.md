Vendored from github.com/ebitengine/oto/v3 v3.4.1 (Apache-2.0, see LICENSE).
Local change: driver_unix.go `deviceCandidates` returns only the "pipewire" PCM when a PipeWire socket exists (or default/plug:default for PulseAudio), so cliamp never opens
raw ALSA hardware devices that PipeWire needs. Re-apply when bumping oto.
