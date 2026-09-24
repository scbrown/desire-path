#!/usr/bin/env python3
"""Exercise the link checker's real CLI with valid and broken document links."""
import subprocess
import sys
import tempfile
from pathlib import Path

checker = Path(__file__).with_name("check-md-links.py")
with tempfile.TemporaryDirectory() as directory:
    source = Path(directory, "control.md")
    Path(directory, "exists.md").write_text("# Present\n")
    cases = [
        ("[present](exists.md#heading)\n", True),
        ("[missing](missing.md)\n", False),
        ('<img src="missing.svg">\n', False),
        ("[repo](https://github.com/scbrown/desire-path/blob/main/README.md)\n", True),
        ("[repo](https://github.com/scbrown/desire-path/blob/main/MISSING.md)\n", False),
        ("```md\n[example](missing.md)\n```\n", True),
    ]
    for content, expected in cases:
        source.write_text(content)
        result = subprocess.run([sys.executable, str(checker), str(source)], capture_output=True, text=True)
        assert (result.returncode == 0) == expected, (content, result.stdout, result.stderr)
print("check-md-links: 6 acceptance/refusal controls passed")
