{ pkgs, guest }:
let
  cfg = guest.config;
  arch = if pkgs.stdenv.hostPlatform.isAarch64 then "aarch64" else "x86_64";
  firecracker = pkgs.fetchurl {
    url = "https://github.com/firecracker-microvm/firecracker/releases/download/v1.17.0/firecracker-v1.17.0-${arch}.tgz";
    sha256 = if pkgs.stdenv.hostPlatform.isAarch64 then "e351ebe4f7a16b5873bbd51005d2e6767103cff4d5ebc829df2d3f95a93e2256" else "06094a1108ae9e82aa4c23a775aa92758f53f1175d422270d9d6162cb9ade558";
  };
in
pkgs.runCommand "care-anywhere-bundle" { nativeBuildInputs = [ pkgs.e2fsprogs pkgs.python3 pkgs.gnutar pkgs.gzip ]; } ''
  mkdir -p $out
  cp ${if pkgs.stdenv.hostPlatform.isAarch64 then "${cfg.microvm.kernel}/Image" else "${cfg.microvm.kernel.dev}/vmlinux"} $out/kernel
  cp ${cfg.microvm.initrdPath} $out/initrd
  cp ${cfg.microvm.storeDisk} $out/system.img
  truncate -s 8G $out/data.img
  mkfs.ext4 -q -F $out/data.img
  tar -xzf ${firecracker}
  cp release-v1.17.0-${arch}/firecracker-v1.17.0-${arch} $out/firecracker
  chmod +x $out/firecracker
  export OUT=$out
  python - <<'PY'
  import os,json,hashlib,pathlib
  root=pathlib.Path(os.environ['OUT'])
  files={}
  for name in ['kernel','initrd','system.img','data.img','firecracker']:
      with (root/name).open('rb') as f: files[name]=hashlib.file_digest(f,'sha256').hexdigest()
  (root/'manifest.json').write_text(json.dumps(dict(version='0.1.0-preview',arch='${if pkgs.stdenv.hostPlatform.isAarch64 then "arm64" else "amd64"}',kernel='kernel',initrd='initrd',system='${cfg.system.build.toplevel}',files=files),indent=2))
  PY
''
