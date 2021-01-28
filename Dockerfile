FROM scratch

COPY bin/kfmt /kfmt

ENTRYPOINT ["/kfmt"]
