"""CLAP audio embeddings from short independently sourced clips, on CPU."""
import hashlib
import json
import os
from pathlib import Path
import tempfile
import time
import numpy as np
from .common import CACHE, private_json, run, public_url

MODEL_ID = "laion/clap-htsat-unfused"
PROMPTS = [
    "psychedelic bass music with deep wobbling sub bass and spacious textures",
    "deep dubstep music with heavy sub bass and sparse half time drums",
    "glitch hop music with intricate broken beats and funky bass",
    "atmospheric future garage music with deep bass and haunting vocals",
    "melodic chillstep electronic music with soft vocals and heavy bass",
    "psydub downtempo music with organic instruments and psychedelic effects",
    "dark experimental bass music with distorted synths and heavy drums",
    "ambient electronic music with floating pads and very few drums",
    "liquid drum and bass music with fast breakbeats and warm melodies",
    "energetic drum and bass music with fast drums and aggressive bass",
    "house music with a steady four on the floor beat and warm chords",
    "techno music with repetitive driving kick drums and dark synths",
    "psytrance music with fast rolling bass and psychedelic synths",
    "dreamy indie pop music with emotional female vocals and slow drums",
    "alternative rock music with distorted guitars and emotional vocals",
    "hip hop music with rapping and heavy bass beats",
    "acoustic folk music with gentle guitar and singing",
    "classical orchestral music with strings and violin",
    "jazz music with improvisation and acoustic instruments",
    "reggae dub music with deep bass and echo effects",
    "soul and rhythm and blues music with warm vocals",
    "metal music with distorted guitars and fast heavy drums",
    "country music with guitar and singing",
    "uplifting trance music with soaring melodies and steady drums",
    "funk and disco music with groovy bass and rhythmic guitar",
    "world fusion electronic music with organic percussion and deep bass",
    "experimental electronic IDM music with complex rhythms and glitch textures",
    "pop music with catchy vocal melodies and electronic drums",
    "peaceful piano music with delicate melodies",
    "hard dubstep and riddim music with aggressive bass drops",
]
CONTENT_PROMPTS = ["instrumental music", "music with singing", "a person speaking without music",
                   "a radio advertisement with an announcer", "silence or static noise"]


def normalize(vector):
    value = np.asarray(vector, dtype=np.float32)
    norm = np.linalg.norm(value)
    if not np.isfinite(norm) or norm < 1e-8:
        raise ValueError("Invalid or silent audio embedding")
    return value / norm


def validate_text_embeddings(text):
    similarities = text @ text.T
    unrelated = similarities[np.triu_indices(len(text), k=1)]
    if not np.isfinite(text).all() or float(unrelated.mean()) > .95:
        raise RuntimeError("Audio checkpoint has collapsed text embeddings; refusing misleading matches")


class AudioModel:
    def __init__(self):
        import torch
        from transformers import ClapModel, ClapProcessor
        torch.set_num_threads(min(4, os.cpu_count() or 1))
        self.torch = torch
        # Download explicitly at setup; runtime remains offline for inference.
        self.processor = ClapProcessor.from_pretrained(MODEL_ID, local_files_only=True)
        self.model = ClapModel.from_pretrained(MODEL_ID, local_files_only=True).eval()
        with torch.inference_mode():
            inputs = self.processor(text=["The sound of " + s for s in PROMPTS], return_tensors="pt", padding=True)
            self.text = self.model.get_text_features(**inputs).numpy()
        self.text /= np.linalg.norm(self.text, axis=1, keepdims=True)
        validate_text_embeddings(self.text)
        with torch.inference_mode():
            inputs = self.processor(text=CONTENT_PROMPTS, return_tensors="pt", padding=True)
            self.content_text = self.model.get_text_features(**inputs).numpy()
        self.content_text /= np.linalg.norm(self.content_text, axis=1, keepdims=True)
        from .genres import GENRES
        with torch.inference_mode():
            inputs = self.processor(text=[p for _, prompts in GENRES.values() for p in prompts], return_tensors="pt", padding=True)
            self.genre_text = self.model.get_text_features(**inputs).numpy()
        self.genre_text /= np.linalg.norm(self.genre_text, axis=1, keepdims=True)

    def embed(self, path):
        return self.analyze(path)["vector"]

    def analyze(self, path):
        # Three independent 10s windows avoid representing only one drop or intro.
        pcm = run(["ffmpeg", "-nostdin", "-v", "error", "-i", str(path),
                   "-t", "30", "-ar", "48000", "-ac", "1", "-f", "f32le", "pipe:1"], binary=True)
        audio = np.frombuffer(pcm, dtype="<f4")
        if len(audio) < 48000 * 8 or not np.isfinite(audio).all() or np.max(np.abs(audio)) < 1e-5:
            raise ValueError("Audio sample is too short or silent")
        windows = [audio[i:i + 480000] for i in range(0, len(audio) - 383999, 480000)]
        embeddings = []
        for chunk in windows:
            inputs = self.processor(audio=chunk, sampling_rate=48000, return_tensors="pt")
            with self.torch.inference_mode():
                embeddings.append(normalize(self.model.get_audio_features(**inputs).numpy()[0]))
        vector = normalize(np.mean(embeddings, axis=0))
        content = np.asarray(embeddings) @ self.content_text.T
        margins = content[:, :2].max(axis=1) - content[:, 2:].max(axis=1)
        return {"vector": vector, "windows": embeddings, "seconds": len(audio) / 48000,
                "music_margin": float(np.median(margins)),
                "music_windows": int(np.sum(margins > 0)), "window_count": len(embeddings),
                "content": "possible speech/ad" if float(np.median(margins)) < 0 else "music-like",
                "silent_fraction": float(np.mean(np.abs(audio) < 1e-5))}

    def describe(self, embeddings, count=5):
        scores = np.asarray(embeddings) @ self.text.T
        order = np.argsort(scores.mean(axis=0))[::-1][:count]
        return [{"description": PROMPTS[i], "similarity": round(float(scores[:, i].mean()), 4)} for i in order]


def embedding_path(key):
    return CACHE / "embeddings" / (hashlib.sha256((MODEL_ID + key).encode()).hexdigest() + ".json")


def cached_source(key):
    try:
        return json.loads(embedding_path(key).read_text()).get("source")
    except (OSError, ValueError):
        return None


def cached_embedding(key, max_age):
    path = embedding_path(key)
    try:
        data = json.loads(path.read_text())
        if time.time() - data["at"] < max_age:
            vector = normalize(data["vector"])
            if vector.shape == (512,):
                return vector
    except (OSError, ValueError, KeyError):
        pass
    return None


def save_embedding(key, vector, source=None):
    folder = CACHE / "embeddings"
    path = embedding_path(key)
    private_json(path, {"at": time.time(), "vector": vector.tolist(), "source": source})
    # Bounded cache; no retained song recordings.
    files = sorted(folder.glob("*.json"), key=lambda p: p.stat().st_mtime)
    for old in files[:-1000]:
        old.unlink(missing_ok=True)


def sample_url(url, destination, start=0, seconds=30):
    if not public_url(url):
        raise ValueError("Invalid stream URL")
    args = ["ffmpeg", "-nostdin", "-v", "error", "-rw_timeout", "12000000"]
    if start:
        args += ["-ss", str(start)]
    run(args + ["-i", url, "-t", str(seconds), "-vn", "-ar", "48000", "-ac", "1", "-y", str(destination)], timeout=seconds+25)


def resolve_youtube(url):
    values = run(["yt-dlp", "--ignore-config", "--no-playlist", "--no-warnings",
                  "--socket-timeout", "12", "--retries", "1", "-f", "bestaudio/best",
                  "--get-url", "--", url], timeout=45).strip().splitlines()
    if not values or not public_url(values[0]):
        raise ValueError("YouTube audio URL unavailable")
    return values[0]
