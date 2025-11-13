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
"""Parse matrix test logs and send summary to Loop webhook."""

import argparse
import json
import os
import re
import sys
import urllib.error
import urllib.request
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


def parse_test_log(log_file: Path) -> dict:
    """Parse test log file and extract results."""
    try:
        content = log_file.read_text(encoding='utf-8')
        
        # Extract run ID from filename or content
        run_id = log_file.stem
        
        # Look for test completion patterns
        success_patterns = [
            r'\[OK\] run_id=([^\s]+) finished',
            r'Ginkgo ran \d+ spec in [\d.]+s',
            r'Test Suite Passed'
        ]
        
        failure_patterns = [
            r'\[ERR\] run_id=([^\s]+) failed',
            r'Ginkgo ran \d+ spec in [\d.]+s.*FAILED',
            r'Test Suite Failed',
            r'API response status: Failure',
            r'admission webhook .* too long',
            r'Unable to connect to the server',
            r'Error while process exit code: exit status 1',
            r'task: Failed to run task .* exit status',
            r'Infrastructure runner \"master-node\" process exited'
        ]
        
        # Check for explicit status markers first
        start_match = re.search(r'\[START\].*run_id=([^\s]+)', content)
        finish_match = re.search(r'\[FINISH\].*run_id=([^\s]+).*status=(\w+)', content)
        
        if start_match and finish_match:
            # Use explicit status markers
            success = finish_match.group(2) == 'ok'
            failure = finish_match.group(2) == 'error'
        else:
            # Fallback to pattern matching
            success = any(re.search(pattern, content, re.IGNORECASE) for pattern in success_patterns)
            failure = any(re.search(pattern, content, re.IGNORECASE) for pattern in failure_patterns)
        
        # Extract storage profile from run_id
        storage_profile = "unknown"
        if '-' in run_id:
            parts = run_id.split('-')
            if len(parts) >= 2:
                # Format: {prefix}-{profile}-{timestamp}-{random}
                # For "test-sds-20251009-221516-17193", we want "sds"
                storage_profile = parts[1]
        
        # Extract test statistics
        test_stats = {'total': 0, 'passed': 0, 'failed': 0, 'skipped': 0}
        
        # Look for Ginkgo test results
        ginkgo_match = re.search(r'Ran (\d+) of (\d+) Specs.*?(\d+) Passed.*?(\d+) Failed.*?(\d+) Skipped', content, re.DOTALL)
        if ginkgo_match:
            test_stats['total'] = int(ginkgo_match.group(1))
            test_stats['passed'] = int(ginkgo_match.group(3))
            test_stats['failed'] = int(ginkgo_match.group(4))
            test_stats['skipped'] = int(ginkgo_match.group(5))
        
        # Extract timing information
        duration = "unknown"
        # Prefer explicit START/FINISH ISO markers
        start_match = re.search(r'^\[START\].*time=([^\s]+)', content, re.MULTILINE)
        finish_match = re.search(r'^\[FINISH\].*time=([^\s]+)', content, re.MULTILINE)
        if start_match and finish_match:
            try:
                started = datetime.fromisoformat(start_match.group(1))
                finished = datetime.fromisoformat(finish_match.group(1))
                delta = finished - started
                total_seconds = int(delta.total_seconds())
                hours = total_seconds // 3600
                minutes = (total_seconds % 3600) // 60
                seconds = total_seconds % 60
                duration = f"{hours}h {minutes}m {seconds}s"
            except Exception:
                pass
        else:
            # Fallback: try to find H:M:S pattern
            time_match = re.search(r'(\d+):(\d+):(\d+)', content)
            if time_match:
                hours, minutes, seconds = time_match.groups()
                duration = f"{hours}h {minutes}m {seconds}s"
        
        # Extract error details - only from E2E test execution
        error_details = []
        if failure:
            # Look for E2E test errors after "Running Suite" or "go run ginkgo"
            e2e_start_patterns = [
                r'Running Suite:',
                r'go run.*ginkgo',
                r'Will run.*specs'
            ]
            
            # Find E2E test section
            e2e_start_pos = -1
            for pattern in e2e_start_patterns:
                match = re.search(pattern, content, re.IGNORECASE)
                if match:
                    e2e_start_pos = match.start()
                    break
            
            if e2e_start_pos > 0:
                # Extract content after E2E tests started
                e2e_content = content[e2e_start_pos:]
                
                # Look for actual test failures with cleaner patterns
                test_error_patterns = [
                    r'\[FAIL\].*?([^\n]+)',
                    r'FAIL!.*?--.*?(\d+) Passed.*?(\d+) Failed',
                    r'Test Suite Failed',
                    r'Ginkgo ran.*FAILED',
                    r'Error occurred during reconciliation.*?([^\n]+)',
                    r'Failed to update resource.*?([^\n]+)',
                    r'admission webhook.*denied the request.*?([^\n]+)',
                    r'context deadline exceeded',
                    r'timed out waiting for the condition.*?([^\n]+)',
                    r'panic.*?([^\n]+)'
                ]
                
                for pattern in test_error_patterns:
                    matches = re.findall(pattern, e2e_content, re.IGNORECASE | re.DOTALL)
                    for match in matches:
                        if isinstance(match, tuple):
                            # Clean up the error message
                            error_msg = f"{match[0]}: {match[1]}"
                        else:
                            error_msg = match
                        
                        # Clean up ANSI escape codes and extra whitespace
                        error_msg = re.sub(r'\x1b\[[0-9;]*[mK]', '', error_msg)
                        error_msg = re.sub(r'\[0m\s*\[38;5;9m\s*\[1m', '', error_msg)
                        error_msg = re.sub(r'\[0m', '', error_msg)
                        error_msg = error_msg.strip()
                        
                        # Skip empty, very short messages, or artifacts
                        if len(error_msg) > 10 and not re.match(r'^\d+:\s*\d+$', error_msg):
                            error_details.append(error_msg)
                
                # Remove duplicates and limit to most meaningful errors
                error_details = list(dict.fromkeys(error_details))[:2]
        
        return {
            'run_id': run_id,
            'storage_profile': storage_profile,
            'success': success and not failure,
            'failure': failure,
            'duration': duration,
            'test_stats': test_stats,
            'error_details': error_details,
            'log_file': str(log_file)
        }
    except Exception as e:
        print(f"[WARN] Failed to parse log {log_file}: {e}", file=sys.stderr)
        return {
            'run_id': log_file.stem,
            'storage_profile': 'unknown',
            'success': False,
            'failure': True,
            'duration': 'unknown',
            'test_stats': {'total': 0, 'passed': 0, 'failed': 0, 'skipped': 0},
            'error_details': [f"Failed to parse log: {e}"],
            'log_file': str(log_file)
        }


def format_matrix_summary(results: list, run_id_prefix: str, profiles: str, github_run_url: str = None) -> str:
    """Format matrix test results into a readable message."""
    total_runs = len(results)
    successful_runs = sum(1 for r in results if r['success'])
    # Treat any non-success as failure for overall counters
    failed_runs = total_runs - successful_runs
    
    # Calculate total test statistics
    total_tests = sum(r['test_stats']['total'] for r in results)
    total_passed = sum(r['test_stats']['passed'] for r in results)
    total_failed = sum(r['test_stats']['failed'] for r in results)
    total_skipped = sum(r['test_stats']['skipped'] for r in results)
    
    # Determine overall status
    if total_runs == 0:
        status_emoji = "⚪"
        status_text = "NO RUNS"
    elif failed_runs > 0:
        status_emoji = "❌"
        status_text = "FAILED"
    else:
        # No failures. Consider Passed if any run succeeded (skips allowed)
        status_emoji = "✅"
        status_text = "PASSED"
    
    # Group results by storage profile
    profile_results = {}
    for result in results:
        profile = result['storage_profile']
        if profile not in profile_results:
            profile_results[profile] = {
                'success': 0, 
                'failure': 0, 
                'test_stats': {'total': 0, 'passed': 0, 'failed': 0, 'skipped': 0}
            }
        if result['success']:
            profile_results[profile]['success'] += 1
        else:
            profile_results[profile]['failure'] += 1
        
        # Aggregate test stats
        for key in profile_results[profile]['test_stats']:
            profile_results[profile]['test_stats'][key] += result['test_stats'][key]
    
    # Build message with table format
    current_date = datetime.now().strftime('%Y-%m-%d')
    test_type = "Nightly" if run_id_prefix in ["n", "nightly"] else run_id_prefix.upper()
    
    message_lines = [
        f"# :dvp: DVP-virtualization {current_date} {test_type} e2e Tests"
    ]
    
    # Add table format for profile results
    if profile_results:
        message_lines.extend([
            "",
            "| Storage Profile | Status | Passed | Failed | Skipped | Success Rate | Duration |",
            "|----------------|--------|--------|--------|---------|--------------|----------|"
        ])
        
        for profile, stats in profile_results.items():
            total_configs = stats['success'] + stats['failure']
            config_success_rate = (stats['success'] / total_configs * 100) if total_configs > 0 else 0
            
            test_stats = stats['test_stats']
            test_success_rate = (test_stats['passed'] / test_stats['total'] * 100) if test_stats['total'] > 0 else 0
            
            status_emoji = "✅" if stats['failure'] == 0 else "❌" if stats['success'] == 0 else "⚠️"
            status_text = "PASSED" if stats['failure'] == 0 else "FAILED" if stats['success'] == 0 else "PARTIAL"
            
            # Get duration and build linked profile name
            profile_duration = "unknown"
            for result in results:
                if result['storage_profile'] == profile:
                    profile_duration = result['duration']
                    break
            name_md = f"[{profile.upper()}]({github_run_url})" if github_run_url else profile.upper()
            
            message_lines.append(
                f"| {name_md} | {status_emoji} **{status_text}** | {test_stats['passed']} | {test_stats['failed']} | {test_stats['skipped']} | {test_success_rate:.1f}% | {profile_duration} |"
            )
    
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
    
    parser = argparse.ArgumentParser(description="Parse matrix test logs and send summary to Loop")
    parser.add_argument("--profiles", required=True, help="Comma-separated list of storage profiles")
    parser.add_argument("--run-id-prefix", required=True, help="Run ID prefix")
    parser.add_argument("--log-dir", required=True, help="Directory containing log files")
    parser.add_argument("--webhook-url", required=False, help="Loop webhook URL", default=os.getenv('LOOP_WEBHOOK'))
    parser.add_argument("--channel", required=False, help="Loop channel name", default=os.getenv('LOOP_CHANNEL', 'test-virtualization-loop-alerts'))
    parser.add_argument("--github-run-url", required=False, help="GitHub Actions run URL to link from profile name")
    
    args = parser.parse_args(argv)
    
    if not args.webhook_url:
        print("[ERR] LOOP_WEBHOOK not set. Set via --webhook-url or LOOP_WEBHOOK env variable", file=sys.stderr)
        return 1
    
    log_dir = Path(args.log_dir)
    if not log_dir.exists():
        print(f"[ERR] Log directory not found: {log_dir}", file=sys.stderr)
        return 1
    
    # Find all log files
    log_files = list(log_dir.glob("*.log"))
    if not log_files:
        print(f"[WARN] No log files found in {log_dir}", file=sys.stderr)
        return 0
    
    # Parse all log files
    results = []
    for log_file in log_files:
        result = parse_test_log(log_file)
        results.append(result)
    
    # Filter by run_id_prefix and profile (no aliases; use canonical names)
    allowed_profiles = set([p.strip() for p in args.profiles.split(",")])
    filtered_results = []
    
    for result in results:
        # Filter by run_id prefix (more flexible matching)
        if not result['run_id'].startswith(args.run_id_prefix):
            continue
        
        # Filter by canonical profile name from run_id
        normalized_profile = result['storage_profile']
        if normalized_profile not in allowed_profiles:
            continue
        
        result['storage_profile'] = normalized_profile
        filtered_results.append(result)
    
    results = filtered_results
    
    if not results:
        print(f"[WARN] No results to report", file=sys.stderr)
        return 0
    
    # Format message
    message = format_matrix_summary(results, args.run_id_prefix, args.profiles, github_run_url=args.github_run_url)
    
    # Send to Loop
    if send_to_loop(args.webhook_url, args.channel, message):
        print(f"[OK] Matrix summary sent to Loop channel '{args.channel}'")
        return 0
    else:
        print(f"[ERR] Failed to send matrix summary to Loop", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
