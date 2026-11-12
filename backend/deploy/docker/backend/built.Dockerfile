FROM alpine:3.22.1

RUN sed -i 's|https://dl-cdn.alpinelinux.org|http://dl-cdn.alpinelinux.org|g' /etc/apk/repositories && \
    apk add --no-cache bash tzdata dumb-init

WORKDIR /opt/dataset

# Secrets come from compose env_file — do not bake config/.env into the image.
COPY bin/backend bin/admin ./

RUN chmod +x backend admin

# Uploads go here. A new named volume takes this owner, so nobody can write to it.
RUN mkdir -p /data/files && chown 65534:65534 /data/files

USER nobody

ENTRYPOINT [ "/usr/bin/dumb-init", "--" ]

CMD [ "./backend" ]
