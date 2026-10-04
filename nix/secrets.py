import os
import secrets
from pathlib import Path
from authlib.jose import JsonWebKey
import base64
import json

os.umask(0o077)
root = Path("/var/lib/care")
root.mkdir(parents=True, exist_ok=True)
config = root / "runtime.env"
if not config.exists():
    key = JsonWebKey.generate_key("RSA", 2048, is_private=True)
    jwks = base64.b64encode(json.dumps({"keys": [key.as_dict(key.dumps_private_key(), alg="RS256")]}).encode()).decode()
    values = {"DJANGO_SECRET_KEY": secrets.token_urlsafe(48), "JWKS_BASE64": jwks,
              "BUCKET_KEY": "care", "BUCKET_SECRET": secrets.token_urlsafe(48)}
    temp = root / "runtime.env.new"
    temp.write_text("".join(f"{k}={v}\n" for k, v in values.items()))
    temp.replace(config)
values = dict(line.split("=", 1) for line in config.read_text().splitlines())
(root / "minio.env").write_text(f"MINIO_ROOT_USER={values['BUCKET_KEY']}\nMINIO_ROOT_PASSWORD={values['BUCKET_SECRET']}\n")
