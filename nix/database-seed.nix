{ pkgs, python, app, env }:
pkgs.runCommand "care-empty-database"
  (env // {
    nativeBuildInputs = [ pkgs.postgresql_17 ];
  }) ''
  export HOME=$TMPDIR
  export DATABASE_URL="postgres:///care?host=$TMPDIR&user=$(id -un)"
  # Build-only credentials: no runtime.env, private key, or administrator is
  # exported. Every installation generates its own secrets on first boot.
  export DJANGO_SECRET_KEY=build-only-empty-database
  export BUCKET_KEY=care
  export BUCKET_SECRET=build-only-not-shipped
  export JWKS_BASE64=$(${python}/bin/python -c 'import base64,json; from authlib.jose import JsonWebKey; k=JsonWebKey.generate_key("RSA",2048,is_private=True); print(base64.b64encode(json.dumps({"keys":[k.as_dict(k.dumps_private_key(),alg="RS256")]}).encode()).decode())')
  initdb -D "$TMPDIR/pg" --no-locale --encoding=UTF8
  pg_ctl -D "$TMPDIR/pg" -o "-k $TMPDIR -h \"\"" -w start
  trap 'pg_ctl -D "$TMPDIR/pg" -m immediate stop' EXIT
  createdb -h "$TMPDIR" care
  cd ${app}
  ${python}/bin/python manage.py migrate --noinput
  ${python}/bin/python - <<'PY'
  import django
  django.setup()
  from django.contrib.auth import get_user_model
  assert not get_user_model().objects.exists(), "Database seed must not contain users"
  PY
  mkdir -p $out
  pg_dump -h "$TMPDIR" --no-owner --no-privileges --format=custom care > "$out/empty.dump"
  # Verify the shipped dump restores atomically and covers every pinned migration.
  createdb -h "$TMPDIR" seedcheck
  pg_restore -h "$TMPDIR" --dbname=seedcheck --no-owner --no-privileges \
    --single-transaction --exit-on-error "$out/empty.dump"
  DATABASE_URL="postgres:///seedcheck?host=$TMPDIR&user=$(id -un)" \
    ${python}/bin/python manage.py migrate --check
  pg_ctl -D "$TMPDIR/pg" -m fast -w stop
  trap - EXIT
''
