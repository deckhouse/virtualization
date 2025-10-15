#!/usr/bin/env python3
# Copyright 2025 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Send notifications to Loop webhook."""

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path


def load_env_file(env_path: Path) -> None:
    """Load environment variables from .env file."""
    if not env_path.exists():
        return
    
    with open(env_path, 'r') as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#') and '=' in line:
                key, value = line.split('=', 1)
                # Don't override existing env vars
                if key not in os.environ:
                    os.environ[key] = value.strip('"').strip("'")


def send_post_request(url: str, channel: str, text: str) -> None:
    """Send JSON payload to Loop webhook."""

    payload = json.dumps({"channel": channel, "text": text}).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    with urllib.request.urlopen(request, timeout=30) as response:  # noqa: S310
        # We just ensure the request succeeded; the body is usually empty.
        response.read()


def main(argv: list[str]) -> int:
    # Load .env file if it exists
    env_path = Path(__file__).parent.parent / '.env'
    load_env_file(env_path)
    
    parser = argparse.ArgumentParser(description="Send message to Loop webhook")
    parser.add_argument("--url", required=False, help="Loop webhook URL", default=os.getenv('LOOP_WEBHOOK'))
    parser.add_argument("--channel", required=False, help="Loop channel name", default=os.getenv('LOOP_CHANNEL', 'test-virtualization-loop-alerts'))
    parser.add_argument("--text", required=True, help="Message text")

    args = parser.parse_args(argv)
    
    if not args.url:
        print("[ERR] LOOP_WEBHOOK not set. Set via --url or LOOP_WEBHOOK env variable", file=sys.stderr)
        return 1

    try:
        send_post_request(url=args.url, channel=args.channel, text=args.text)
    except urllib.error.HTTPError as exc:  # pragma: no cover - network failure path
        print(f"[ERR] HTTP error {exc.code}: {exc.reason}", file=sys.stderr)
        return 1
    except urllib.error.URLError as exc:  # pragma: no cover - network failure path
        print(f"[ERR] URL error: {exc.reason}", file=sys.stderr)
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))

