{ buildGo122Module
, lib
}:
buildGo122Module {
  name = "ts-proxy";

  src = ./.;

  vendorHash = "sha256-DmmEXXXi+19+OcW6DTQ2bCbebIldDdkMRjvm6Dp3Df0=";
}
