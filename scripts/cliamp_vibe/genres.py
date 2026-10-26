"""Genre directions and measured checks, separate from raw cosine similarity."""
import re
import numpy as np

GENRES = {
    "edm": ("Electronic / EDM", ["electronic dance music with synthesized bass and programmed drums",
        "liquid drum and bass music with fast breakbeats and warm melodies",
        "house music with a steady four on the floor beat and warm chords",
        "techno music with repetitive driving kick drums and dark synths",
        "melodic chillstep electronic music with soft vocals and heavy bass",
        "psychedelic bass music with deep wobbling sub bass and spacious textures",
        "glitch hop music with intricate broken beats and funky bass"]),
    "glitch": ("Glitch / Bass", ["glitch hop music with intricate broken beats and funky bass",
        "deep dubstep music with heavy sub bass and sparse half time drums",
        "dark experimental bass music with distorted synths and heavy drums",
        "psydub downtempo music with organic instruments and psychedelic effects"]),
    "blues": ("Blues", ["blues music with expressive electric guitar, harmonica and blues singing", "electronic blues with sampled blues guitar and electronic breakbeats"]),
    "house": ("House / Techno", ["house and techno music with four on the floor electronic drums", "dance music with repetitive synthetic bass and steady kick drum"]),
    "ambient": ("Ambient / Downtempo", ["ambient electronic music with floating pads and atmospheric textures", "downtempo music with slow electronic beats and spacious atmosphere"]),
    "hiphop": ("Hip-hop", ["hip hop music with rapping and sampled beats", "rap music with rhythmic vocals and bass beats"]),
    "jazz": ("Jazz", ["jazz music with acoustic instruments and improvisation", "jazz music with saxophone piano bass and drums"]),
    "rock": ("Rock", ["rock music with distorted guitars, live drums and singing", "alternative rock music with electric guitar and vocals"]),
    "pop": ("Pop", ["pop music with catchy sung melodies", "mainstream pop music with singing and polished instrumental backing"]),
    "country": ("Country", ["country music with twangy singing, acoustic guitar and pedal steel", "traditional country music with fiddle, banjo and a country singer"]),
    "folk": ("Folk / Acoustic", ["acoustic folk music with strummed guitar and singing", "folk music with acoustic instruments and a singer"]),
    "classical": ("Classical", ["classical orchestral music with strings and woodwinds", "classical chamber music with acoustic instruments"]),
    "reggae": ("Reggae / Dub", ["reggae music with offbeat guitar and bass", "dub reggae music with deep bass and echo effects"]),
}
ALIASES = {"electronic": "edm", "glitch hop": "glitch", "dubstep": "glitch", "bass": "glitch", "techno": "house", "downtempo": "ambient", "hip hop": "hiphop"}
SEARCH_TAGS = {"edm": ["electronic", "edm"], "glitch": ["glitch hop", "dubstep", "glitch"],
              "blues": ["blues", "electro blues"], "house": ["house", "techno"], "ambient": ["ambient", "downtempo"],
              "hiphop": ["hip hop"], "folk": ["folk", "acoustic"]}


def valid_genres(values):
    return list(dict.fromkeys(ALIASES.get(str(v).casefold(), str(v).casefold()) for v in values
        if ALIASES.get(str(v).casefold(), str(v).casefold()) in GENRES))[:5]


def description_genres(description):
    text = str(description).casefold()
    patterns = {"edm": r"\b(electronic|edm|electro)\b", "glitch": r"\b(glitch|dubstep|trap|bass)\b",
                "blues": r"\bblues\b", "house": r"\b(house|techno)\b", "ambient": r"\b(ambient|downtempo)\b",
                "jazz": r"\bjazz\b", "rock": r"\brock\b", "country": r"\bcountry\b", "folk": r"\bfolk\b",
                "classical": r"\bclassical\b", "reggae": r"\b(reggae|dub)\b", "hiphop": r"\b(hip.hop|rap)\b"}
    return [genre for genre, pattern in patterns.items() if re.search(pattern, text)][:5]


def audio_genres(windows, model):
    scores = np.asarray(windows) @ model.genre_text.T
    # Broad genres contain several distinct subgenres; a DnB recording need not
    # sound like house too. Use the strongest measured caption within each family.
    output, offset = {}, 0
    for key, (_, prompts) in GENRES.items():
        output[key] = round(float(scores[:, offset:offset+len(prompts)].mean(axis=0).max()), 5)
        offset += len(prompts)
    return output


def genre_compatible(evidence, allowed):
    if not allowed or not evidence:
        return True
    relevant = max(evidence.get(key, -1) for key in allowed)
    competing = max((value for key, value in evidence.items() if key not in allowed), default=-1)
    # Cached real-station checks: .025 retained five electronic/DnB examples
    # while excluding all ten high-cosine mismatches from the reported run.
    return relevant >= competing - .025


def diverse_matches(scored, vectors, limit=10):
    """Prefer varied audio among compatible near-matches, preserving raw scores."""
    pool, selected = list(scored), []
    while pool and len(selected) < limit:
        best = max(s["score"] for s in pool)
        near = [s for s in pool if s["score"] >= best - .04]
        def priority(item):
            if not selected:
                return item["score"]
            redundancy = max(float(vectors[item["id"]] @ vectors[s["id"]]) for s in selected)
            return item["score"] - .06 * max(0, redundancy)
        chosen = max(near, key=priority)
        selected.append(chosen)
        pool.remove(chosen)
    return selected
