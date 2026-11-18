FROM golang:1.26.11 AS builder

WORKDIR /opt/dataset

COPY go.mod .
COPY go.sum .

RUN --mount=type=secret,id=trusted_cas,required=true \
    SSL_CERT_FILE=/run/secrets/trusted_cas go mod download

COPY cmd/ cmd/
COPY config/ config/
COPY internal/ internal/
COPY pkg/ pkg/
COPY Makefile Makefile

RUN make build-mailer


FROM alpine:3.22.1

RUN sed -i 's|https://dl-cdn.alpinelinux.org|http://dl-cdn.alpinelinux.org|g' /etc/apk/repositories && \
    apk add --no-cache bash tzdata dumb-init

WORKDIR /opt/dataset

# Secrets via compose env_file — do not copy config/.env into the image.
COPY --from=builder /opt/dataset/bin/mailer .

USER nobody

ENTRYPOINT [ "/usr/bin/dumb-init", "--" ]

CMD [ "./mailer" ]
