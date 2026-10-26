"""Offline behavioral tests; no downloads, model weights, or network calls."""
import json
from pathlib import Path
import random
import tempfile
import unittest
from unittest.mock import patch
import numpy as np
from cliamp_vibe.engine import sample_songs, rank_streams
from cliamp_vibe.discovery import song_match, discover, youtube_candidates, candidate_relevant
from cliamp_vibe.common import public_url
from cliamp_vibe.local_model import discovery_queries, local_backend
from cliamp_vibe.audio import validate_text_embeddings
from cliamp_vibe.preferences import playlist_hint


class VibeTests(unittest.TestCase):
    def test_private_taste_hint_is_scoped_to_the_selected_playlist(self):
        with tempfile.TemporaryDirectory() as folder:
            (Path(folder) / "vibe-preferences.json").write_text(json.dumps({"chosen": {"description": "dreamy bass"}}))
            with patch.dict("os.environ", {"CLIAMP_CONFIG_DIR": folder}):
                self.assertEqual(playlist_hint({"id": "chosen"})["description"], "dreamy bass")
                self.assertEqual(playlist_hint({"id": "other"}), {})

    def test_rejects_collapsed_checkpoint_instead_of_returning_misleading_scores(self):
        validate_text_embeddings(np.eye(5))
        with self.assertRaisesRegex(RuntimeError, "collapsed"):
            validate_text_embeddings(np.ones((5, 8)) / np.sqrt(8))

    def test_random_sample_covers_whole_playlist_and_deduplicates(self):
        tracks = [{"title": str(i), "artist": "artist"} for i in range(100)]
        sample = sample_songs(tracks + tracks, rng=random.Random(7))
        self.assertEqual(len(sample), 12)
        self.assertEqual(len({song["title"] for song in sample}), 12)
        self.assertTrue(any(int(song["title"]) > 50 for song in sample))
        self.assertNotEqual(sample, sample_songs(tracks, rng=random.Random(8)))

    def test_audio_ranking_is_independent_of_artist_or_candidate_name(self):
        refs = np.asarray([[1., 0.], [.8, .6]])
        candidates = [({"name": "same artist", "url": "b"}, np.asarray([0., 1.])),
                      ({"name": "unrelated artist", "url": "a"}, np.asarray([1., 0.]))]
        self.assertEqual(rank_streams(refs, candidates)[0]["url"], "a")
        self.assertEqual(len(rank_streams(refs, candidates * 9)), 10)
        self.assertEqual(rank_streams(refs, []), [])

    def test_song_matching_rejects_ambiguous_or_wrong_audio(self):
        song = {"title": "Nightshift", "artist": "Lab Group", "duration": 180}
        good = {"title": "Lab Group - Nightshift (Official Audio)", "duration": 184}
        self.assertTrue(song_match(song, good))
        for bad in ({**good, "duration": 800}, {**good, "title": "Somebody - Nightshift"},
                    {**good, "title": "Lab Group - Nightshift (karaoke cover)"},
                    {**good, "live_status": "is_live"}):
            self.assertFalse(song_match(song, bad))

    def test_public_url_rejects_local_and_executable_inputs(self):
        for value in ("file:///etc/passwd", "http://127.0.0.1/", "http://[::1]/", "http://localhost/", "-bad", "http://user:pw@site.com"):
            self.assertFalse(public_url(value))
        self.assertTrue(public_url("https://ice2.somafm.com/dubstep-128-mp3"))

    def test_youtube_requires_live_flag_and_valid_video_id(self):
        videos = [{"id": "abcdefghijk", "title": "station", "live_status": "is_live"},
                  {"id": "lmnopqrstuv", "title": "old station", "live_status": "was_live"},
                  {"id": "invalid", "title": "bad", "live_status": "is_live"}]
        with patch("cliamp_vibe.discovery.youtube_search", return_value=videos), patch(
                "cliamp_vibe.discovery.candidate_relevant", return_value=True):
            self.assertEqual(len(youtube_candidates("bass")), 1)

    def test_candidate_requires_genre_evidence_beyond_search_ranking(self):
        self.assertFalse(candidate_relevant("psychedelic bass dubstep", {"title": "Night Drive Deep House Radio"}))
        self.assertTrue(candidate_relevant("psychedelic bass dubstep", {"title": "Psychedelic Dub and Deep Bass Radio"}))
        self.assertTrue(candidate_relevant("melodic bass chillstep", {"title": "Chillstep Live Radio"}))

    def test_broadcaster_catalog_does_not_pad_with_unrelated_stations(self):
        from cliamp_vibe.discovery import soma_candidates
        channels = [{"id": "dub", "title": "Dub station", "description": "deep bass dubstep", "playlists": [
            {"format": "mp3", "url": "https://radio.example/dub.pls"}]},
            {"id": "christmas", "title": "Christmas Lounge", "description": "holiday classics", "playlists": [
                {"format": "mp3", "url": "https://radio.example/christmas.pls"}]}]
        with patch("cliamp_vibe.discovery.request_json", return_value={"channels": channels}), patch(
                "cliamp_vibe.discovery.request_text", return_value="File1=https://stream.example/dub\n"):
            self.assertEqual([x["name"] for x in soma_candidates("dubstep bass")], ["SomaFM: Dub station"])

    def test_discovery_deduplicates_station_mirrors(self):
        first = {"name": "Station", "url": "https://stream.example/a", "station_id": "one"}
        second = {"name": "Station", "url": "https://stream.example/b", "station_id": "two"}
        with patch("cliamp_vibe.discovery.radio_candidates", return_value=[first, second]), patch(
                "cliamp_vibe.discovery.youtube_candidates", return_value=[]), patch(
                "cliamp_vibe.discovery.soma_candidates", return_value=[]):
            results, warnings = discover({"radio_tags": ["dub", "bass"], "youtube_queries": ["bass"]})
        self.assertEqual(len(results), 1)
        self.assertEqual(warnings, [])

    def test_text_run_is_local_tools_disabled_and_has_no_cloud_fallback(self):
        provider = {"baseUrl": "http://127.0.0.1:8000/v1", "models": [{"id": "local"}]}

        def fake_run(args, **kwargs):
            config = json.loads(Path(args[args.index("--config") + 1]).read_text())
            self.assertEqual(config["tools"]["deny"], ["*"])
            self.assertEqual(config["agents"]["defaults"]["model"]["fallbacks"], [])
            self.assertEqual(list(config["models"]["providers"]), ["llama-cpp"])
            self.assertIn("OPENCLAW_STATE_DIR", kwargs["env"])
            return json.dumps({"ok": True, "provider": "llama-cpp", "final": json.dumps({
                "radio_tags": ["dubstep"], "youtube_queries": ["bass music"], "summary": "deep bass"})})

        with patch("cliamp_vibe.local_model.local_backend", return_value=("llama-cpp", provider, "local")), patch(
                "cliamp_vibe.local_model.run", side_effect=fake_run):
            self.assertEqual(discovery_queries([])["model"], "local")

    def test_cloud_only_configuration_fails_instead_of_falling_back(self):
        with tempfile.TemporaryDirectory() as folder:
            config = Path(folder) / "config.json"
            config.write_text(json.dumps({"models": {"providers": {"cloud": {
                "baseUrl": "https://cloud.example/v1", "models": [{"id": "remote"}]}}}}))
            with patch.dict("os.environ", {"OPENCLAW_CONFIG_PATH": str(config)}):
                with self.assertRaisesRegex(RuntimeError, "No local"):
                    local_backend()


if __name__ == "__main__":
    unittest.main()
