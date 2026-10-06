{ config, lib, pkgs, self, care, frontend, minioPackage, modulesPath, ... }:
let
  layers = import ./app.nix { inherit pkgs care frontend; };
  postgres = pkgs.postgresql_17.override { jitSupport = false; };
  agent = pkgs.buildGoModule {
    pname = "care-anywhere-agent";
    version = "0.2.0";
    src = lib.fileset.toSource {
      root = ../.;
      fileset = lib.fileset.unions [
        (lib.fileset.fileFilter (file: file.hasExt "go") ../.)
        ../go.mod
        ../go.sum
      ];
    };
    vendorHash = "sha256-eU5/KvEy1gyFXFCQDJ39u3hhog7nQNpHgMvjlC7HkGM=";
    env.CGO_ENABLED = "0";
    ldflags = [ "-s" "-w" ];
    doCheck = false;
  };
  service = name: {
    wantedBy = [ "multi-user.target" ];
    requires = [ "care-init.service" ];
    after = [ "care-init.service" ];
    serviceConfig = {
      User = "care";
      Group = "care";
      EnvironmentFile = "/var/lib/care/runtime.env";
      ExecStart = "/run/care/app/bin/${name}";
      Restart = "on-failure";
      RestartSec = 5;
      TimeoutStopSec = 120;
      NoNewPrivileges = true;
      PrivateTmp = true;
    };
  };
  admin = name: pkgs.writeShellScriptBin "care-${name}" ''
    set -euo pipefail
    set -a
    source /var/lib/care/runtime.env
    exec ${pkgs.util-linux}/bin/runuser -u care -- /run/care/app/bin/${name}
  '';
  layerMount = id: {
    device = "/dev/disk/by-id/virtio-care-${id}";
    fsType = "erofs";
    options = [ "ro" ];
    neededForBoot = true;
    noCheck = true;
  };
in
{
  imports = [ "${modulesPath}/profiles/minimal.nix" "${modulesPath}/profiles/headless.nix" ];
  system.stateVersion = "25.05";
  system.build.careApp = layers.app;
  system.build.careRuntime = layers.runtime;
  networking.hostName = "care-anywhere";
  nix.enable = false;
  system.switch.enable = false;
  documentation.enable = false;
  environment.defaultPackages = [ ];
  security.sudo.enable = false;
  microvm = {
    hypervisor = "qemu";
    mem = 4096;
    vcpu = 2;
    storeOnDisk = true;
    storeDiskType = "erofs";
    storeDiskErofsFlags = [ "-zlz4hc" "-Eztailpacking" "-Efragments" "-Ededupe" ];
    volumes = [{ image = "data.img"; mountPoint = "/var/lib"; size = 8192; }];
  };
  fileSystems."/nix/.base-store" = layerMount "base";
  fileSystems."/nix/.runtime-store" = layerMount "runtime";
  fileSystems."/nix/.app-store" = layerMount "app";
  fileSystems."/nix/store" = lib.mkForce {
    neededForBoot = true;
    overlay.lowerdir = [ "/nix/.app-store" "/nix/.runtime-store" "/nix/.base-store" ];
  };
  fileSystems."/var/lib".device = lib.mkForce "/dev/disk/by-id/virtio-care-data";
  fileSystems."/var/lib".autoResize = true;
  fileSystems."/var/lib".neededForBoot = true;
  boot.initrd.supportedFilesystems = [ "erofs" "overlay" "ext4" ];
  boot.kernelModules = [ "vmw_vsock_virtio_transport" "virtio_net" "qemu_fw_cfg" "virtio_balloon" ];
  networking.useDHCP = true;
  networking.firewall.enable = true;
  networking.firewall.allowedTCPPorts = [ 8080 ];
  users.groups.care = { };
  users.users.care = { isSystemUser = true; group = "care"; };
  services.postgresql = {
    enable = true;
    package = postgres;
    ensureDatabases = [ "care" ];
    ensureUsers = [{ name = "care"; ensureDBOwnership = true; }];
    authentication = lib.mkForce ''local all all peer'';
    settings.shared_buffers = "128MB";
  };
  services.redis.servers."" = { enable = true; port = 6379; };
  # Silo retains MinIO's on-disk format and API, with current security fixes.
  services.minio = {
    enable = true;
    listenAddress = "127.0.0.1:9100";
    rootCredentialsFile = "/var/lib/care/minio.env";
    package = minioPackage;
  };
  services.fstrim.enable = true;
  fonts = { fontconfig.enable = true; packages = [ pkgs.dejavu_fonts ]; };
  environment.systemPackages = [ (admin "admin") (admin "admin-reset") ];
  systemd.tmpfiles.rules = [ "d /run/care 0755 root root -" "L /run/care/app - - - - /nix/.app-store/entry" ];
  systemd.services.care-secrets = {
    requiredBy = [ "minio.service" "care-init.service" ];
    before = [ "minio.service" "care-init.service" ];
    after = [ "systemd-tmpfiles-setup.service" ];
    serviceConfig = { Type = "oneshot"; RemainAfterExit = true; StateDirectory = "care"; };
    script = ''
      /run/care/app/bin/secrets
      chown -R care:care /var/lib/care
    '';
  };
  systemd.services.care-init = {
    wantedBy = [ "multi-user.target" ];
    requires = [ "postgresql.target" "redis.service" "care-secrets.service" ];
    after = [ "postgresql.target" "redis.service" "care-secrets.service" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      User = "care";
      Group = "care";
      EnvironmentFile = "/var/lib/care/runtime.env";
      ExecStart = "/run/care/app/bin/init";
    };
  };
  systemd.services.care-api = lib.recursiveUpdate (service "api") { serviceConfig.RuntimeDirectory = "care-api"; };
  systemd.services.care-worker = service "worker";
  systemd.services.care-beat = service "beat";
  systemd.services.care-buckets = {
    wantedBy = [ "multi-user.target" ];
    requires = [ "minio.service" "care-secrets.service" ];
    after = [ "minio.service" "care-secrets.service" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      User = "care";
      Group = "care";
      EnvironmentFile = "/var/lib/care/runtime.env";
      ExecStart = "/run/care/app/bin/buckets";
    };
  };
  systemd.services.care-agent = {
    wantedBy = [ "multi-user.target" ];
    after = [ "local-fs.target" ];
    path = [ pkgs.systemd ];
    serviceConfig = { ExecStart = "${agent}/bin/care_anywhere guest"; Restart = "on-failure"; };
  };
  services.caddy = {
    enable = true;
    extraConfig = ''
      http://127.0.0.1:8081 {
        import /run/care/app/Caddyfile
      }
    '';
  };
}
