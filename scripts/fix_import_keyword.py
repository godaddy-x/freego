from pathlib import Path
import re

roots = [
    Path(r"e:\work\coding\open_gateway"),
    Path(r"e:\work\coding\open_scanner"),
    Path(r"e:\work\github\wallet-mpc-broker"),
    Path(r"e:\work\github\wallet-mpc-node"),
]
pat = re.compile(
    r'^((?!import)[A-Za-z_][A-Za-z0-9_]*) ("github.com/godaddy-x/freego/[^"]+")\s*$',
    re.M,
)
n = 0
for root in roots:
    for p in root.rglob("*.go"):
        if any(x in p.parts for x in (".git", "vendor", "node_modules")):
            continue
        t = p.read_text(encoding="utf-8")
        nt = pat.sub(r"import \1 \2", t)
        if nt != t:
            p.write_text(nt, encoding="utf-8")
            n += 1
            print(p)
print("fixed", n)
