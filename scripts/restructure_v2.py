# One-shot directory restructure helper. Run from repo root: python scripts/restructure_v2.py
import os
import shutil
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

# (src_rel, dst_rel) git-mv style copies for already-separate packages
MOVES = [
    ("ex", "core/ex"),
    ("zlog", "infra/zlog"),
    ("yaml", "infra/config"),
    ("amqp", "store/amqp"),
    ("job", "server/job"),
    ("goquery", "server/goquery"),
    ("gc", "infra/gc"),
    ("geetest", "server/geetest"),
    ("utils/crypto", "core/crypto"),
    ("utils/decimal", "core/decimal"),
    ("utils/concurrent", "core/concurrent"),
    ("utils/envoverlay", "core/envoverlay"),
    ("utils/snowflake", "core/snowflake"),
    ("utils/jwt", "core/jwt"),
    ("utils/gauth", "core/gauth"),
    ("common", "core/const"),
    ("ormx/sqlc", "core/query"),
    ("ormx/sqld/dialect", "core/query"),  # merge into query; change package dialect -> sqlc later
]


def copy_tree(src: Path, dst: Path):
    dst.parent.mkdir(parents=True, exist_ok=True)
    if src.is_file():
        shutil.copy2(src, dst)
        return
    if dst.exists():
        return
    shutil.copytree(src, dst)


def main():
    os.chdir(ROOT)
    for src, dst in MOVES:
        s, d = ROOT / src, ROOT / dst
        if not s.exists():
            print("skip missing", src)
            continue
        print("copy", src, "->", dst)
        if s.is_dir():
            d.mkdir(parents=True, exist_ok=True)
            for p in s.rglob("*"):
                if p.is_file():
                    rel = p.relative_to(s)
                    target = d / rel
                    # dialect files go into core/query root
                    if src == "ormx/sqld/dialect":
                        target = d / p.name
                    target.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copy2(p, target)
        else:
            copy_tree(s, d)

    # utils leaf files
    str_files = [
        "time.go",
        "gorand.go",
        "regular.go",
        "os.go",
        "common.go",
        "base64pool.go",
        "protocol_nonce.go",
    ]
    (ROOT / "core/str").mkdir(parents=True, exist_ok=True)
    for name in str_files:
        p = ROOT / "utils" / name
        if p.exists():
            shutil.copy2(p, ROOT / "core/str" / name)
            print("copy utils/" + name, "-> core/str")

    (ROOT / "core/codec").mkdir(parents=True, exist_ok=True)
    if (ROOT / "utils/jsonlib.go").exists():
        shutil.copy2(ROOT / "utils/jsonlib.go", ROOT / "core/codec/jsonlib.go")

    (ROOT / "core/reflect").mkdir(parents=True, exist_ok=True)
    for name in ["reflect_field.go", "reflect_value.go"]:
        p = ROOT / "utils" / name
        if p.exists():
            shutil.copy2(p, ROOT / "core/reflect" / name)

    if (ROOT / "utils/crypto_aes.go").exists():
        shutil.copy2(ROOT / "utils/crypto_aes.go", ROOT / "core/crypto/crypto_aes.go")

    print("done copies")


if __name__ == "__main__":
    main()
