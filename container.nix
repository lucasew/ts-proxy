{ dockerTools
, python3Packages
, lib
, self
}:

dockerTools.streamLayeredImage {
  name = "ts-proxy";
  tag = "${builtins.readFile ./version.txt}-${self.shortRev or self.dirtyShortRev}";
  maxLayers = 2;

  contents = [
    dockerTools.binSh
    (dockerTools.fakeNss.override {
      extraPasswdLines = ["user:x:1000:1000:new user:/tmp:/bin/sh"];
      extraGroupLines = ["user:x:1000:"];
    })
  ];

  extraCommands = ''
    mkdir -m777 -p tmp etc dev/shm
  '';

  uid = 1000;
  gid = 1000;
  uname = "user";
  gname = "user";

  config = {
    Entrypoint = [
      (lib.getExe (python3Packages.callPackage ./package.nix {}))
    ];
    User = "user";
    Env = [
      "HOME=/tmp"
      "LANGUAGE=en_US"
      "UID=1000"
      "GID=1000"
      "TZ=UTC"
    ];
  };
}
