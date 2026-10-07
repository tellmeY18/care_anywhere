{ pkgs, care }:
let
  artifacts = builtins.fromJSON (builtins.readFile (if pkgs.stdenv.hostPlatform.isAarch64 then ./python-aarch64.json else ./python-x86_64.json));
  sources = map (a: pkgs.fetchurl { inherit (a) url sha256; }) artifacts;
  wheelhouse = pkgs.linkFarm "care-python-sources" (pkgs.lib.imap0
    (i: a: {
      name = a.filename;
      path = builtins.elemAt sources i;
    })
    artifacts);
in
pkgs.stdenv.mkDerivation {
  pname = "care-python";
  version = "preview";
  # Dependencies depend on the lock, not the frequently changing CARE source.
  dontUnpack = true;
  nativeBuildInputs = [ pkgs.python313 pkgs.python313Packages.pip pkgs.python313Packages.setuptools pkgs.python313Packages.wheel pkgs.pkg-config pkgs.postgresql_17.pg_config ];
  buildInputs = [ pkgs.libpq pkgs.libffi pkgs.openssl pkgs.zlib pkgs.libjpeg ];
  dontConfigure = true;
  dontBuild = true;
  dontStrip = true;
  # auditwheel's bundled libraries use $ORIGIN RPATHs. Generic Nix shrinking
  # removes those search paths because they are not Nix build inputs.
  dontPatchELF = true;
  installPhase = ''
    export HOME=$TMPDIR
    python -m venv $out
    export PYTHONPATH=${pkgs.python313Packages.setuptools}/${pkgs.python313.sitePackages}:${pkgs.python313Packages.wheel}/${pkgs.python313.sitePackages}
    $out/bin/pip install --no-compile --no-index --find-links=${wheelhouse} --no-build-isolation ${pkgs.lib.concatMapStringsSep " " (a: "'${a.requirement}'") artifacts}
    # Upstream python-magic wheel's filename is py2.py3-none, but its WHEEL
    # metadata incorrectly says cp312. It contains ctypes/libmagic, not a CPython
    # extension. Normalize tags to the published filename before pip check.
    substituteInPlace $out/lib/python3.13/site-packages/python_magic-0.4.28.dist-info/WHEEL \
      --replace-fail 'cp312-cp312-' 'py3-none-'
    $out/bin/pip check
    find $out -type d -name __pycache__ -prune -exec rm -rf {} +
    # One ordinary bytecode variant pays compilation once at build time. Removing
    # all bytecode makes every Django/Celery process compile the wheels again,
    # which exceeds the hosted Windows TCG boot budget.
    $out/bin/python -m compileall -q -j "$NIX_BUILD_CORES" -o 0 --invalidation-mode checked-hash $out/lib/python3.13/site-packages
  '';
}
