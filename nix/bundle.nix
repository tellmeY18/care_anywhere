{ pkgs, guest }:
let
  cfg = guest.config;
  baseClosure = pkgs.closureInfo { rootPaths = [ cfg.system.build.toplevel ]; };
  runtimeClosure = pkgs.closureInfo { rootPaths = [ cfg.system.build.careRuntime ]; };
  appClosure = pkgs.closureInfo { rootPaths = [ cfg.system.build.careApp ]; };
in
pkgs.runCommand "care-anywhere-bundle" { nativeBuildInputs = [ pkgs.e2fsprogs pkgs.erofs-utils pkgs.python3 ]; } ''
  mkdir -p $out
  cp ${if pkgs.stdenv.hostPlatform.isAarch64 then "${cfg.microvm.kernel}/Image" else "${cfg.microvm.kernel.dev}/vmlinux"} $out/kernel
  cp ${cfg.microvm.initrdPath} $out/initrd
  cp ${cfg.microvm.storeDisk} $out/system.img
  # Ship only a small, formatted seed. The host extends its private copy sparsely;
  # the guest grows ext4 before mounting /var/lib, without ever formatting user data.
  truncate -s 64M $out/data.img
  mkfs.ext4 -q -F -L care-data $out/data.img
  sort ${baseClosure}/store-paths > base-paths
  sort ${runtimeClosure}/store-paths > runtime-paths
  sort ${appClosure}/store-paths > app-paths
  mkdir runtime app
  for path in $(comm -23 runtime-paths base-paths); do cp -a "$path" runtime/; done
  sort -u base-paths runtime-paths > shared-paths
  for path in $(comm -23 app-paths shared-paths); do cp -a "$path" app/; done
  ln -s ${cfg.system.build.careApp} app/entry
  mkfs.erofs -zlz4hc -Eztailpacking -Efragments -Ededupe -T 0 --all-root -L care-runtime $out/runtime.img runtime
  mkfs.erofs -zlz4hc -Eztailpacking -Efragments -Ededupe -T 0 --all-root -L care-app $out/app.img app
  export OUT=$out
  python - <<'PY'
  import os,json,hashlib,pathlib
  root=pathlib.Path(os.environ['OUT'])
  files={}
  for name in ['kernel','initrd','system.img','runtime.img','app.img','data.img']:
      with (root/name).open('rb') as f: files[name]=hashlib.file_digest(f,'sha256').hexdigest()
  (root/'manifest.json').write_text(json.dumps(dict(version='0.2.0-layered',format=2,base_abi=1,requires_base_abi=1,protocol=1,min_host_protocol=1,data_schema=1,migrates=True,data_bytes=8*1024**3,requires_runtime=files['runtime.img'],requires_base=files['system.img'],arch='${if pkgs.stdenv.hostPlatform.isAarch64 then "arm64" else "amd64"}',kernel='kernel',initrd='initrd',system='${cfg.system.build.toplevel}',files=files),indent=2))
  sizes={p.name:p.stat().st_size for p in root.iterdir()}
  print(json.dumps(sizes,indent=2))
  PY
''
