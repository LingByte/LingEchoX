#!/usr/bin/env python3
"""
Send an HTTP request signed with tenant Access Key / Secret Key (X-Ak, X-Ts, X-Sign).

Signing matches internal/models/credential.go:
  stringToSign = METHOD + "\\n" + pathWithSortedQuery + "\\n" + ts + "\\n" + sha256hex(body)
  X-Sign = hex(hmac-sha256(secretKey, stringToSign))

Examples:
  python3 scripts/aksk_request.py \\
    --ak ak_xxx --sk your_secret_hex \\
    --url 'http://localhost:8080/api/sip-center/trunk-numbers'

  python3 scripts/aksk_request.py -ak ak_xxx -sk skhex -u 'http://localhost:8080/api/sip-center/trunks' -v
"""

from __future__ import annotations

import argparse
import hashlib
import hmac
import subprocess
import sys
import time
from urllib.parse import parse_qsl, urlencode, urlparse

EMPTY_BODY_SHA256 = hashlib.sha256(b"").hexdigest()


def path_with_sorted_query(url: str) -> str:
    parsed = urlparse(url)
    path = parsed.path or "/"
    if not parsed.query:
        return path
    pairs = parse_qsl(parsed.query, keep_blank_values=True)
    return path + "?" + urlencode(sorted(pairs), doseq=True)


def build_string_to_sign(method: str, path_with_query: str, ts: str, body: bytes) -> str:
    body_hash = hashlib.sha256(body).hexdigest()
    return f"{method.upper()}\n{path_with_query}\n{ts}\n{body_hash}"


def sign_hex(secret_key: str, message: str) -> str:
    return hmac.new(secret_key.encode(), message.encode(), hashlib.sha256).hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser(
        description="AK/SK signed HTTP request for LingEchoX API",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("--ak", "-a", required=True, help="Access key (X-Ak)")
    parser.add_argument("--sk", "-s", required=True, help="Secret key")
    parser.add_argument(
        "--url",
        "-u",
        required=True,
        help="Full URL, e.g. http://localhost:8080/api/sip-center/trunk-numbers",
    )
    parser.add_argument(
        "-X",
        "--method",
        default="GET",
        help="HTTP method (default: GET)",
    )
    parser.add_argument(
        "--body",
        "-b",
        default="",
        help="Request body (for POST/PUT/PATCH)",
    )
    parser.add_argument(
        "--body-file",
        "-f",
        help="Read request body from file",
    )
    parser.add_argument(
        "-v",
        "--verbose",
        action="store_true",
        help="Print string-to-sign and curl command on stderr",
    )
    parser.add_argument(
        "-i",
        "--include-headers",
        action="store_true",
        help="Pass -i to curl (show response headers)",
    )
    args = parser.parse_args()

    ak = args.ak.strip().lstrip("\ufeff")
    sk = args.sk.strip()
    method = args.method.upper()

    body = b""
    if args.body_file:
        with open(args.body_file, "rb") as f:
            body = f.read()
    elif args.body:
        body = args.body.encode()

    no_body_methods = {"GET", "HEAD", "DELETE", "OPTIONS", "TRACE"}
    if method in no_body_methods:
        body = b""

    path_q = path_with_sorted_query(args.url)
    ts = str(int(time.time()))
    string_to_sign = build_string_to_sign(method, path_q, ts, body)
    signature = sign_hex(sk, string_to_sign)

    if args.verbose:
        print("path (signed):", path_q, file=sys.stderr)
        print("string-to-sign:", repr(string_to_sign), file=sys.stderr)
        print("body sha256:", hashlib.sha256(body).hexdigest(), file=sys.stderr)
        print("empty body sha256:", EMPTY_BODY_SHA256, file=sys.stderr)
        print("X-Ts:", ts, file=sys.stderr)
        print("X-Sign:", signature, file=sys.stderr)

    curl_cmd = [
        "curl",
        "-sS",
        "-X",
        method,
        args.url,
        "-H",
        f"X-Ak: {ak}",
        "-H",
        f"X-Ts: {ts}",
        "-H",
        f"X-Sign: {signature}",
    ]
    if args.include_headers:
        curl_cmd.insert(1, "-i")
    if body and method not in no_body_methods:
        curl_cmd.extend(["-H", "Content-Type: application/json", "--data-binary", body])

    if args.verbose:
        print("curl:", " ".join(curl_cmd[:12]), "...", file=sys.stderr)

    try:
        result = subprocess.run(curl_cmd, check=False)
    except FileNotFoundError:
        print("curl not found; install curl or run with --verbose and call manually", file=sys.stderr)
        return 127
    return result.returncode


if __name__ == "__main__":
    sys.exit(main())
