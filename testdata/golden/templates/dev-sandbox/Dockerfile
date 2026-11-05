# flkt-template: made by flkforge from flkforge.toml. Change the recipe and run
# `flkforge render`, or edit this file and stop using flkforge for it.
#
# On the workspace volume, at /workspace: your projects.

FROM docker.io/library/ubuntu:24.04

RUN apt-get update \
 && apt-get install -y --no-install-recommends git curl ca-certificates build-essential less vim-tiny sudo \
 && rm -rf /var/lib/apt/lists/*

# Rename the image's ubuntu user (uid/gid 1000) to dev.
RUN usermod -l dev ubuntu \
 && groupmod -n dev ubuntu \
 && usermod -d /home/dev -m dev \
 && echo "dev ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/dev \
 && chmod 440 /etc/sudoers.d/dev

# The mount point, owned by the app's user, so the volume's root takes
# that owner the first time it's mounted.
RUN install -d -o dev -g dev /workspace
# The home directory as the image built it, copied onto the home volume at
# first boot by flk-seed-home (the volume hides the image's copy).
COPY flk-seed-home /usr/local/bin/flk-seed-home
RUN cp -a /home/dev /opt/flk-home-seed

ENV HOME=/home/dev
USER dev
WORKDIR /workspace

# Stay up for flk ssh console / flk exec; exit at once on flk's SIGTERM.
CMD ["/bin/sh", "-c", "flk-seed-home && trap 'exit 0' TERM; while true; do sleep 3600 & wait $!; done"]
