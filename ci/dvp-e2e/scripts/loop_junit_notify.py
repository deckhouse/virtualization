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
"""Parse JUnit XML and send test results to Loop webhook."""

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET
from datetime import datetime
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


def parse_junit_xml(junit_file: Path) -> dict:
    """Parse JUnit XML file and extract test results."""
    try:
        tree = ET.parse(junit_file)
        root = tree.getroot()
        
        # Handle both testsuites and testsuite root elements
        if root.tag == 'testsuites':
            testsuites = root
        else:
            testsuites = root
            
        total_tests = int(testsuites.get('tests', 0))
        total_failures = int(testsuites.get('failures', 0))
        total_errors = int(testsuites.get('errors', 0))
        total_skipped = int(testsuites.get('skipped', 0))
        total_time = float(testsuites.get('time', 0))
        
        # Calculate success rate
        successful_tests = total_tests - total_failures - total_errors
        success_rate = (successful_tests / total_tests * 100) if total_tests > 0 else 0
        
        # Extract failed test details
        failed_tests = []
        for testsuite in testsuites.findall('testsuite'):
            for testcase in testsuite.findall('testcase'):
                failure = testcase.find('failure')
                error = testcase.find('error')
                if failure is not None or error is not None:
                    failed_tests.append({
                        'name': testcase.get('name', 'unknown'),
                        'class': testcase.get('classname', 'unknown'),
                        'time': float(testcase.get('time', 0)),
                        'message': (failure.get('message', '') if failure is not None else '') or 
                                 (error.get('message', '') if error is not None else '')
                    })
        
        return {
            'total_tests': total_tests,
            'successful_tests': successful_tests,
            'failed_tests': total_failures + total_errors,
            'skipped_tests': total_skipped,
            'success_rate': success_rate,
            'total_time': total_time,
            'failed_test_details': failed_tests[:5],  # Limit to first 5 failures
            'has_more_failures': len(failed_tests) > 5
        }
    except ET.ParseError as e:
        print(f"[ERR] Failed to parse JUnit XML: {e}", file=sys.stderr)
        return None
    except Exception as e:
        print(f"[ERR] Error processing JUnit file: {e}", file=sys.stderr)
        return None


def format_test_results(results: dict, run_id: str, storage_profile: str, timeout: str) -> str:
    """Format test results into a readable message."""
    if results is None:
        return f"❌ Failed to parse test results for {run_id}"
    
    # Determine status emoji and color
    if results['failed_tests'] == 0:
        status_emoji = "✅"
        status_text = "SUCCESS"
    elif results['success_rate'] >= 80:
        status_emoji = "⚠️"
        status_text = "PARTIALLY SUCCESS"
    else:
        status_emoji = "❌"
        status_text = "FAILED"
    
    # Format time
    time_str = f"{results['total_time']:.1f}s"
    if results['total_time'] > 60:
        minutes = int(results['total_time'] // 60)
        seconds = int(results['total_time'] % 60)
        time_str = f"{minutes}m {seconds}s"
    
    # Build message
    message_lines = [
        f"{status_emoji} E2E tests for virtualization completed",
        f"📋 Run ID: {run_id}",
        f"💾 Storage: {storage_profile}",
        f"⏱️ Timeout: {timeout}",
        f"🕐 Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
        "",
        f"📊 Results: {status_text}",
        f"• Total tests: {results['total_tests']}",
        f"• Passed: {results['successful_tests']}",
        f"• Failed: {results['failed_tests']}",
        f"• Skipped: {results['skipped_tests']}",
        f"• Success rate: {results['success_rate']:.1f}%",
        f"• Duration: {time_str}"
    ]
    
    # Add failed test details if any
    if results['failed_test_details']:
        message_lines.extend([
            "",
            "🔍 Failed tests:"
        ])
        for test in results['failed_test_details']:
            message_lines.append(f"• {test['class']}.{test['name']}")
            if test['message']:
                # Truncate long messages
                msg = test['message'][:100] + "..." if len(test['message']) > 100 else test['message']
                message_lines.append(f"  {msg}")
        
        if results['has_more_failures']:
            message_lines.append(f"• ... and {len(results['failed_test_details']) - 5} more tests")
    
    return "\n".join(message_lines)


def send_to_loop(webhook_url: str, channel: str, message: str) -> bool:
    """Send message to Loop webhook."""
    try:
        payload = json.dumps({"channel": channel, "text": message}).encode("utf-8")
        request = urllib.request.Request(
            webhook_url,
            data=payload,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        
        with urllib.request.urlopen(request, timeout=30) as response:
            response.read()
        return True
    except urllib.error.HTTPError as e:
        print(f"[ERR] HTTP error {e.code}: {e.reason}", file=sys.stderr)
        return False
    except urllib.error.URLError as e:
        print(f"[ERR] URL error: {e.reason}", file=sys.stderr)
        return False
    except Exception as e:
        print(f"[ERR] Unexpected error: {e}", file=sys.stderr)
        return False


def main(argv: list[str]) -> int:
    # Load .env file if it exists
    env_path = Path(__file__).parent.parent / '.env'
    load_env_file(env_path)
    
    parser = argparse.ArgumentParser(description="Parse JUnit XML and send results to Loop")
    parser.add_argument("--junit-file", required=True, help="Path to JUnit XML file")
    parser.add_argument("--run-id", required=True, help="Test run ID")
    parser.add_argument("--storage-profile", required=True, help="Storage profile used")
    parser.add_argument("--webhook-url", required=False, help="Loop webhook URL", default=os.getenv('LOOP_WEBHOOK'))
    parser.add_argument("--channel", required=False, help="Loop channel name", default=os.getenv('LOOP_CHANNEL', 'test-virtualization-loop-alerts'))
    parser.add_argument("--timeout", default="30m", help="Test timeout")
    
    args = parser.parse_args(argv)
    
    if not args.webhook_url:
        print("[ERR] LOOP_WEBHOOK not set. Set via --webhook-url or LOOP_WEBHOOK env variable", file=sys.stderr)
        return 1
    
    junit_file = Path(args.junit_file)
    if not junit_file.exists():
        print(f"[ERR] JUnit file not found: {junit_file}", file=sys.stderr)
        return 1
    
    # Parse JUnit results
    results = parse_junit_xml(junit_file)
    
    # Format message
    message = format_test_results(results, args.run_id, args.storage_profile, args.timeout)
    
    # Send to Loop
    if send_to_loop(args.webhook_url, args.channel, message):
        print(f"[OK] Results sent to Loop channel '{args.channel}'")
        return 0
    else:
        print(f"[ERR] Failed to send results to Loop", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
