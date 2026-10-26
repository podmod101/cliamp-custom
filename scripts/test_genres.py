"""Offline regressions for intent, genre eligibility and honest diverse ranking."""
from pathlib import Path
import random
import tempfile
import unittest
from unittest.mock import patch
import numpy as np
from cliamp_vibe.genres import GENRES, description_genres, genre_compatible, diverse_matches, audio_genres
from cliamp_vibe.engine import reference_songs
from cliamp_vibe.world_reasoning import search_plan
from cliamp_vibe.world_sampling import verify_candidates


class GenreTests(unittest.TestCase):
    def test_sampler_withholds_a_perfect_cosine_country_result_under_edm_filter(self):
        count = sum(len(p) for _, p in GENRES.values())
        genre_text = np.zeros((count, 512))
        genre_text[:, :count] = np.eye(count)
        country_index = sum(len(p) for key, (_, p) in GENRES.items() if list(GENRES).index(key) < list(GENRES).index("country"))
        country, edm = np.eye(512)[country_index], np.eye(512)[0]
        model = unittest.mock.Mock()
        model.genre_text = genre_text
        model.describe.return_value = [{"description": "measured audio"}]
        def analyze(path):
            vector = country if path.stem == "country" else edm
            return {"vector":vector,"windows":[vector,vector],"seconds":20.,"music_margin":.2,
                    "music_windows":2,"window_count":2,"content":"music-like","silent_fraction":0}
        model.analyze.side_effect = analyze
        profiles = unittest.mock.Mock()
        profiles.fresh.return_value = None
        candidates=[{"id":key,"name":key,"source":"Radio","url":"https://"+key+".example/live"} for key in ("country","edm")]
        def capture(item, folder, *args):
            path=folder/(item["id"]+".wav")
            path.touch()
            return path,.1,item["url"]
        settings={"workers":2,"sample_seconds":20,"confirm_seconds":30,"audio_budget_seconds":120,
                  "audio_seconds_limit":100,"minimum_attempts":24,"confirm_limit":0,"allowed_genres":["edm"]}
        with tempfile.TemporaryDirectory() as folder, patch("cliamp_vibe.world_sampling.capture_candidate",side_effect=capture):
            results, coverage=verify_candidates(candidates,[country],model,Path(folder),profiles,settings,lambda *args:None)
        self.assertEqual([r["id"] for r in results],["edm"])
        self.assertEqual(results[0]["score"],0.)
        self.assertEqual(coverage["genre_rejected"],1)

    def test_curator_description_preserves_electronic_glitch_blues_directions(self):
        description = "Delta, Chicago Blues & Field Hollers Meet Glitch Hop, Trap, Dubstep & Electronic."
        self.assertEqual(description_genres(description), ["edm", "glitch", "blues"])
        answer = {"radio_tags": ["indie pop"], "youtube_queries": ["pop radio"],
                  "genres": ["pop", "country"], "summary": "shared sound and atmosphere"}
        with tempfile.TemporaryDirectory() as folder, patch("cliamp_vibe.world_reasoning.CACHE", Path(folder)), patch(
                "cliamp_vibe.world_reasoning.local_json", return_value=(answer, "local")) as model:
            result = search_plan([], [], {"playlist_description": description, "selected_genre": "edm"}, [])
        self.assertEqual(result["genres"], ["edm", "glitch", "blues"])
        self.assertEqual(result["radio_tags"][0], "electronic")
        self.assertNotIn("shared sound and atmosphere", result["summary"])
        self.assertIn(description, model.call_args.args[0])

    def test_high_general_cosine_cannot_override_wrong_measured_genre(self):
        country = {"country": .56, "edm": .40, "blues": .42, "glitch": .38}
        electronic = {"country": .32, "edm": .55, "blues": .42, "glitch": .54}
        self.assertFalse(genre_compatible(country, ["edm"]))
        self.assertFalse(genre_compatible(country, ["edm", "glitch", "blues"]))
        self.assertTrue(genre_compatible(electronic, ["edm"]))
        self.assertTrue(genre_compatible(country, ["country"]))
        self.assertTrue(genre_compatible({"edm": .50, "hiphop": .52}, ["edm"]))

    def test_broad_edm_accepts_a_matching_subgenre_without_requiring_every_subgenre(self):
        length = sum(len(prompts) for _, prompts in GENRES.values())
        model = unittest.mock.Mock()
        model.genre_text = np.eye(length)
        v = np.zeros(length)
        v[1] = 1  # DnB caption in the broad EDM family.
        result = audio_genres([v], model)
        self.assertEqual(result["edm"], 1)
        self.assertTrue(genre_compatible(result, ["edm"]))

    def test_variety_selection_preserves_raw_scores(self):
        scored = [{"id": "a", "score": .80}, {"id": "b", "score": .79}, {"id": "c", "score": .77}]
        vectors = {"a": np.asarray([1., 0.]), "b": np.asarray([1., 0.]), "c": np.asarray([0., 1.])}
        result = diverse_matches(scored, vectors)
        self.assertEqual([r["id"] for r in result], ["a", "c", "b"])
        self.assertEqual({r["id"]: r["score"] for r in result}, {"a": .80, "b": .79, "c": .77})

    def test_large_playlist_references_include_curated_opening_and_later_pages(self):
        tracks = [{"title": str(i), "artist": "Artist", "duration": 180} for i in range(300)]
        songs = reference_songs({"tracks": tracks}, random.Random(7))
        self.assertEqual(len(songs), 36)
        self.assertEqual(sum(int(s["title"]) < 100 for s in songs[:12]), 8)
        self.assertEqual(sum(int(s["title"]) >= 100 for s in songs[:12]), 4)
        self.assertEqual(len({(s["title"], s["artist"]) for s in songs}), 36)


if __name__ == "__main__":
    unittest.main()
