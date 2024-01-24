FROM scratch
LABEL org.opencontainers.image.source=https://github.com/helenabutow/bunny
LABEL org.opencontainers.image.description="Bee - a container for testing Bunny, the quasi-proxy for Kubernetes probes"
LABEL org.opencontainers.image.licenses=GPL3
WORKDIR /
USER 1312
ADD bee bee
# use the exec form of ENTRYPOINT to ensure that we don't try to create a shell process
ENTRYPOINT [ "/bee" ]