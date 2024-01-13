# TODO-LOW: bundle otel-cli into the container: https://github.com/equinix-labs/otel-cli
# write docs on how to use it from an exec probe, to propagate context to child processes
# TODO-LOW: need to switch to a scratch container
# we need to confirm that Telepresence continues to work after the change
FROM alpine
WORKDIR /
RUN apk upgrade
ADD bunny bunny
ENTRYPOINT /bunny