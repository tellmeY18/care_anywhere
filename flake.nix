{
  description = "CARE Anywhere native clinic appliance";
  inputs = {
    # Pinned to nixos-unstable rather than the nixos-26.05 release branch so a
    # single nixpkgs provides silo (github:pgsty/silo, merged into nixpkgs as
    # pkgs.silo): an actively maintained MinIO fork shipping the same `minio`
    # server binary, MINIO_* environment variables and on-disk format, with
    # fixes for every CVE nixpkgs flagged against upstream minio/minio
    # (CVE-2026-40344, -41145, -33322, -33419, -34204, -39414) plus later ones.
    # silo is not backported to nixos-26.05 (checked 2026-10-06); re-evaluate
    # returning to a release branch once it is. See https://silo.pgsty.com/
    # and nixpkgs PR #525650.
    nixpkgs.url = "github:NixOS/nixpkgs/75d3d33e5e727d548fb60e391cf23c2fd3cebd69";
    microvm = {
      url = "github:microvm-nix/microvm.nix/a4251ae36520c59a31afc07ffc32aa4fd7f214d2";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    care = { url = "github:ohcnetwork/care/3fe704930d9b2d7b6ffdc212cbf6a6a72bfaede4"; flake = false; };
    frontend = { url = "github:ohcnetwork/care_fe/2546061ece7f2f8a10e6ff669ad3bceed2fb830f"; flake = false; };
  };
  outputs = { self, nixpkgs, microvm, care, frontend }:
    let
      systems = [ "aarch64-linux" "x86_64-linux" ];
      forSystems = nixpkgs.lib.genAttrs systems;
    in {
      nixosConfigurations = forSystems (system: nixpkgs.lib.nixosSystem {
        inherit system;
        specialArgs = { inherit self care frontend; minioPackage = nixpkgs.legacyPackages.${system}.silo; };
        modules = [ microvm.nixosModules.microvm ./nix/guest.nix ];
      });
      packages = forSystems (system:
        let pkgs = nixpkgs.legacyPackages.${system};
        in {
          bundle = import ./nix/bundle.nix { inherit pkgs; guest = self.nixosConfigurations.${system}; };
          default = self.packages.${system}.bundle;
        });
    };
}
