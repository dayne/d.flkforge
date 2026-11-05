# flkt-template: made by flkforge from flkforge.toml. Change the recipe and run
# `flkforge render`, or edit this file and stop using flkforge for it.
#
# On the data volume, at /data: checkpoints, datasets, caches.

FROM docker.io/library/python:3.12-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates procps less \
 && rm -rf /var/lib/apt/lists/*

# Pinned, so rebuilds are reproducible; bump GIT_REF to pick up upstream changes.
ARG GIT_REF=c70274273f489abc73832b2b98584a8483b6b203
RUN git clone https://github.com/HazyResearch/ncm.git /opt/ncm \
 && git -C /opt/ncm checkout "$GIT_REF"

RUN pip install --no-cache-dir -r requirements.txt

# Stay up for flk ssh console / flk exec; exit at once on flk's SIGTERM.
CMD ["/bin/sh", "-c", "trap 'exit 0' TERM; while true; do sleep 3600 & wait $!; done"]
