"""Synthesize the music library the demo recordings browse and play.

Every track is generated here (drums, bass, pads and arpeggios, so the
analyzer has something to show) and tagged with made-up artists, plus the
album iTunes put into everyone's library in September 2014. Encoding needs
ffmpeg; playing the .m4a files in cassetty needs it too.

usage: python3 library.py <out dir>
"""

import os
import subprocess
import sys
import tempfile
import wave

import numpy as np

SR = 44100
rng = np.random.default_rng(7)


def midi(n):
    return 440.0 * 2 ** ((n - 69) / 12)


def env(n, attack, decay):
    t = np.arange(n) / SR
    a = np.clip(t / max(attack, 1e-4), 0, 1)
    return a * np.exp(-t / decay)


def additive(freq, n, harmonics, square=False, detune=0.0):
    t = np.arange(n) / SR
    out = np.zeros(n)
    f = freq * (1 + detune)
    for h in range(1, harmonics + 1):
        if square and h % 2 == 0:
            continue
        if f * h > 15000:
            break
        out += np.sin(2 * np.pi * f * h * t + h) / h
    return out


def kick(n):
    t = np.arange(n) / SR
    f = 45 + 90 * np.exp(-t / 0.04)
    ph = 2 * np.pi * np.cumsum(f) / SR
    return np.sin(ph) * np.exp(-t / 0.22)


def snare(n):
    t = np.arange(n) / SR
    noise = rng.standard_normal(n)
    noise = np.concatenate([[0], np.diff(noise)]) * 0.5 + noise * 0.3
    return noise * np.exp(-t / 0.12) * 0.5 + np.sin(2 * np.pi * 190 * t) * np.exp(-t / 0.06) * 0.6


def hat(n):
    t = np.arange(n) / SR
    noise = np.diff(rng.standard_normal(n + 1))
    return noise * np.exp(-t / 0.03) * 0.25


def place(buf, start, sig):
    end = min(len(buf), start + len(sig))
    if start < end:
        buf[start:end] += sig[: end - start]


CHORDS = {
    "maj": [0, 4, 7, 11],
    "min": [0, 3, 7, 10],
}


def render_loop(spec, flags):
    """Render four bars as stereo float arrays."""
    bpm = spec["bpm"]
    beat = int(SR * 60 / bpm)
    bar = beat * 4
    n = bar * 4
    L = np.zeros(n)
    R = np.zeros(n)
    sixteenth = beat // 4
    for b, (root, quality) in enumerate(spec["prog"]):
        base = b * bar
        tones = [root + i for i in CHORDS[quality]]
        if flags.get("drums"):
            for k in spec["kick"]:
                s = kick(int(SR * 0.4))
                place(L, base + k * sixteenth, s * 0.9)
                place(R, base + k * sixteenth, s * 0.9)
            for k in (4, 12):
                s = snare(int(SR * 0.3))
                place(L, base + k * sixteenth, s * 0.55)
                place(R, base + k * sixteenth, s * 0.5)
            step = 2 if spec["hats"] == 8 else 1
            for k in range(0, 16, step):
                s = hat(int(SR * 0.08)) * (1.0 if k % 4 == 2 else 0.6)
                place(L, base + k * sixteenth, s * 0.5)
                place(R, base + k * sixteenth, s * 0.9)
        if flags.get("bass"):
            for k, on in enumerate(spec["bass"]):
                if not on:
                    continue
                note = tones[0] - 24 + (12 if on == 2 else 0)
                ln = sixteenth * 2
                s = additive(midi(note), ln, 8) * env(ln, 0.005, 0.18) * 0.5
                place(L, base + k * sixteenth, s)
                place(R, base + k * sixteenth, s)
        if flags.get("pad"):
            pl = np.zeros(bar)
            pr = np.zeros(bar)
            for t in tones[:3]:
                f = midi(t)
                pl += additive(f, bar, 10, detune=-0.003)
                pr += additive(f, bar, 10, detune=0.004)
            e = env(bar, 0.35, 6.0)
            place(L, base, pl * e * 0.09)
            place(R, base, pr * e * 0.09)
        if flags.get("arp"):
            pattern = spec["arp"]
            for k in range(16):
                idx = pattern[k % len(pattern)]
                note = tones[idx % len(tones)] + 12 * (1 + idx // len(tones))
                ln = sixteenth
                s = additive(midi(note), ln, 7, square=True) * env(ln, 0.002, 0.09) * 0.16
                pan = 0.5 + 0.35 * np.sin(k / 16 * 2 * np.pi)
                place(L, base + k * sixteenth, s * (1 - pan) * 2)
                place(R, base + k * sixteenth, s * pan * 2)
        if flags.get("lead"):
            for k, iv in enumerate(spec["lead"]):
                if iv is None:
                    continue
                note = tones[0] + 24 + iv
                ln = beat
                s = additive(midi(note), ln, 5) * env(ln, 0.02, 0.5) * 0.18
                vib = 1 + 0.002 * np.sin(np.arange(ln) / SR * 2 * np.pi * 5)
                place(L, base + k * beat, s * vib * 0.8)
                place(R, base + k * beat, s * vib)
    return L, R


ARRANGEMENT = [
    {"drums": 1, "bass": 1, "pad": 1},
    {"drums": 1, "bass": 1, "pad": 1, "arp": 1},
    {"drums": 1, "bass": 1, "pad": 1},
    {"drums": 1, "bass": 1, "pad": 1, "arp": 1},
    {"drums": 1, "bass": 1, "pad": 1, "arp": 1, "lead": 1},
    {"drums": 1, "bass": 1, "arp": 1, "lead": 1},
    {"bass": 1, "pad": 1, "arp": 1},
    {"drums": 1, "bass": 1, "pad": 1, "arp": 1, "lead": 1},
]


def render_track(spec):
    cache = {}
    Ls, Rs = [], []
    if "dur" in spec:
        loop = 16 * int(SR * 60 / spec["bpm"]) / SR
        spec["loops"] = int(spec["dur"] // loop) + 1
    for i in range(spec["loops"]):
        if i < 2:
            flags = ARRANGEMENT[i]
        elif i >= spec["loops"] - 1:
            flags = {"pad": 1, "arp": 1}
        else:
            flags = ARRANGEMENT[2 + (i - 2) % (len(ARRANGEMENT) - 2)]
        key = tuple(sorted(flags))
        if key not in cache:
            cache[key] = render_loop(spec, flags)
        L, R = cache[key]
        Ls.append(L)
        Rs.append(R)
    L = np.concatenate(Ls)
    R = np.concatenate(Rs)
    if "dur" in spec:
        L = L[: int(spec["dur"] * SR)]
        R = R[: int(spec["dur"] * SR)]
    fade = int(SR * 3)
    ramp = np.linspace(1, 0, fade)
    L[-fade:] *= ramp
    R[-fade:] *= ramp
    st = np.stack([L, R], axis=1)
    st = np.tanh(st * 1.4)
    st /= np.max(np.abs(st)) / 0.89
    return (st * 32767).astype(np.int16)


def write_wav(path, data):
    with wave.open(path, "wb") as w:
        w.setnchannels(2)
        w.setsampwidth(2)
        w.setframerate(SR)
        w.writeframes(data.tobytes())


def prog(root, *qs):
    return [(root + off, q) for off, q in qs]


LIBRARY = [
    ("Analog Summer", "Sunbleached", "1987", [
        ("Dashboard Heat", "flac", dict(bpm=112, prog=prog(57, (0, "min"), (-4, "maj"), (-9, "maj"), (-2, "maj")), loops=24)),
        ("Faded Rainbow", "mp3", dict(bpm=104, prog=prog(60, (0, "maj"), (9, "min"), (5, "maj"), (7, "maj")), loops=22)),
        ("Polaroid Afternoon", "mp3", dict(bpm=96, prog=prog(62, (0, "maj"), (4, "min"), (5, "maj"), (0, "maj")), loops=20)),
        ("Rewind the Coast", "mp3", dict(bpm=120, prog=prog(55, (0, "maj"), (2, "min"), (4, "min"), (5, "maj")), loops=28)),
        ("Glovebox Mixtape", "mp3", dict(bpm=108, prog=prog(64, (0, "min"), (5, "min"), (3, "maj"), (-2, "maj")), loops=21)),
        ("Side B Sunset", "mp3", dict(bpm=92, prog=prog(58, (0, "maj"), (5, "maj"), (9, "min"), (7, "maj")), loops=19)),
    ]),
    ("Reel Mechanics", "Capstan Drive", "1984", [
        ("Pinch Roller", "mp3", dict(bpm=124, prog=prog(52, (0, "min"), (3, "maj"), (8, "maj"), (10, "maj")), loops=26)),
        ("Wow and Flutter", "mp3", dict(bpm=100, prog=prog(59, (0, "min"), (-2, "maj"), (-4, "maj"), (-5, "maj")), loops=20)),
        ("Azimuth", "mp3", dict(bpm=116, prog=prog(61, (0, "maj"), (7, "maj"), (9, "min"), (5, "maj")), loops=24)),
        ("Bias Tone", "mp3", dict(bpm=88, prog=prog(56, (0, "min"), (8, "maj"), (3, "maj"), (10, "maj")), loops=17)),
    ]),
    # The album the demos play.
    ("The Hiss Choir", "Chrome Position", "1991", [
        ("Rewind It With a Pencil", "mp3", dict(bpm=110, prog=prog(63, (0, "maj"), (5, "maj"), (2, "min"), (7, "maj")), loops=22)),
        ("Degauss Me Gently", "mp3", dict(bpm=104, prog=prog(62, (0, "maj"), (7, "maj"), (9, "min"), (5, "maj")), loops=21)),
        ("How Do I Exit Vim", "mp3", dict(bpm=128, prog=prog(53, (0, "min"), (5, "min"), (7, "min"), (3, "maj")), loops=30)),
        ("Burned Into My Phosphor", "mp3", dict(bpm=94, prog=prog(60, (0, "min"), (-4, "maj"), (3, "maj"), (-2, "maj")), loops=18)),
        ("Ctrl-S Froze My Heart", "mp3", dict(bpm=118, prog=prog(57, (0, "min"), (5, "maj"), (8, "maj"), (3, "maj")), loops=25)),
        ("Tape Spaghetti", "mp3", dict(bpm=122, prog=prog(58, (0, "maj"), (7, "maj"), (5, "maj"), (2, "min")), loops=26)),
        ("Kill -9 My Darling", "mp3", dict(bpm=112, prog=prog(59, (0, "min"), (8, "maj"), (3, "maj"), (10, "maj")), loops=23)),
        ("The 15 kHz Whine", "mp3", dict(bpm=126, prog=prog(52, (0, "min"), (3, "maj"), (5, "maj"), (10, "maj")), loops=24)),
        ("Heart Dumped (Segmentation Fault)", "mp3", dict(bpm=96, prog=prog(60, (0, "maj"), (4, "min"), (5, "maj"), (7, "maj")), loops=19)),
        ("Side B Is Forty Minutes of Hiss", "mp3", dict(bpm=88, prog=prog(61, (0, "maj"), (9, "min"), (4, "min"), (5, "maj")), loops=20)),
        ("Command Not Found", "mp3", dict(bpm=116, prog=prog(54, (0, "min"), (5, "min"), (3, "maj"), (8, "maj")), loops=24)),
    ]),
    ("Night Drive Tapes", "Vol. 2", "1989", [
        ("Sodium Lights", "mp3", dict(bpm=118, prog=prog(57, (0, "min"), (5, "min"), (-2, "maj"), (3, "maj")), loops=25)),
        ("Overpass", "mp3", dict(bpm=102, prog=prog(54, (0, "min"), (-4, "maj"), (-7, "maj"), (-2, "maj")), loops=21)),
        ("Last Exit", "mp3", dict(bpm=90, prog=prog(59, (0, "maj"), (4, "min"), (9, "min"), (5, "maj")), loops=19)),
    ]),
    # Nobody asked for it, but there it is.
    ("U2", "Songs of Innocence", "2014", [
        ("The Miracle (of Joey Ramone)", "m4a", dict(bpm=118, prog=prog(57, (0, "min"), (5, "maj"), (8, "maj"), (3, "maj")), dur=255)),
        ("Every Breaking Wave", "m4a", dict(bpm=104, prog=prog(62, (0, "maj"), (7, "maj"), (9, "min"), (5, "maj")), dur=252)),
        ("California (There Is No End to Love)", "m4a", dict(bpm=110, prog=prog(64, (0, "maj"), (5, "maj"), (9, "min"), (7, "maj")), dur=240)),
        ("Song for Someone", "m4a", dict(bpm=96, prog=prog(60, (0, "maj"), (4, "min"), (5, "maj"), (7, "maj")), dur=227)),
        ("Iris (Hold Me Close)", "m4a", dict(bpm=112, prog=prog(59, (0, "min"), (8, "maj"), (3, "maj"), (10, "maj")), dur=319)),
        ("Volcano", "m4a", dict(bpm=126, prog=prog(52, (0, "min"), (3, "maj"), (5, "maj"), (10, "maj")), dur=194)),
        ("Raised by Wolves", "m4a", dict(bpm=120, prog=prog(55, (0, "min"), (-2, "maj"), (-4, "maj"), (-5, "maj")), dur=246)),
        ("Cedarwood Road", "m4a", dict(bpm=116, prog=prog(54, (0, "min"), (5, "min"), (3, "maj"), (8, "maj")), dur=265)),
        ("Sleep Like a Baby Tonight", "m4a", dict(bpm=88, prog=prog(61, (0, "maj"), (9, "min"), (4, "min"), (5, "maj")), dur=302)),
        ("This Is Where You Can Reach Me Now", "m4a", dict(bpm=122, prog=prog(58, (0, "maj"), (7, "maj"), (5, "maj"), (2, "min")), dur=305)),
        ("The Troubles", "m4a", dict(bpm=92, prog=prog(56, (0, "min"), (8, "maj"), (10, "maj"), (3, "maj")), dur=285)),
    ]),
]

KICKS = [[0, 8], [0, 6, 8], [0, 8, 10], [0, 4, 8, 12], [0, 7, 10]]
BASSES = [
    [1, 0, 1, 0, 2, 0, 1, 1, 0, 1, 2, 0, 1, 0, 1, 0],
    [1, 0, 0, 1, 0, 0, 1, 0, 1, 0, 0, 1, 0, 2, 0, 0],
    [1, 1, 0, 1, 2, 0, 1, 0, 1, 1, 0, 1, 2, 0, 1, 0],
]
ARPS = [[0, 1, 2, 3, 4, 3, 2, 1], [0, 2, 4, 2, 1, 3, 5, 3], [0, 4, 2, 5, 1, 4, 3, 6]]
LEADS = [[0, None, 3, 2], [7, 5, None, 3], [0, 2, 3, None], [None, 7, 5, 4]]


def main():
    out = sys.argv[1]
    n = 0
    for artist, album, year, tracks in LIBRARY:
        folder = os.path.join(out, artist, f"{album} ({year})")
        os.makedirs(folder, exist_ok=True)
        for i, (title, fmt, spec) in enumerate(tracks, 1):
            spec.setdefault("kick", KICKS[n % len(KICKS)])
            spec.setdefault("bass", BASSES[n % len(BASSES)])
            spec.setdefault("arp", ARPS[n % len(ARPS)])
            spec.setdefault("lead", LEADS[n % len(LEADS)])
            spec.setdefault("hats", 16 if n % 2 else 8)
            n += 1
            dst = os.path.join(folder, f"{i:02d} {title}.{fmt}")
            if os.path.exists(dst):
                continue
            data = render_track(spec)
            with tempfile.NamedTemporaryFile(suffix=".wav") as tmp:
                write_wav(tmp.name, data)
                codec = {
                    "flac": ["-c:a", "flac"],
                    "mp3": ["-c:a", "libmp3lame", "-b:a", "192k"],
                    "m4a": ["-c:a", "aac", "-b:a", "256k"],  # iTunes Plus
                }[fmt]
                subprocess.run(
                    ["ffmpeg", "-loglevel", "error", "-y", "-i", tmp.name, *codec,
                     "-metadata", f"title={title}", "-metadata", f"artist={artist}",
                     "-metadata", f"album={album}", "-metadata", f"date={year}",
                     "-metadata", f"track={i}/{len(tracks)}", dst],
                    check=True,
                )
            print(dst, f"{len(data) / SR:.0f}s")


if __name__ == "__main__":
    main()
