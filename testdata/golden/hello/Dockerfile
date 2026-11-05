# hello: made by flkforge from flkforge.toml. Change the recipe and run
# `flkforge render`, or edit this file and stop using flkforge for it.
#
# On the data volume, at /data: what must survive a deploy.

FROM docker.io/library/alpine:3.20

RUN apk add --no-cache busybox-extras

COPY start.sh /start.sh

CMD ["/start.sh"]
