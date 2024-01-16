FROM scratch
WORKDIR /
USER 1312
ADD bee bee
# use the exec form of ENTRYPOINT to ensure that we don't try to create a shell process
ENTRYPOINT [ "/bee" ]