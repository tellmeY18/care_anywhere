{ pkgs, guest }:
let
  cfg = guest.config;
in
pkgs.runCommand "care-anywhere-bundle" { nativeBuildInputs = [ pkgs.e2fsprogs pkgs.python3 pkgs.gnutar pkgs.gzip ]; } ''
  mkdir -p $out
  cp ${cfg.microvm.kernel}/${pkgs.stdenv.hostPlatform.linux-kernel.target} $out/kernel
  cp ${cfg.microvm.initrdPath} $out/initrd
  cp ${cfg.microvm.storeDisk} $out/system.img
  truncate -s 8G $out/data.img
  mkfs.ext4 -q -F $out/data.img
  export OUT=$out
  python - <<'PY'
  import os,json,hashlib,pathlib
  root=pathlib.Path(os.environ['OUT'])
  files={}
  for name in ['kernel','initrd','system.img','data.img']:
      with (root/name).open('rb') as f: files[name]=hashlib.file_digest(f,'sha256').hexdigest()
  (root/'manifest.json').write_text(json.dumps(dict(version='0.1.0-preview',arch='${if pkgs.stdenv.hostPlatform.isAarch64 then "arm64" else "amd64"}',kernel='kernel',initrd='initrd',system='${cfg.system.build.toplevel}',files=files),indent=2))
  PY
''
