#!/usr/bin/env python3
"""Regenerate the guided-tour narration clips in public/audio/tour.

The tour text lives in src/constants/tours.ts. When a step's body changes, run
this script so the recorded narration matches the on-screen text again.

Requirements:
  - MiniMax T2A helper at $MMTTS (default ~/Desktop/tts-test/mmtts.py)
  - ffmpeg on PATH (used to shrink the clips to 22.05kHz mono 24kbps mp3)

Usage:
  python web/scripts/generate-tour-audio.py            # only missing/changed
  python web/scripts/generate-tour-audio.py --force    # regenerate everything

The MiniMax endpoint is rate limited per minute, so the script spaces requests
and retries throttled chunks instead of failing the whole run.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
TOURS_TS = ROOT / "src" / "constants" / "tours.ts"
OUT_DIR = ROOT / "public" / "audio" / "tour"
MANIFEST = OUT_DIR / "manifest.json"
VOICE = "Chinese (Mandarin)_Gentle_Senior"
MMTTS = Path(os.environ.get("MMTTS", Path.home() / "Desktop" / "tts-test" / "mmtts.py"))
THROTTLE_SECONDS = 2.2


def parse_tours() -> dict[str, list[str]]:
    source = TOURS_TS.read_text(encoding="utf-8")
    tours: dict[str, list[str]] = {}
    # Blocks are indented differently for page tours and form tours, so accept
    # any closing indentation between 2 and 4 spaces.
    pattern = r"key: '([a-z-]+)',\s*title: '[^']*',\s*steps: \[(.*?)\n\s{2,4}\],"
    for key, body in re.findall(pattern, source, re.S):
        tours[key] = re.findall(r"body: '([^']*)'", body)
    if not tours:
        raise SystemExit(f"no tours found in {TOURS_TS}")
    return tours


def manifest_key(text: str) -> str:
    import hashlib

    return hashlib.sha1(text.encode("utf-8")).hexdigest()[:12]


def synthesize(text: str, target: Path) -> bool:
    raw = target.with_suffix(".raw.mp3")
    result = subprocess.run(
        [sys.executable, str(MMTTS), "--text", text, "--out", str(raw),
         "--voice", VOICE, "--bitrate", "32000", "--sample-rate", "24000"],
        capture_output=True, text=True, encoding="utf-8",
    )
    try:
        info = json.loads(result.stdout.strip().splitlines()[-1])
    except (IndexError, json.JSONDecodeError):
        info = {"ok": False, "error": (result.stdout or result.stderr)[-200:]}
    if not info.get("ok"):
        raw.unlink(missing_ok=True)
        print(f"  retry later: {info.get('error')}")
        return False
    # Compress: mono 22.05 kHz @ 24 kbps keeps speech clear at ~1/25 the size of
    # a 128 kbps stereo clip.
    compress = subprocess.run(
        ["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", str(raw),
         "-ac", "1", "-ar", "22050", "-b:a", "24k", "-map_metadata", "-1", str(target)],
        capture_output=True, text=True,
    )
    raw.unlink(missing_ok=True)
    if compress.returncode != 0:
        print(f"  ffmpeg failed: {compress.stderr[-200:]}")
        return False
    return True


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--force", action="store_true", help="regenerate every clip")
    args = parser.parse_args()

    if not MMTTS.exists():
        raise SystemExit(f"mmtts.py not found at {MMTTS}; set MMTTS=/path/to/mmtts.py")
    if not shutil.which("ffmpeg"):
        raise SystemExit("ffmpeg is required to compress the clips")

    OUT_DIR.mkdir(parents=True, exist_ok=True)
    previous = json.loads(MANIFEST.read_text(encoding="utf-8")) if MANIFEST.exists() else {}
    tours = parse_tours()

    pending: list[tuple[str, str, Path]] = []
    manifest: dict[str, str] = {}
    for key, bodies in tours.items():
        for index, body in enumerate(bodies):
            name = f"{key}-{index}.mp3"
            digest = manifest_key(body)
            manifest[name] = digest
            target = OUT_DIR / name
            unchanged = previous.get(name) == digest and not args.force
            if unchanged and target.exists():
                continue
            pending.append((name, body, target))

    print(f"{len(pending)} clip(s) to generate, {len(manifest) - len(pending)} up to date")
    for name, body, target in pending:
        print(f"- {name}")
        for attempt in range(6):
            if synthesize(body, target):
                break
            time.sleep(20 + attempt * 10)
        else:
            print(f"  giving up on {name}")
        time.sleep(THROTTLE_SECONDS)

    MANIFEST.write_text(json.dumps(manifest, ensure_ascii=False, indent=1), encoding="utf-8")
    print("done")
    return 0


if __name__ == "__main__":
    sys.exit(main())
