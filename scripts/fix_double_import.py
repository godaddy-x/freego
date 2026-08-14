from pathlib import Path

roots = [
    Path(r"e:\work\coding\open_gateway"),
    Path(r"e:\work\coding\open_scanner"),
    Path(r"e:\work\github\wallet-mpc-broker"),
    Path(r"e:\work\github\wallet-mpc-node"),
]
n = 0
for root in roots:
    for p in root.rglob("*.go"):
        if any(x in p.parts for x in (".git", "vendor", "node_modules")):
            continue
        t = p.read_text(encoding="utf-8")
        nt = t.replace("import import ", "import ")
        if nt != t:
            p.write_text(nt, encoding="utf-8")
            n += 1
print("fixed", n)
