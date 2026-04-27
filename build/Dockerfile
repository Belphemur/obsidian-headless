FROM alpine:latest

ARG TARGETPLATFORM
ARG S6_OVERLAY_VERSION=3.2.2.0

# ---------------------------------------------------------------------------
# Install s6-overlay (static binaries – works on musl and glibc)
# ---------------------------------------------------------------------------
RUN apk add --no-cache --virtual .s6-deps xz \
    && S6_ARCH="$(case "${TARGETPLATFORM}" in \
         linux/amd64) echo x86_64;; \
         linux/arm64) echo aarch64;; \
         *) echo "Unsupported platform: ${TARGETPLATFORM}" >&2; exit 1;; \
       esac)" \
    && wget -qO /tmp/s6-noarch.tar.xz \
       "https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz" \
    && wget -qO /tmp/s6-arch.tar.xz \
       "https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-${S6_ARCH}.tar.xz" \
    && tar -C / -Jxpf /tmp/s6-noarch.tar.xz \
    && tar -C / -Jxpf /tmp/s6-arch.tar.xz \
    && rm -f /tmp/s6-*.tar.xz \
    && apk del .s6-deps

# ---------------------------------------------------------------------------
# Runtime deps: shadow provides usermod/groupmod for PUID/PGID support
# ---------------------------------------------------------------------------
RUN apk add --no-cache shadow

# ---------------------------------------------------------------------------
# Create default non-root user (UID/GID adjustable at runtime via PUID/PGID)
# ---------------------------------------------------------------------------
RUN addgroup -g 1000 obsidian \
    && adduser -u 1000 -G obsidian -h /home/obsidian -s /bin/sh -D obsidian \
    && mkdir -p /vault /home/obsidian/.config \
    && chown -R obsidian:obsidian /home/obsidian

# ---------------------------------------------------------------------------
# Copy the pre-built Go binary (goreleaser places it under $TARGETPLATFORM/)
# We install it as 'ob' to maintain compatibility with existing scripts.
# ---------------------------------------------------------------------------
COPY $TARGETPLATFORM/ob /usr/local/bin/ob
RUN chmod +x /usr/local/bin/ob

# ---------------------------------------------------------------------------
# Copy s6-overlay service definitions, scripts, and helper
# ---------------------------------------------------------------------------
COPY build/rootfs/ /
COPY build/get-token.sh /usr/local/bin/get-token
COPY build/README.md /README.md
RUN chmod +x /usr/local/bin/get-token \
    && find /etc/s6-overlay/scripts -type f -exec chmod +x {} + \
    && chmod +x /etc/s6-overlay/s6-rc.d/svc-obsidian-sync/run

# ---------------------------------------------------------------------------
# Volumes: vault data + user config persistence (login state, etc.)
# ---------------------------------------------------------------------------
VOLUME ["/vault", "/home/obsidian/.config"]

# s6-overlay: stop container if any init oneshot fails
ENV S6_BEHAVIOUR_IF_STAGE2_FAILS=2
ENV HOME=/home/obsidian

ENTRYPOINT ["/init"]
