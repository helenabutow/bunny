# TODO-LOW: write docs on how to use otel-cli from an exec probe, to propagate context to child processes
FROM ghcr.io/equinix-labs/otel-cli:latest AS otel-cli-source

FROM scratch
WORKDIR /
USER 1312
COPY --from=otel-cli-source /otel-cli /otel-cli
ADD bunny bunny
# use the exec form of ENTRYPOINT to ensure that we don't try to create a shell process
ENTRYPOINT [ "/bunny" ]