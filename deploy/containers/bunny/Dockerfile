FROM ghcr.io/equinix-labs/otel-cli:latest AS otel-cli-source

FROM scratch
LABEL org.opencontainers.image.source=https://github.com/helenabutow/bunny
LABEL org.opencontainers.image.description="Bunny - the quasi-proxy for Kubernetes probes"
LABEL org.opencontainers.image.licenses=GPL3

WORKDIR /
USER 1312
COPY --from=otel-cli-source /otel-cli /otel-cli
ADD bunny bunny
# use the exec form of ENTRYPOINT to ensure that we don't try to create a shell process
ENTRYPOINT [ "/bunny" ]