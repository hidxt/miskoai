"""Development-only structural build verification; not Linux execution/RSS."""
import hashlib
import json
import pathlib
import struct
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[1]
results = []
for name, machine in [("miskoai-linux-amd64", 62), ("miskoai-linux-arm64", 183)]:
    data = (ROOT / "dist" / name).read_bytes()
    assert data[:6] == b"\x7fELF\x02\x01", "expected ELF64 little endian"
    assert struct.unpack_from("<H", data, 18)[0] == machine, "wrong architecture"
    offset = struct.unpack_from("<Q", data, 32)[0]
    size, count = struct.unpack_from("<HH", data, 54)
    segments = [struct.unpack_from("<I", data, offset + i * size)[0] for i in range(count)]
    assert 2 not in segments and 3 not in segments, "unexpected dynamic loader/dependency"
    results.append({"file": name, "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest(),
                    "elf_machine": machine, "dynamic_loader": False})
binary = ROOT / "dist" / "miskoai-windows-amd64.exe"
data = binary.read_bytes()
pe_offset = struct.unpack_from("<I", data, 60)[0]
assert data[pe_offset:pe_offset+4] == b"PE\0\0"
assert struct.unpack_from("<H", data, pe_offset+4)[0] == 0x8664
assert subprocess.check_output([str(binary), "licenses"]) == (ROOT / "internal/notices/notices.txt").read_bytes(), "embedded license mismatch"
results.append({"file": binary.name, "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest(),
                "embedded_notices_exact": True})
print(json.dumps({"kind": "development cross-build evidence, not release/Linux runtime", "builds": results}, indent=2))
