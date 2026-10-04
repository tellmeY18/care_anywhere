"""Fill missing registry URLs/integrity in the pinned upstream lock, not versions."""
import json
import sys
import urllib.request
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor

lock = json.loads(Path(sys.argv[1]).read_text())
def fill(entry):
    path, item = entry
    if not path or "resolved" in item or item.get("link"):
        return
    name = item.get("name", path.rsplit("node_modules/", 1)[1])
    item["version"] = item["version"].removeprefix("v")
    try:
        data = json.load(urllib.request.urlopen(f"https://registry.npmjs.org/{name}/{item['version']}", timeout=30))
    except Exception as error:
        raise ValueError(f"{path}: {name}@{item['version']}: {error}") from error
    item["resolved"] = data["dist"]["tarball"]
    item["integrity"] = data["dist"]["integrity"]
with ThreadPoolExecutor(max_workers=12) as pool:
    list(pool.map(fill, lock["packages"].items()))
Path(sys.argv[2]).write_text(json.dumps(lock, indent=2) + "\n")
