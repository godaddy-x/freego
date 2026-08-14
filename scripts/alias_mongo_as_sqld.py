from pathlib import Path

ROOTS = [
    Path(r"e:\work\coding\open_gateway"),
    Path(r"e:\work\coding\open_scanner"),
    Path(r"e:\work\github\wallet-mpc-broker"),
    Path(r"e:\work\github\wallet-mpc-node"),
    Path(r"e:\work\github\wallet-api-go"),
]
OLD_IMP = 'mongo "github.com/godaddy-x/freego/store/orm/mongo"'
NEW_IMP = 'sqld "github.com/godaddy-x/freego/store/orm/mongo"'
IDENTS = [
    "UseTransactionWithContext",
    "UseTransaction",
    "RebuildMongoDBIndex",
    "EncodeObjectToBson",
    "DecodeBsonToObject",
    "ModelDriver",
    "ModelTime",
    "MongoClose",
    "NewMongo",
    "MGOManager",
    "MGOConfig",
    "PackContext",
    "DBManager",
    "DBConfig",
    "SortBy",
    "Option",
]


def rewrite(text: str) -> str:
    text = text.replace(OLD_IMP, NEW_IMP)
    for ident in IDENTS:
        text = text.replace(f"mongo.{ident}", f"sqld.{ident}")
    return text


n = 0
for root in ROOTS:
    for p in root.rglob("*.go"):
        if any(x in p.parts for x in (".git", "vendor", "node_modules")):
            continue
        t = p.read_text(encoding="utf-8")
        nt = rewrite(t)
        if nt != t:
            p.write_text(nt, encoding="utf-8")
            n += 1
print("updated", n)
