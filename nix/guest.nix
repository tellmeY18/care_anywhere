{ config, lib, pkgs, self, care, frontend, ... }:
let
  python = import ./python.nix { inherit pkgs care; };
  web = import ./frontend.nix { inherit pkgs frontend; };
  agent = pkgs.buildGoModule {
    pname = "care-anywhere-agent";
    version = "0.1.0-preview";
    src = lib.fileset.toSource {
      root = ../.;
      fileset = lib.fileset.unions [
        (lib.fileset.fileFilter (file: file.hasExt "go") ../.)
        ../go.mod ../go.sum
      ];
    };
    vendorHash = "sha256-eU5/KvEy1gyFXFCQDJ39u3hhog7nQNpHgMvjlC7HkGM=";
    env.CGO_ENABLED = "0";
    doCheck = false;
  };
  libs = lib.makeLibraryPath (with pkgs; [ stdenv.cc.cc.lib libpq gmp file glib pango harfbuzz fontconfig freetype cairo ]);
  app = pkgs.runCommand "care-app" { nativeBuildInputs = [ pkgs.gettext ]; } ''
    cp -R ${care} $out
    chmod -R u+w $out
    # Source is read-only at runtime; use explicit persistent/generated paths.
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
    LD_LIBRARY_PATH = libs;
    FONTCONFIG_FILE = "/etc/fonts/fonts.conf";
    PYTHONDONTWRITEBYTECODE = "1";
    PYTHONUNBUFFERED = "1";
    BUCKET_REGION = "us-east-1";
    BUCKET_ENDPOINT = "http://127.0.0.1:9100";
    BUCKET_EXTERNAL_ENDPOINT = "http://127.0.0.1:8484";
    FILE_UPLOAD_BUCKET = "patient-bucket";
    FACILITY_S3_BUCKET = "facility-bucket";
  };
  service = command: {
    wantedBy = [ "multi-user.target" ];
    requires = [ "care-init.service" ]; after = [ "care-init.service" ];
    environment = env;
    serviceConfig = {
      User = "care"; Group = "care"; WorkingDirectory = app;
      EnvironmentFile = "/var/lib/care/runtime.env";
      ExecStart = command; Restart = "on-failure"; RestartSec = 5;
      TimeoutStopSec = 120; NoNewPrivileges = true; PrivateTmp = true;
    };
  };
  admin = pkgs.writeShellScriptBin "care-admin" ''
    cd ${app}
    set -a
    source /var/lib/care/runtime.env
    ${lib.concatStringsSep "\n" (lib.mapAttrsToList (k: v: "export ${k}=${lib.escapeShellArg v}") env)}
    exec ${pkgs.util-linux}/bin/runuser -u care -- ${python}/bin/python -c 'import json,sys,django; django.setup(); from django.contrib.auth import get_user_model; d=json.load(sys.stdin); U=get_user_model(); assert not U.objects.filter(is_superuser=True).exists(), "Administrator already exists"; U.objects.create_superuser(username=d["Username"],email="",password=d["Password"])'
  '';
  adminReset = pkgs.writeShellScriptBin "care-admin-reset" ''
    cd ${app}
    set -a
    source /var/lib/care/runtime.env
    ${lib.concatStringsSep "\n" (lib.mapAttrsToList (k: v: "export ${k}=${lib.escapeShellArg v}") env)}
    exec ${pkgs.util-linux}/bin/runuser -u care -- ${python}/bin/python -c 'import json,sys,django; django.setup(); from django.contrib.auth import get_user_model; d=json.load(sys.stdin); U=get_user_model(); u=U.objects.get(username=d["Username"],is_superuser=True); u.set_password(d["Password"]); u.save()'
  '';
in {
  system.stateVersion = "25.05";
  networking.hostName = "care-anywhere";
  microvm = {
    hypervisor = "qemu";
    mem = 4096; vcpu = 2; storeOnDisk = true;
    volumes = [{ image = "data.img"; mountPoint = "/var/lib"; size = 8192; }];
  };
  boot.kernelModules = [ "vmw_vsock_virtio_transport" "virtio_net" ];
  # Local-first: the host attaches outbound-only NAT internet by default (see
  # vm_darwin.go) so features that need it — e.g. SNOMED lookups via the
  # Snowstorm terminology server — work. The guest DHCPs if a NIC appears and
  # is otherwise inert (no NIC => no traffic). The firewall below blocks all
  # unsolicited inbound traffic; nothing listens for the LAN or the internet.
  networking.useDHCP = true;
  networking.firewall.enable = true;
  users.groups.care = {};
  users.users.care = { isSystemUser = true; group = "care"; };
  services.postgresql = {
    enable = true; package = pkgs.postgresql_17;
    ensureDatabases = [ "care" ]; ensureUsers = [{ name = "care"; ensureDBOwnership = true; }];
    authentication = lib.mkForce ''local all all peer'';
  };
  services.redis.servers."" = { enable = true; port = 6379; };
  services.minio = {
    enable = true; listenAddress = "127.0.0.1:9100";
    rootCredentialsFile = "/var/lib/care/minio.env";
  };
  fonts = { fontconfig.enable = true; packages = [ pkgs.dejavu_fonts ]; };
  environment.systemPackages = [ admin adminReset ];
  systemd.services.care-secrets = {
    requiredBy = [ "minio.service" "care-init.service" ]; before = [ "minio.service" "care-init.service" ];
    serviceConfig = { Type = "oneshot"; RemainAfterExit = true; StateDirectory = "care"; };
    script = ''
      ${python}/bin/python ${./secrets.py}
      chown -R care:care /var/lib/care
    '';
  };
  systemd.services.care-init = {
    wantedBy = [ "multi-user.target" ];
    requires = [ "postgresql.target" "redis.service" "care-secrets.service" ];
    after = [ "postgresql.target" "redis.service" "care-secrets.service" ];
    environment = env;
    serviceConfig = { Type = "oneshot"; RemainAfterExit = true; User = "care"; Group = "care"; WorkingDirectory = app; EnvironmentFile = "/var/lib/care/runtime.env"; };
    script = ''
      ${python}/bin/python manage.py migrate --noinput
      ${python}/bin/python manage.py sync_permissions_roles
      ${python}/bin/python manage.py sync_valueset
      ${python}/bin/python manage.py collectstatic --noinput
    '';
  };
  systemd.services.care-api = service "${python}/bin/gunicorn config.wsgi:application --bind 127.0.0.1:9000 --workers=2";
  systemd.services.care-buckets = {
    wantedBy = [ "multi-user.target" ];
    requires = [ "minio.service" "care-secrets.service" ];
    after = [ "minio.service" "care-secrets.service" ];
    environment = env;
    serviceConfig = { Type = "oneshot"; RemainAfterExit = true; User = "care"; Group = "care"; EnvironmentFile = "/var/lib/care/runtime.env"; };
    script = ''
      ${python}/bin/python ${./buckets.py}
    '';
  };
  systemd.services.care-worker = service "${python}/bin/celery -A config.celery_app worker --concurrency=1 --loglevel=info";
  systemd.services.care-beat = service "${python}/bin/celery -A config.celery_app beat --schedule=/var/lib/care/beat --loglevel=info";
  systemd.services.care-agent = {
    wantedBy = [ "multi-user.target" ]; after = [ "local-fs.target" ];
    path = [ pkgs.systemd ];
    serviceConfig = { ExecStart = "${agent}/bin/care_anywhere guest"; Restart = "on-failure"; };
  };
  services.caddy = {
    enable = true;
    extraConfig = ''
      http://127.0.0.1:8081 {
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
      }
    '';
  };
}
