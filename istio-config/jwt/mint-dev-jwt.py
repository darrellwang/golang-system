#!/usr/bin/env python3
"""Mint a short-lived RS256 JWT for the golang-system-dev issuer."""

from __future__ import annotations

import argparse
import base64
import json
import time
from pathlib import Path

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import padding

HERE = Path(__file__).resolve().parent
PRIVATE_KEY = HERE / "dev-private.pem"
KID = "rOxTWJqbhHs"
ISSUER = "golang-system-dev"
AUDIENCE = "golang-system"


def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--sub", default="dev-user")
    parser.add_argument("--org", default="dev-org", dest="org_id")
    parser.add_argument("--email", default="dev@example.com")
    parser.add_argument("--ttl", type=int, default=3600, help="seconds")
    args = parser.parse_args()

    now = int(time.time())
    header = {"alg": "RS256", "typ": "JWT", "kid": KID}
    payload = {
        "iss": ISSUER,
        "aud": AUDIENCE,
        "sub": args.sub,
        "org_id": args.org_id,
        "email": args.email,
        "iat": now,
        "exp": now + args.ttl,
    }
    signing_input = f"{b64url(json.dumps(header, separators=(',', ':')).encode())}." \
                    f"{b64url(json.dumps(payload, separators=(',', ':')).encode())}"
    key = serialization.load_pem_private_key(PRIVATE_KEY.read_bytes(), password=None)
    sig = key.sign(signing_input.encode(), padding.PKCS1v15(), hashes.SHA256())
    print(f"{signing_input}.{b64url(sig)}")


if __name__ == "__main__":
    main()
