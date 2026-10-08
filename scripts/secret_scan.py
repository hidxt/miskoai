"""Development-only secret gate. Prints file/line/rule, never secret contents.

Scans git-visible working files, staged content and optionally every history tree.
Use a dedicated audited scanner as an additional release gate; this is a narrow
project policy scanner, not a guarantee that no possible secret exists.
"""
import argparse
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
RULES = {
    "private-key": re.compile(rb"-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----"),
    "provider-key": re.compile(rb"\bsk-[A-Za-z0-9_-]{20,}\b"),
    "github-token": re.compile(rb"\b(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,})\b"),
    "aws-access": re.compile(rb"\b(?:AKIA|ASIA)[A-Z0-9]{16}\b"),
    "credential-value": re.compile(rb'(?i)(?:api[_-]?key|bot[_-]?token|admin[_-]?password|session[_-]?token)\s*[=:]\s*[\"\x27]?([A-Za-z0-9_./+\-=]{16,})'),
}
FORBIDDEN = re.compile(r"(?i)(?:^|/)(?:\.env(?:\..+)?|[^/]+\.(?:db|sqlite3?|pem|key|p12)|[^/]*(?:credentials|session)[^/]*\.json)$")


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)


def scan(label, data):
    found = []
    for rule, expression in RULES.items():
        for match in expression.finditer(data):
            value = match.group(1) if rule == "credential-value" else match.group(0)
            if value.startswith((b"example-", b"synthetic", b"fixture", b"not-a-real")):
                continue
            found.append(f"{label}:{data.count(bytes([10]), 0, match.start()) + 1}: {rule}")
    return found


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--history", action="store_true")
    parser.add_argument("--staged", action="store_true")
    opts = parser.parse_args()
    failures = []
    count = 0
    if opts.staged:
        names = git("diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z").split(b"\0")
    else:
        names = git("ls-files", "--cached", "--others", "--exclude-standard", "-z").split(b"\0")
    for raw in set(names):
        if not raw:
            continue
        name = raw.decode("utf-8")
        if name != ".env.example" and FORBIDDEN.search(name):
            failures.append(f"{name}: forbidden public artifact")
        path = ROOT / name
        if opts.staged:
            data = git("show", ":" + name)
        elif path.is_file():
            data = path.read_bytes()
        else:
            continue
        failures.extend(scan(name, data))
        count += 1
    if opts.history:
        # Visit each immutable blob once; filenames and deleted history are covered.
        seen = set()
        for line in git("rev-list", "--objects", "--all").splitlines():
            parts = line.split(b" ", 1)
            if len(parts) != 2 or parts[0] in seen:
                continue
            oid, rawname = parts
            seen.add(oid)
            if git("cat-file", "-t", oid.decode()).strip() != b"blob":
                continue
            name = rawname.decode("utf-8", "replace")
            if name != ".env.example" and FORBIDDEN.search(name):
                failures.append(f"history/{name}: forbidden public artifact")
            failures.extend(scan("history/" + name, git("cat-file", "blob", oid.decode())))
            count += 1
    for failure in sorted(set(failures)):
        print(failure, file=sys.stderr)
    print(f"Secret policy scan: {count} contents, {len(set(failures))} findings (values redacted)")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
