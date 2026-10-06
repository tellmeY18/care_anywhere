{ pkgs, care, frontend }:
let
  lib = pkgs.lib;
  postgres = pkgs.postgresql_17;
  python = import ./python.nix { inherit pkgs care; };
  web = import ./frontend.nix { inherit pkgs frontend; };
  native = with pkgs; [ stdenv.cc.cc.lib libpq gmp file glib pango harfbuzz fontconfig freetype cairo ];
  runtime = pkgs.linkFarm "care-runtime" ([{ name = "python"; path = python; }] ++
    lib.imap0 (i: path: { name = "lib-${toString i}"; inherit path; }) native);
  source = pkgs.runCommand "care-source" { nativeBuildInputs = [ pkgs.gettext ]; } ''
    cp -R ${care} $out
    chmod -R u+w $out
    substituteInPlace $out/config/settings/base.py \
      --replace-fail 'STATIC_ROOT = str(BASE_DIR / "staticfiles")' 'STATIC_ROOT = "/var/lib/care/staticfiles"' \
      --replace-fail 'MEDIA_ROOT = str(APPS_DIR / "media")' 'MEDIA_ROOT = "/var/lib/care/media"'
    substituteInPlace $out/config/settings/deployment.py \
      --replace-fail 'env("JWKS_BASE64", default=get_jwks_from_file(BASE_DIR))' 'env("JWKS_BASE64")'
    find $out/locale -name '*.po' -exec sh -c 'msgfmt "$1" -o "''${1%.po}.mo"' sh {} \;
  '';
  env = {
    DJANGO_SETTINGS_MODULE = "config.settings.deployment";
    DATABASE_URL = "postgres:///care?host=/run/postgresql";
    REDIS_URL = "redis://127.0.0.1:6379/0";
    CELERY_BROKER_URL = "redis://127.0.0.1:6379/1";
    DJANGO_ALLOWED_HOSTS = ''["127.0.0.1","localhost","guest"]'';
    DJANGO_SECURE_SSL_REDIRECT = "False";
    LD_LIBRARY_PATH = lib.makeLibraryPath native;
    FONTCONFIG_FILE = "/etc/fonts/fonts.conf";
    PYTHONDONTWRITEBYTECODE = "1";
    PYTHONUNBUFFERED = "1";
    BUCKET_REGION = "us-east-1";
    BUCKET_ENDPOINT = "http://127.0.0.1:9100";
    BUCKET_EXTERNAL_ENDPOINT = "http://127.0.0.1:8484";
    FILE_UPLOAD_BUCKET = "patient-bucket";
    FACILITY_S3_BUCKET = "facility-bucket";
  };
  seed = import ./database-seed.nix { inherit pkgs python env; app = source; };
  command = name: body: pkgs.writeShellScriptBin name ''
    set -euo pipefail
    cd ${source}
    ${lib.concatStringsSep "\n" (lib.mapAttrsToList (k: v: "export ${k}=${lib.escapeShellArg v}") env)}
    ${body}
  '';
  scripts = [
    (command "init" ''
      tables=$(${postgres}/bin/psql "$DATABASE_URL" -Atc "SELECT count(*) FROM pg_tables WHERE schemaname = 'public'")
      if [ "$tables" = 0 ]; then
        ${postgres}/bin/pg_restore --dbname="$DATABASE_URL" --no-owner --no-privileges --single-transaction --exit-on-error ${seed}/empty.dump
      fi
      ${python}/bin/python manage.py migrate --noinput
      ${python}/bin/python manage.py sync_permissions_roles
      ${python}/bin/python manage.py sync_valueset
      ${python}/bin/python manage.py collectstatic --noinput
    '')
    (command "api" ''exec ${python}/bin/gunicorn config.wsgi:application --bind 127.0.0.1:9000 --workers=2 --timeout=300 --worker-tmp-dir=/run/care-api'')
    (command "worker" ''exec ${python}/bin/celery -A config.celery_app worker --concurrency=1 --loglevel=info'')
    (command "beat" ''exec ${python}/bin/celery -A config.celery_app beat --schedule=/var/lib/care/beat --loglevel=info'')
    (command "secrets" ''exec ${python}/bin/python ${./secrets.py}'')
    (command "buckets" ''exec ${python}/bin/python ${./buckets.py}'')
    (command "admin" ''exec ${python}/bin/python -c 'import json,sys,django; django.setup(); from django.contrib.auth import get_user_model; d=json.load(sys.stdin); U=get_user_model(); assert not U.objects.filter(is_superuser=True).exists(), "Administrator already exists"; U.objects.create_superuser(username=d["Username"],email="",password=d["Password"])' '')
    (command "admin-reset" ''exec ${python}/bin/python -c 'import json,sys,django; django.setup(); from django.contrib.auth import get_user_model; d=json.load(sys.stdin); U=get_user_model(); u=U.objects.get(username=d["Username"],is_superuser=True); u.set_password(d["Password"]); u.save()' '')
  ];
  caddy = pkgs.writeText "care-caddy" ''
    handle /api/* { reverse_proxy 127.0.0.1:9000 }
    handle /ping/* { reverse_proxy 127.0.0.1:9000 }
    handle /static/* { reverse_proxy 127.0.0.1:9000 }
    handle /patient-bucket/* { reverse_proxy 127.0.0.1:9100 }
    handle /facility-bucket/* { reverse_proxy 127.0.0.1:9100 }
    handle {
      root * ${web}
      try_files {path} /index.html
      file_server
    }
  '';
in
{
  inherit runtime;
  app = pkgs.symlinkJoin {
    name = "care-app";
    paths = scripts;
    postBuild = ''ln -s ${caddy} $out/Caddyfile'';
  };
}
