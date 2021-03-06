FROM scratch

COPY bin/kfmt-linux-amd64 /kfmt

ENTRYPOINT ["/kfmt"]
