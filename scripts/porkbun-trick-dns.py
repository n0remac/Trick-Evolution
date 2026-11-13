#!/usr/bin/env python3
"""Read or update only trick.noremac.dev's existing Porkbun A record.

Credentials live outside Git in ~/.config/porkbun/credentials.json.
"""

import argparse
import ipaddress
import json
import os
from pathlib import Path
import stat
import sys
import urllib.error
import urllib.request
import uuid


API = "https://api.porkbun.com/api/json/v3"
DOMAIN = "noremac.dev"
HOST = "trick.noremac.dev"
KEY_FILE = Path.home() / ".config/porkbun/credentials.json"


def credentials():
    directory = KEY_FILE.parent
    if directory.is_symlink() or KEY_FILE.is_symlink():
        raise ValueError("credential path must not be a symlink")
    dstat, fstat = directory.stat(), KEY_FILE.stat()
    if dstat.st_uid != os.getuid() or stat.S_IMODE(dstat.st_mode) != 0o700:
        raise ValueError("credential directory must be owned by this user and mode 700")
    if fstat.st_uid != os.getuid() or stat.S_IMODE(fstat.st_mode) != 0o600:
        raise ValueError("credential file must be owned by this user and mode 600")
    with KEY_FILE.open(encoding="utf-8") as stream:
        data = json.load(stream)
    if set(data) != {"apikey", "secretapikey"}:
        raise ValueError("credential JSON must contain only apikey and secretapikey")
    if not all(isinstance(data[k], str) and data[k].strip() for k in data):
        raise ValueError("both Porkbun keys are required")
    return data


def api_request(keys, path, body=None):
    headers = {
        "Accept": "application/json",
        "X-API-Key": keys["apikey"],
        "X-Secret-API-Key": keys["secretapikey"],
    }
    if body is not None:
        headers["Content-Type"] = "application/json"
        headers["Idempotency-Key"] = str(uuid.uuid4())
    request = urllib.request.Request(
        API + path,
        data=None if body is None else json.dumps(body).encode("utf-8"),
        headers=headers,
        method="GET" if body is None else "POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            result = json.load(response)
    except urllib.error.HTTPError as error:
        raise RuntimeError(f"Porkbun API returned HTTP {error.code}") from None
    if result.get("status") != "SUCCESS":
        raise RuntimeError(f"Porkbun API error: {result.get('code', 'unknown')}")
    for warning in result.get("warnings", []):
        print(f"Porkbun warning: {warning}", file=sys.stderr)
    return result


def trick_records(records):
    relevant = [
        record for record in records
        if str(record.get("name", "")).rstrip(".").lower() in {"trick", HOST}
        and record.get("type") in {"A", "AAAA", "CNAME"}
    ]
    a_records = [record for record in relevant if record["type"] == "A"]
    if len(a_records) != 1 or len(relevant) != 1:
        raise ValueError("expected exactly one Trick A record and no Trick AAAA/CNAME")
    record = a_records[0]
    if not str(record.get("id", "")).isdigit():
        raise ValueError("Trick A record has no numeric ID")
    ipaddress.IPv4Address(record.get("content", ""))
    return record


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["status", "set"])
    parser.add_argument("address", nargs="?", help="new IPv4 address for set")
    parser.add_argument("--expected-current", help="refuse an update if the record changed")
    args = parser.parse_args()
    if args.action == "set":
        if not args.address or not args.expected_current:
            parser.error("set requires ADDRESS and --expected-current ADDRESS")
        desired = str(ipaddress.IPv4Address(args.address))
        expected = str(ipaddress.IPv4Address(args.expected_current))
    elif args.address or args.expected_current:
        parser.error("status takes no address")
    keys = credentials()
    record = trick_records(api_request(keys, f"/dns/retrieve/{DOMAIN}")["records"])
    current = record["content"]
    print(f"{HOST} A record {record['id']}: {current}")
    if args.action == "status":
        return
    if current != expected:
        raise ValueError(f"record changed: expected {expected}, found {current}; no update made")
    if current == desired:
        print("already at requested address; no update made")
        return
    api_request(keys, f"/dns/edit/{DOMAIN}/{record['id']}", {"content": desired})
    updated = trick_records(api_request(keys, f"/dns/retrieve/{DOMAIN}")["records"])
    if updated["content"] != desired:
        raise RuntimeError("API accepted update but readback did not match")
    print(f"updated {HOST} A record {record['id']}: {current} -> {desired}")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, json.JSONDecodeError, RuntimeError) as error:
        print(f"DNS update failed: {error}", file=sys.stderr)
        sys.exit(1)
