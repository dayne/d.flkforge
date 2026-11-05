# flkt-template: made by flkforge from flkforge.toml. Change the recipe and run
# `flkforge render`, or edit this file and stop using flkforge for it.
#
# The server is Steam app 2278520, downloaded by steamcmd in its own stage
# and pinned by STEAM_BUILDID ([build] args in flk.toml). Steam serves only
# its current build, so the build fails once Steam has moved past the pin;
# old builds come back only from kept release images (flk releases rollback).
#
# On the data volume, at /data: what must survive a deploy.

FROM docker.io/steamcmd/steamcmd:ubuntu-24

RUN apt-get update \
 && apt-get install -y --no-install-recommends wine64 procps ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/*

# Ubuntu's wine64 package keeps wine64 and wineserver in /usr/lib/wine.
ENV PATH=/usr/lib/wine:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    WINEPREFIX=/home/ubuntu/.wine \
    WINEARCH=win64 \
    WINEDEBUG=-all \
    WINEDLLOVERRIDES="mscoree,mshtml="

# The Wine prefix is made at build time, so a boot doesn't spend a minute
# on it. It lives in the image (per release), not on the volume.
USER ubuntu
RUN wine64 wineboot --init && wineserver -w && test -d "$WINEPREFIX/drive_c"
USER root

# The server, downloaded in this image (no copy between stages) as ubuntu.
ARG STEAM_BUILDID=latest
RUN install -d -o ubuntu -g ubuntu /server \
 && for try in 1 2 3; do \
      su ubuntu -s /bin/sh -c "steamcmd +@sSteamCmdForcePlatformType windows +force_install_dir /server +login anonymous +app_update 2278520 validate +quit" && break; \
      echo "steamcmd failed (try $try)"; sleep 5; \
    done \
 && got=$(sed -n 's/^[[:space:]]*"buildid"[[:space:]]*"\([0-9]*\)".*/\1/p' /server/steamapps/appmanifest_2278520.acf | head -1) \
 && echo "$got" > /server/BUILDID \
 && echo "Steam build $got (pinned: $STEAM_BUILDID)" \
 && if [ "$STEAM_BUILDID" != latest ] && [ "$STEAM_BUILDID" != "$got" ]; then \
      echo "ERROR: Steam served build $got but flk.toml pins $STEAM_BUILDID: set the new build id" >&2; exit 1; \
    fi \
 && rm -rf /server/steamapps/downloading /server/steamapps/temp

# The run engine (flkforge's core/run-wrapper) and its hooks. Late in the
# file, so changing them doesn't rebuild the layers above.
COPY flk-run /usr/local/bin/flk-run
COPY flk-run.d/ /usr/local/lib/flk-run/

# The mount point, owned by the app's user, so the volume's root takes
# that owner the first time it's mounted.
RUN install -d -o ubuntu -g ubuntu /data

ENV HOME=/home/ubuntu
USER ubuntu
WORKDIR /server

# The base image has its own ENTRYPOINT; clear it so CMD is the app.
ENTRYPOINT []
CMD ["/usr/local/bin/flk-run", "--", "wine64", "/server/enshrouded_server.exe"]
