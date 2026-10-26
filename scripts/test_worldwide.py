"""Offline world-search invariants; no catalogs, audio downloads or model weights."""
import json
from pathlib import Path
import random
import tempfile
import time
import unittest
from unittest.mock import patch
import numpy as np
from cliamp_vibe.world_catalog import RadioDirectory, deduplicate, live_youtube
from cliamp_vibe.world_reasoning import diverse_order, shortlist, search_plan
from cliamp_vibe.station_profiles import StationProfiles, measured_score, recommendation
from cliamp_vibe.world_policy import policy, should_stop, observe_progress
from cliamp_vibe.world_sampling import verify_candidates
from cliamp_vibe.world_sampling import combine_measurements


def station(i, **extra):
    return dict({"id": "c"+str(i), "name": "Station "+str(i), "url": f"https://stream{i}.example/audio",
                 "source": "Radio", "country": "", "language": "", "homepage": ""}, **extra)


def observation(vector=None, **extra):
    v = vector if vector is not None else np.eye(1, 512)[0]
    return dict({"vector": v, "windows": [v, v], "seconds": 20., "music_margin": .1,
                 "music_windows": 2, "window_count": 2, "content": "music-like", "silent_fraction": .01,
                 "at": int(time.time())}, **extra)


class WorldTests(unittest.TestCase):
    def test_growing_or_shrinking_result_lists_do_not_fake_diminishing_returns(self):
        history = []
        for count in (6, 8, 9, 10):
            observe_progress(history, [.7]*count)
        self.assertEqual(len(history), 1)
        self.assertFalse(should_stop(history, 10, 30, {"minimum_attempts":24}))
        for _ in range(3):
            observe_progress(history, [.7]*10)
        self.assertTrue(should_stop(history, 10, 48, {"minimum_attempts":24}))
        observe_progress(history, [.7]*9)
        self.assertEqual(history, [])

    def test_machine_timing_changes_capacity_and_deeper_allocates_more_effort(self):
        with tempfile.TemporaryDirectory() as folder, patch("cliamp_vibe.world_policy.CACHE", Path(folder)):
            (Path(folder)/"worldwide-tuning.json").write_text(json.dumps({"capture_workers": 6,
                "capture_p75_seconds": 12.47, "inference_seconds": .4, "sample_seconds": 20}))
            normal, deeper = policy(), policy(True)
        self.assertEqual(normal["workers"], 6)
        self.assertTrue(normal["calibrated"])
        self.assertGreater(deeper["candidate_limit"], normal["candidate_limit"])
        self.assertGreater(deeper["minimum_attempts"], normal["minimum_attempts"])
        self.assertLessEqual(normal["candidate_limit"], 64)

    def test_multiple_captures_increase_coverage_and_variable_audio_is_disclosed(self):
        first = observation()
        second = observation(vector=np.eye(2, 512)[1], seconds=30, windows=[np.eye(2, 512)[1]]*3,
                             window_count=3, music_windows=3)
        combined = combine_measurements(dict(first, captures=1), second)
        self.assertEqual(combined["seconds"], 50)
        self.assertEqual(len(combined["windows"]), 5)
        self.assertEqual(combined["captures"], 2)
        result = recommendation(station(1), combined, [np.eye(1, 512)[0]], "audio")
        self.assertIn("variable", result["uncertainty"])

    def test_metadata_never_changes_measured_similarity(self):
        refs = np.eye(2, 512)
        measured = observation()
        a = recommendation(station(1, prediction="perfect match", _predicted_fit=1), measured, refs, "audio")
        b = recommendation(station(1, prediction="unlikely", _predicted_fit=0), measured, refs, "audio")
        self.assertEqual(a["score"], b["score"])
        self.assertNotIn("_predicted_fit", a)
        self.assertIn("short excerpt", a["uncertainty"])
        self.assertLessEqual(a["reference_min"], a["reference_max"])

    def test_stale_cache_only_guides_search_and_schedule_changes_invalidate_freshness(self):
        with tempfile.TemporaryDirectory() as folder:
            profiles = StationProfiles(Path(folder)/"profiles.json")
            item = station(1, schedule="show A")
            record = profiles.save(item, observation())
            self.assertIsNotNone(profiles.fresh(item))
            self.assertIsNone(profiles.fresh(dict(item, schedule="show B")))
            self.assertIsNone(profiles.fresh(item, now=record["at"]+121))
            self.assertIsNotNone(profiles.prior(item, [np.eye(1, 512)[0]]))
            self.assertEqual((Path(folder)/"profiles.json").stat().st_mode & 0o777, 0o600)
            profiles.entries[next(iter(profiles.entries))]["windows"] = [[float("nan")]*512]
            self.assertIsNone(profiles.get(item))

    def test_world_diversity_includes_unknown_metadata_without_popularity_reward(self):
        items = [station(i, country="US", language="English", homepage="https://same.example/"+str(i), votes=99999) for i in range(10)]
        items += [station(10+i, country=c, language=l, votes=0) for i, (c, l) in enumerate(
                  [("Japan", "Japanese"), ("Brazil", "Portuguese"), ("France", "French"), ("", "")])]
        selected = diverse_order(items, lambda _: .5, 6, random.Random(1))
        self.assertGreaterEqual(len({s["country"] for s in selected}), 4)
        self.assertTrue(any(not s["country"] for s in selected))

    def test_live_youtube_does_not_hard_filter_poor_genre_labels(self):
        videos = [{"id": "abcdefghijk", "title": "夜の音", "live_status": "is_live"},
                  {"id": "lmnopqrstuv", "title": "old broadcast", "live_status": "was_live"}]
        with patch("cliamp_vibe.world_catalog.youtube_search", return_value=videos):
            self.assertEqual(len(live_youtube("bass")), 1)

    def test_station_mirrors_are_deduplicated_before_sampling(self):
        item = station(1, station_id="same", homepage="https://radio.example/")
        duplicate = dict(item, url="https://alternate.example/audio")
        self.assertEqual(len(deduplicate([item, duplicate, station(2)])), 2)

    def test_directory_uses_random_order_and_caches_metadata(self):
        with tempfile.TemporaryDirectory() as folder, patch("cliamp_vibe.world_catalog.CACHE", Path(folder)), patch(
                "cliamp_vibe.world_catalog.radio_servers", return_value=["directory.example"]), patch(
                "cliamp_vibe.world_catalog.request_json", return_value=[{"name": "Station", "url": "https://radio.example/audio"}]) as fetch:
            directory = RadioDirectory()
            first, _ = directory.search("tag", "bass")
            second, status = directory.search("tag", "bass")
            self.assertEqual(first, second)
            self.assertEqual(fetch.call_count, 1)
            self.assertIn("order=random", fetch.call_args.args[0])
            self.assertNotIn("votes", fetch.call_args.args[0])
            self.assertIn("cached", status)

    def test_ai_shortlist_uses_ids_only_and_keeps_exploration_candidates(self):
        items = [station(i, country=str(i % 8)) for i in range(100)]
        profiles = unittest.mock.Mock()
        profiles.prior.return_value = None
        plan = {"summary": "dreamy bass", "radio_tags": ["bass"], "youtube_queries": ["bass radio"]}
        answer = {"candidates": [{"id": "c0", "fit": 1, "reason": "metadata only"},
                                  {"id": "invented", "fit": 1, "reason": "fabricated"}]}
        with patch("cliamp_vibe.world_reasoning.local_json", return_value=(answer, "local")):
            selected, report = shortlist(items, plan, profiles, [np.eye(1, 512)[0]], 32)
        self.assertEqual(len(selected), 32)
        self.assertTrue(all(s["id"] != "invented" for s in selected))
        self.assertTrue(any("exploration" in s["prediction"] for s in selected))
        self.assertGreater(report["ai_considered"], 32)

    def test_ai_search_plan_receives_playlist_sound_and_never_supplies_urls(self):
        answer = {"radio_tags": ["dubstep"], "youtube_queries": ["bajos radio"], "station_terms": [], "summary": "deep"}
        with tempfile.TemporaryDirectory() as folder, patch("cliamp_vibe.world_reasoning.CACHE", Path(folder)), patch(
                "cliamp_vibe.world_reasoning.local_json", return_value=(answer, "local")) as model:
            result = search_plan([{"title": "Example", "artist": "Artist"}], [{"description": "bass"}], {}, [])
        self.assertIn("Example", model.call_args.args[0])
        self.assertIn("Spanish", model.call_args.args[0])
        self.assertEqual(result["model"], "local")

    def test_adaptive_stop_requires_coverage_and_diminishing_returns(self):
        settings = {"minimum_attempts": 24}
        self.assertFalse(should_stop([.2]*5, 9, 24, settings))
        self.assertFalse(should_stop([.2]*5, 10, 18, settings))
        self.assertFalse(should_stop([.2, .21, .22, .23], 10, 24, settings))
        self.assertTrue(should_stop([.2, .201, .201, .202], 10, 24, settings))

    def test_progressive_results_exclude_speech_and_do_not_use_historical_predictions(self):
        items = [station(i, prediction="great") for i in range(4)]
        model = unittest.mock.Mock()
        model.describe.return_value = [{"description": "measured music"}]
        model.analyze.side_effect = [observation(music_margin=-.1, music_windows=0)] + [observation() for _ in range(3)]
        profiles = unittest.mock.Mock()
        profiles.fresh.return_value = None
        published = []
        def capture(item, directory, index, seconds, deadline):
            path = directory/f"{index}.wav"
            path.touch()
            return path, .1, item["url"]
        settings = dict(workers=2, sample_seconds=20, confirm_seconds=30, audio_budget_seconds=120,
                        audio_seconds_limit=200, confirm_limit=0, minimum_attempts=24)
        with tempfile.TemporaryDirectory() as folder, patch("cliamp_vibe.world_sampling.capture_candidate", side_effect=capture):
            ranked, coverage = verify_candidates(items, [np.eye(1, 512)[0]], model, Path(folder), profiles, settings,
                                                 lambda matches, counts: published.append((matches, counts)))
            self.assertEqual(list(Path(folder).glob("*.wav")), [])
        self.assertEqual(len(ranked), 3)
        self.assertEqual(coverage["verified"], 3)
        self.assertEqual(coverage["audio_rejected"], 1)
        self.assertTrue(published)
        self.assertEqual(profiles.save.call_count, 3)
        self.assertTrue(all(s["sample_seconds"] == 20 for s in ranked))


if __name__ == "__main__":
    unittest.main()
