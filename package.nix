{ buildGo122Module
, lib
}:
buildGo122Module {
  name = "ts-proxy";

  src = ./.;

  # vendorHash = "sha256:${lib.fakeSha256}";
  vendorHash = "sha256-9pAuWilrN8lSJtCprbclqJPqmEkXegpygA4hcCEJ73o=";

  postConfigure = ''
    # chmod -R +w vendor/gvisor.dev/gvisor #/pkg/refs/refs_template.go
    # rm vendor/gvisor.dev/gvisor/pkg/refs/refs_template.go
    # substituteInPlace vendor/gvisor.dev/gvisor/pkg/refs/refs_template.go \
    #   --replace refs_template refs
  '';
}
