{
  description = "CARE Anywhere native clinic appliance";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/5e2a59a5b1a82f89f2c7e598302a9cacebb72a67";
    microvm = {
      url = "github:microvm-nix/microvm.nix/81f70a2002254f75b572fc34c55e7df16e9bb176";
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
        specialArgs = { inherit self care frontend; };
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
