#!/usr/bin/env python3
"""Rewrite consumer imports from old fat freego paths to the split layout."""

from __future__ import annotations

import re
import sys
from pathlib import Path

SKIP_DIRS = {".git", "vendor", "node_modules", ".idea", "dist", "build"}

SIMPLE_PATHS = [
    ("github.com/godaddy-x/freego/utils/envoverlay", "github.com/godaddy-x/freego/core/envoverlay"),
    ("github.com/godaddy-x/freego/utils/crypto", "github.com/godaddy-x/freego/core/crypto"),
    ("github.com/godaddy-x/freego/utils/jwt", "github.com/godaddy-x/freego/core/jwt"),
    ("github.com/godaddy-x/freego/utils/gauth", "github.com/godaddy-x/freego/core/gauth"),
    ("github.com/godaddy-x/freego/utils/decimal", "github.com/godaddy-x/freego/core/decimal"),
    ("github.com/godaddy-x/freego/utils/concurrent", "github.com/godaddy-x/freego/core/concurrent"),
    ("github.com/godaddy-x/freego/geetest/sdk", "github.com/godaddy-x/freego/server/geetest/sdk"),
    ("github.com/godaddy-x/freego/geetest", "github.com/godaddy-x/freego/server/geetest"),
    ("github.com/godaddy-x/freego/ormx/sqlc", "github.com/godaddy-x/freego/core/query"),
    ("github.com/godaddy-x/freego/amqp", "github.com/godaddy-x/freego/store/amqp"),
    ("github.com/godaddy-x/freego/rpcx", "github.com/godaddy-x/freego/server/rpc"),
    ("github.com/godaddy-x/freego/yaml", "github.com/godaddy-x/freego/infra/config"),
    ("github.com/godaddy-x/freego/zlog", "github.com/godaddy-x/freego/infra/zlog"),
    ("github.com/godaddy-x/freego/ex", "github.com/godaddy-x/freego/core/ex"),
    ("github.com/godaddy-x/freego/gc", "github.com/godaddy-x/freego/infra/gc"),
    ("github.com/godaddy-x/freego/job", "github.com/godaddy-x/freego/server/job"),
    ("github.com/godaddy-x/freego/goquery", "github.com/godaddy-x/freego/server/goquery"),
    ("github.com/godaddy-x/freego/common", "github.com/godaddy-x/freego/core/const"),
    ("github.com/godaddy-x/freego/utils", "github.com/godaddy-x/freego/core/str"),
]

WIRE_NODE = {"JsonBody", "JsonResp", "PublicKey", "PrivateKey", "AuthToken"}
CACHE_REDIS = (
    "NewDefaultLocalCache",  # placeholder to keep order; local handled separately
)
CACHE_LOCAL_IDS = ["NewDefaultLocalCache", "NewLocalCache"]
CACHE_REDIS_IDS = [
    "ShutdownAllRedisManagers",
    "RedisManager",
    "RedisConfig",
    "LockConfig",
    "TryLocker",
    "NewRedis",
    "Lock",
]
CACHE_CONTRACT_IDS = ["CacheManager", "PutObj", "Cache", "LOCAL", "REDIS"]

SDK_WS_IDS = ["NewSocketSDK", "SocketSDK"]
SDK_GRPC_IDS = ["NewRPC", "RPC"]
SDK_HTTP_IDS = ["NewHttpSDK", "HttpSDK"]
SDK_WIRE_IDS = ["AuthToken"]

IMPORT_LINE = re.compile(
    r'^([ \t]*)(?:([A-Za-z_][A-Za-z0-9_]*)\s+)?"(github.com/godaddy-x/freego/[^"]+)"\s*$',
    re.M,
)
IDENT = re.compile(r"\b(?:cache|sdk|node|sqld)\.([A-Za-z_][A-Za-z0-9_]*)")


def iter_go_files(root: Path):
    for p in root.rglob("*.go"):
        if any(part in SKIP_DIRS for part in p.parts):
            continue
        yield p


def used_idents(text: str, pkg: str) -> set[str]:
    return set(re.findall(rf"\b{pkg}\.([A-Za-z_][A-Za-z0-9_]*)", text))


def is_ws_file(path: Path, text: str) -> bool:
    hints = (
        "ConnectionContext",
        "NewWsServer",
        "WsServer",
        "SubjectDeviceUnique",
        "ConnectionManager",
        "AddWsPreFilter",
    )
    if any(h in text for h in hints):
        return True
    s = str(path).replace("\\", "/")
    if "/api_main/" in s:
        return True
    if "websocket" in s.lower():
        return True
    return False


def replace_import_path(text: str, old: str, new: str, force_alias: str | None = None) -> str:
    def sub(m: re.Match) -> str:
        indent, alias, path = m.group(1), m.group(2), m.group(3)
        if path != old:
            return m.group(0)
        a = force_alias if force_alias is not None else alias
        if a:
            return f'{indent}{a} "{new}"'
        return f'{indent}"{new}"'

    return IMPORT_LINE.sub(sub, text)


def replace_import_with_block(text: str, old_path: str, new_lines: list[str]) -> str:
    def sub(m: re.Match) -> str:
        indent, _alias, path = m.group(1), m.group(2), m.group(3)
        if path != old_path:
            return m.group(0)
        return "\n".join(indent + line for line in new_lines)

    return IMPORT_LINE.sub(sub, text)


def rewrite_file(path: Path) -> bool:
    text = path.read_text(encoding="utf-8")
    orig = text

    cache_ids = used_idents(text, "cache")
    sdk_ids = used_idents(text, "sdk")
    node_ids = used_idents(text, "node")
    has_sqld = "github.com/godaddy-x/freego/ormx/sqld" in text
    has_cache_imp = "github.com/godaddy-x/freego/cache" in text
    has_sdk_imp = "github.com/godaddy-x/freego/utils/sdk" in text
    has_node_imp = re.search(r'"github.com/godaddy-x/freego/node"', text) is not None
    has_common_imp = "github.com/godaddy-x/freego/node/common" in text

    if has_common_imp:
        text = replace_import_path(
            text,
            "github.com/godaddy-x/freego/node/common",
            "github.com/godaddy-x/freego/protocol/dto",
            force_alias="common",
        )

    if has_sqld:
        text = replace_import_path(
            text,
            "github.com/godaddy-x/freego/ormx/sqld",
            "github.com/godaddy-x/freego/store/orm/mongo",
            force_alias="mongo",
        )
        text = text.replace("sqld.", "mongo.")

    if has_cache_imp:
        lines = []
        need_contract = bool(cache_ids & set(CACHE_CONTRACT_IDS))
        need_local = bool(cache_ids & set(CACHE_LOCAL_IDS))
        need_redis = bool(cache_ids & set(CACHE_REDIS_IDS))
        if not cache_ids:
            need_contract = True
        if need_contract:
            lines.append('cache "github.com/godaddy-x/freego/infra/cache/contract"')
        if need_local:
            lines.append('"github.com/godaddy-x/freego/infra/cachelocal"')
        if need_redis:
            lines.append('"github.com/godaddy-x/freego/store/cacheredis"')
        if not lines:
            lines.append('cache "github.com/godaddy-x/freego/infra/cache/contract"')
        text = replace_import_with_block(text, "github.com/godaddy-x/freego/cache", lines)
        for ident in CACHE_LOCAL_IDS:
            text = text.replace(f"cache.{ident}", f"cachelocal.{ident}")
        for ident in CACHE_REDIS_IDS:
            text = text.replace(f"cache.{ident}", f"cacheredis.{ident}")

    if has_sdk_imp:
        lines = []
        if sdk_ids & set(SDK_WS_IDS):
            lines.append('"github.com/godaddy-x/freego/client/ws"')
        if sdk_ids & set(SDK_GRPC_IDS):
            lines.append('grpcx "github.com/godaddy-x/freego/client/grpcx"')
        if sdk_ids & set(SDK_HTTP_IDS):
            lines.append('httpx "github.com/godaddy-x/freego/client/http"')
        if sdk_ids & set(SDK_WIRE_IDS):
            lines.append('"github.com/godaddy-x/freego/protocol/wire"')
        if not lines:
            lines.append('"github.com/godaddy-x/freego/client/ws"')
        text = replace_import_with_block(text, "github.com/godaddy-x/freego/utils/sdk", lines)
        text = text.replace("sdk.NewSocketSDK", "ws.New")
        text = text.replace("sdk.SocketSDK", "ws.SDK")
        text = text.replace("sdk.NewRPC", "grpcx.New")
        text = text.replace("sdk.RPC", "grpcx.RPC")
        text = text.replace("sdk.NewHttpSDK", "httpx.New")
        text = text.replace("sdk.HttpSDK", "httpx.SDK")
        text = text.replace("sdk.AuthToken", "wire.AuthToken")

    if has_node_imp:
        ws = is_ws_file(path, orig)
        wire_used = bool(node_ids & WIRE_NODE)
        lines = []
        if ws:
            lines.append('wssvr "github.com/godaddy-x/freego/server/ws"')
        else:
            lines.append('httpsvr "github.com/godaddy-x/freego/server/http"')
        if wire_used:
            lines.append('"github.com/godaddy-x/freego/protocol/wire"')
        text = replace_import_with_block(text, "github.com/godaddy-x/freego/node", lines)
        if wire_used:
            for ident in sorted(WIRE_NODE, key=len, reverse=True):
                text = text.replace(f"node.{ident}", f"wire.{ident}")
        if ws:
            text = text.replace("node.", "wssvr.")
        else:
            text = text.replace("node.", "httpsvr.")

    for old, new in SIMPLE_PATHS:
        if old in text:
            text = text.replace(f'"{old}"', f'"{new}"')

    if text != orig:
        path.write_text(text, encoding="utf-8")
        return True
    return False


def fix_socket_test(root: Path) -> None:
    p = root / "socket_test.go"
    if not p.exists():
        return
    t = p.read_text(encoding="utf-8")
    t = t.replace("wssvr.(ws.SubjectDeviceUnique)", "wssvr.NewWsServer(wssvr.SubjectDeviceUnique)")
    t = t.replace("ws.ConnectionContext", "wssvr.ConnectionContext")
    t = t.replace("ws.RouterConfig", "wssvr.RouterConfig")
    t = t.replace("ws.DevConn", "wssvr.DevConn")
    t = t.replace("ws.SubjectDeviceUnique", "wssvr.SubjectDeviceUnique")
    p.write_text(t, encoding="utf-8")


def main() -> None:
    if len(sys.argv) < 2:
        print("usage: rewrite_consumers.py <dir>...")
        sys.exit(2)
    n = 0
    for arg in sys.argv[1:]:
        root = Path(arg)
        if root.name == "freego" or (root / "socket_test.go").exists():
            fix_socket_test(root)
        for f in iter_go_files(root):
            if rewrite_file(f):
                n += 1
                print(f"updated {f}")
    print(f"changed {n} files")


if __name__ == "__main__":
    main()
