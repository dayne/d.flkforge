# flkt-template: made by flkforge from flkforge.toml. Change the recipe and run
# `flkforge render`, or edit this file and stop using flkforge for it.
#
# On the data volume, at /data: what must survive a deploy.

FROM mcr.microsoft.com/dotnet/runtime:10.0

# VS: pinned by version and sha256; a file that doesn't match fails the build.
ARG VS_VERSION=1.22.7
ARG VS_SHA256=a7d4a520604590a96de812ee6c6baf4404d43d86cf1ec4fbac1b3a580b52c847
COPY dist/vs_server_linux-x64_${VS_VERSION}.tar.gz /tmp/VS.download
RUN echo "${VS_SHA256}  /tmp/VS.download" | sha256sum -c - \
 && mkdir -p /server \
 && tar xzf /tmp/VS.download -C /server \
 && rm /tmp/VS.download \
 && chown -R root:root /server

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

CMD ["/usr/local/bin/flk-run", "--", "dotnet", "VintagestoryServer.dll"]
