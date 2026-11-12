# Builds the Next.js standalone server inside Docker. Context: frontend/ (make compose-build).
FROM node:22-alpine AS deps

RUN sed -i 's|https://dl-cdn.alpinelinux.org|http://dl-cdn.alpinelinux.org|g' /etc/apk/repositories && \
    apk add --no-cache libc6-compat

WORKDIR /app

RUN corepack enable && corepack prepare pnpm@10.33.2 --activate

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

FROM node:22-alpine AS builder

RUN sed -i 's|https://dl-cdn.alpinelinux.org|http://dl-cdn.alpinelinux.org|g' /etc/apk/repositories && \
    apk add --no-cache libc6-compat

WORKDIR /app

RUN corepack enable && corepack prepare pnpm@10.33.2 --activate

COPY --from=deps /app/node_modules ./node_modules
COPY . .

# Build-time public/config defaults (runtime env_file still overrides API_URL etc.).
ENV NEXT_TELEMETRY_DISABLED=1
ARG BUILD_REVISION=unknown
ENV BUILD_REVISION=${BUILD_REVISION}
RUN pnpm build

FROM node:22-alpine AS runner

RUN sed -i 's|https://dl-cdn.alpinelinux.org|http://dl-cdn.alpinelinux.org|g' /etc/apk/repositories && \
    apk add --no-cache libc6-compat

WORKDIR /app

ENV NODE_ENV=production
ENV PORT=3005
ENV HOSTNAME=0.0.0.0
ENV NEXT_TELEMETRY_DISABLED=1

COPY --from=builder /app/public ./public
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static

RUN chown -R node:node /app
ARG BUILD_REVISION=unknown
ENV BUILD_REVISION=${BUILD_REVISION}
LABEL org.opencontainers.image.revision=${BUILD_REVISION}
USER node

EXPOSE 3005

CMD ["node", "server.js"]
