# TODO-LOW: need to switch to a scratch container
# since we're now always running Delve locally, we don't need it in the container
# we need to confirm that Telepresence continues to work after the change
# TODO-LOW: bundle otel-cli into the container: https://github.com/equinix-labs/otel-cli
# write docs on how to use it from an exec probe, to propagate context to child processes
FROM --platform=linux/arm64 alpine

WORKDIR /
RUN apk upgrade
# RUN apk add go
# RUN go install github.com/go-delve/delve/cmd/dlv@latest
ADD bunny bunny
# ENTRYPOINT /root/go/bin/dlv --listen=0.0.0.0:50666 --api-version=2 --headless=true \
#     --only-same-user=false --accept-multiclient --check-go-version=false exec /bunny
ENTRYPOINT /bunny