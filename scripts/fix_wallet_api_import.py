from pathlib import Path
import re

root = Path(r"e:\work\github\wallet-api-go")
pat = re.compile(
    r'^((?!import)[A-Za-z_][A-Za-z0-9_]*) ("github.com/godaddy-x/freego/[^"]+")\s*$',
    re.M,
)
n = 0
for p in root.rglob("*.go"):
    t = p.read_text(encoding="utf-8")
    nt = pat.sub(r"import \1 \2", t)
    if nt != t:
        p.write_text(nt, encoding="utf-8")
        n += 1
        print(p)
print("fixed", n)
