# fake-game: made by flkforge from flkforge.toml. Change the recipe and run
# `flkforge render`, or edit this file and stop using flkforge for it.
#
# On the world volume, at /data: the fake world (world.txt) and its start count.

FROM docker.io/library/ubuntu:24.04

COPY fakeserver /usr/local/bin/fakeserver
# The run engine (flkforge's core/run-wrapper) and its hooks. Late in the
# file, so changing them doesn't rebuild the layers above.
COPY flk-run /usr/local/bin/flk-run
COPY flk-run.d/ /usr/local/lib/flk-run/

# The mount point, owned by the app's user, so the volume's root takes
# that owner the first time it's mounted.
RUN install -d -o ubuntu -g ubuntu /data

ENV HOME=/home/ubuntu
USER ubuntu
WORKDIR /home/ubuntu

CMD ["/usr/local/bin/flk-run", "--", "/usr/local/bin/fakeserver"]
