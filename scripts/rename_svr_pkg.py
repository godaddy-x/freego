from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def repl_pkg(dir_rel, old, new):
    d = ROOT / dir_rel
    for p in d.glob("*.go"):
        t = p.read_text(encoding="utf-8")
        t = t.replace("package " + old, "package " + new, 1)
        p.write_text(t, encoding="utf-8")
        print("pkg", p.relative_to(ROOT))


def main():
    repl_pkg("server/http", "http", "httpsvr")
    repl_pkg("server/ws", "ws", "wssvr")

    old_imp = '"github.com/godaddy-x/freego/server/http"'
    new_imp = 'httpsvr "github.com/godaddy-x/freego/server/http"'
    for p in ROOT.rglob("*.go"):
        if "server" in p.parts and "http" in p.parts and p.parent.name == "http":
            continue
        t = p.read_text(encoding="utf-8")
        if old_imp not in t:
            continue
        if "httpsvr " + old_imp in t or 'httpsvr "github.com/godaddy-x/freego/server/http"' in t:
            continue
        t = t.replace(old_imp, new_imp)
        p.write_text(t, encoding="utf-8")
        print("imp", p.relative_to(ROOT))


if __name__ == "__main__":
    main()
