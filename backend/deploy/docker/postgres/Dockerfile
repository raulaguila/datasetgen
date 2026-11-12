FROM postgres:17.2-alpine

RUN sed -i 's|https://dl-cdn.alpinelinux.org|http://dl-cdn.alpinelinux.org|g' /etc/apk/repositories && \
    apk add --no-cache postgresql-pg_cron \
    && cp /usr/lib/postgresql*/pg_cron.so /usr/local/lib/postgresql/ \
    && cp /usr/share/postgresql*/extension/pg_cron* /usr/local/share/postgresql/extension/
