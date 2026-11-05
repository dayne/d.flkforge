# flkt-template: made by flkforge from flkforge.toml. Change the recipe and run
# `flkforge render`, or edit this file and stop using flkforge for it.
#
# On the world volume, at /server: the server folder: the jar, mods, config and world.
#
# The program itself lives on the volume at /server, not in this image.

FROM docker.io/library/eclipse-temurin:25-jre

# The run engine (flkforge's core/run-wrapper) and its hooks. Late in the
# file, so changing them doesn't rebuild the layers above.
COPY flk-run /usr/local/bin/flk-run
COPY flk-run.d/ /usr/local/lib/flk-run/

# The mount point, owned by the app's user, so the volume's root takes
# that owner the first time it's mounted.
RUN install -d -o ubuntu -g ubuntu /server

ENV HOME=/home/ubuntu
USER ubuntu
WORKDIR /server

CMD ["/usr/local/bin/flk-run", "--", "bash", "-c", "exec java -Xms$JAVA_HEAP -Xmx$JAVA_HEAP $JAVA_OPTS -jar \"$SERVER_JAR\" nogui"]
