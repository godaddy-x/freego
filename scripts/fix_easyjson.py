from pathlib import Path

root = Path(r"e:\work\coding\open_gateway")
old = 'dialect "github.com/godaddy-x/freego/ormx/sqld/dialect"'
new = 'sqlc "github.com/godaddy-x/freego/core/query"'
n = 0
for p in root.rglob("*_easyjson.go"):
    t = p.read_text(encoding="utf-8")
    nt = t.replace(old, new).replace("dialect.PageResult", "sqlc.PageResult")
    if "github.com/godaddy-x/freego/ormx" in nt or "github.com/godaddy-x/freego/node" in nt:
        print("STILL OLD", p)
    if nt != t:
        p.write_text(nt, encoding="utf-8")
        n += 1
        print("patched", p)
print("patched", n)
