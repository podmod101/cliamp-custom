"""Offline search/replenishment regressions. No network or model downloads."""
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from cliamp_vibe.common import CommandError, command_budget, command_error_kind, run
from cliamp_vibe.local_model import plan_song_queries, parse_model_json
from cliamp_vibe.song_lookup import song_match, song_videos
from cliamp_vibe.references import collect_references
from cliamp_vibe.engine import prepare_song


class SongSearchTests(unittest.TestCase):
    def test_model_json_handles_omitted_outer_brace_but_rejects_invented_values(self):
        self.assertEqual(parse_model_json('{"songs":[{"id":"0","queries":["a","b"]}]'),
                         {"songs": [{"id": "0", "queries": ["a", "b"]}]})
        self.assertEqual(parse_model_json('```json\n{"text":"brace } in string"}\n```'),
                         {"text": "brace } in string"})
        for invalid in ('{"songs":["truncated', '{"songs":[}', '{"songs":', '{"songs":["a",'):
            with self.assertRaises((ValueError, json.JSONDecodeError)):
                parse_model_json(invalid)

    def test_local_ai_plans_are_mapped_by_id_and_feedback_is_data(self):
        songs = [{"title": "Title - Guest Remix", "artist": "Main, Guest, Other"},
                 {"title": "H U M A N", "artist": "Example"}]
        answer = {"songs": [{"id": "1", "queries": ["Example HUMAN", "H U M A N Example"]},
                            {"id": "0", "queries": ["Main Title Guest Remix", "Guest Title remix"]}]}
        with patch("cliamp_vibe.local_model.local_json", return_value=(answer, "local")) as call:
            plans, model = plan_song_queries(songs, [{"id": "0", "results": ["wrong song"]}])
        self.assertEqual(plans[0][0], "Main Title Guest Remix")
        self.assertEqual(model, "local")
        self.assertIn("wrong song", call.call_args.args[0])
        with patch("cliamp_vibe.local_model.local_json", return_value=({"songs": []}, "local")):
            with self.assertRaisesRegex(RuntimeError, "every sampled song"):
                plan_song_queries(songs)

    def test_name_normalization_preserves_recording_identity(self):
        cases = [("H U M A N", "Human"), ("Café - Original Mix", "Cafe"), ("Desire <3", "Desire ᐸ3"),
                 ("Places - Guest Remix", "Places (Guest Remix)")]
        for original, upload in cases:
            song = {"title": original, "artist": "Artist", "duration": 180}
            self.assertTrue(song_match(song, {"title": "Artist - " + upload, "duration": 182}))
        song = {"title": "Places - Guest Remix", "artist": "Artist, Guest", "duration": 180}
        for title in ("Artist - Places", "Artist - Places (Different Remix)",
                      "Artist - Places (Guest Remix) slowed", "Other - Places (Guest Remix)"):
            self.assertFalse(song_match(song, {"title": title, "duration": 180}))
        self.assertFalse(song_match({"title": "Places", "artist": "Artist"},
                                   {"title": "Artist - Places Remix", "duration": 180}))

    def test_search_tries_ai_alternative_and_keeps_failed_results_for_feedback(self):
        song = {"title": "Places", "artist": "Artist, Other", "duration": 180,
                "_queries": ["long query", "Artist Places"]}
        wrong = {"id": "abcdefghijk", "title": "Artist Other Song", "duration": 180}
        right = {"id": "lmnopqrstuv", "title": "Artist Places", "duration": 180}
        with patch("cliamp_vibe.discovery.youtube_search", side_effect=[[wrong], [right]]) as search:
            self.assertEqual(song_videos(song)[0]["url"], "https://www.youtube.com/watch?v=lmnopqrstuv")
        self.assertEqual(search.call_count, 2)
        self.assertEqual(len(song["_lookup_feedback"]), 2)

    def test_capture_tries_another_verified_upload_and_ignores_wrong_cached_identity(self):
        song = {"title": "Places", "artist": "Artist", "duration": 180}
        matches = [{"url": "https://www.youtube.com/watch?v=abcdefghijk", "duration": 180},
                   {"url": "https://www.youtube.com/watch?v=lmnopqrstuv", "duration": 180}]
        with tempfile.TemporaryDirectory() as folder, patch("cliamp_vibe.engine.cached_embedding", return_value=[1]), patch(
                "cliamp_vibe.engine.cached_source", return_value={"title": "Wrong Song", "duration": 180}), patch(
                "cliamp_vibe.engine.song_videos", return_value=matches), patch(
                "cliamp_vibe.engine.resolve_youtube", side_effect=[CommandError("yt-dlp", "unavailable"),
                    "https://media.example/audio"]), patch("cliamp_vibe.engine.sample_url") as capture:
            key, vector, path = prepare_song(song, Path(folder), 0)
        self.assertIsNone(vector)
        self.assertEqual(song["_source"], matches[1])
        self.assertEqual(capture.call_count, 1)

    def test_replaces_failures_and_repairs_queries_from_actual_results(self):
        songs = [{"title": str(i), "artist": "Artist"} for i in range(24)]
        calls = []

        def planner(batch, feedback, **kwargs):
            calls.append(feedback)
            return [["first", "second"] for _ in batch], "local"

        def collector(batch, *args):
            found = []
            for song in batch:
                if int(song["title"]) < 10:
                    song["_failure_kind"] = "no_confident_match"
                    song["_lookup_feedback"] = [{"results": [{"title": "unrelated"}]}]
                else:
                    found.append((song, [1., 0.]))
            return found, len(batch) - len(found)

        with tempfile.TemporaryDirectory() as folder, patch("cliamp_vibe.references.CACHE", Path(folder)), patch(
                "cliamp_vibe.references.plan_song_queries", side_effect=planner):
            refs, summary = collect_references(songs, None, None, Path(folder), collector)
            diagnostic = json.loads((Path(folder) / "latest-reference-search.json").read_text())
        self.assertGreaterEqual(len(refs), 12)
        self.assertGreater(summary["attempted"], 12)
        self.assertTrue(summary["feedback_retry"])
        self.assertTrue(any(calls))
        self.assertEqual(diagnostic["usable"], len(refs))
        self.assertEqual(len({s["title"] for s, _ in refs}), len(refs))

    def test_shared_auth_failure_stops_without_guessing_or_more_requests(self):
        songs = [{"title": str(i), "artist": "Artist"} for i in range(36)]

        def collector(batch, *args):
            for song in batch:
                song["_failure_kind"] = "youtube_auth_required"
            return [], len(batch)

        with tempfile.TemporaryDirectory() as folder, patch("cliamp_vibe.references.CACHE", Path(folder)), patch(
                "cliamp_vibe.references.plan_song_queries", return_value=([["a", "b"]] * 12, "local")) as planner:
            with self.assertRaisesRegex(RuntimeError, "youtube auth required: 12"):
                collect_references(songs, None, None, Path(folder), collector)
            self.assertEqual(planner.call_count, 1)

    def test_subprocess_errors_are_categorized_without_exposing_urls(self):
        self.assertEqual(command_error_kind("Sign in to confirm you're not a bot"), "youtube_auth_required")
        self.assertEqual(command_error_kind("HTTP Error 429"), "rate_limited")
        self.assertEqual(command_error_kind("Requested format is not available"), "format_unavailable")
        process = unittest.mock.Mock(returncode=1)
        process.communicate.return_value = (b"", b"HTTP Error 403 https://signed.example/?token=secret")
        with patch("cliamp_vibe.common.subprocess.Popen", return_value=process):
            with self.assertRaises(CommandError) as raised:
                run(["yt-dlp"])
        self.assertEqual(raised.exception.kind, "access_denied")
        self.assertNotIn("secret", str(raised.exception))
        with command_budget(0), patch("cliamp_vibe.common.subprocess.Popen") as start:
            with self.assertRaisesRegex(CommandError, "timeout"):
                run(["yt-dlp"])
            start.assert_not_called()


if __name__ == "__main__":
    unittest.main()
