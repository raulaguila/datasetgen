FROM alpine:3.22.1

RUN sed -i 's|https://dl-cdn.alpinelinux.org|http://dl-cdn.alpinelinux.org|g' /etc/apk/repositories && \
    apk add --no-cache bash tzdata dumb-init

WORKDIR /opt/dataset

# Secrets come from compose env_file — do not bake config/.env into the image.
COPY bin/mailer .

RUN chmod +x mailer

USER nobody

ENTRYPOINT [ "/usr/bin/dumb-init", "--" ]

CMD [ "./mailer" ]
