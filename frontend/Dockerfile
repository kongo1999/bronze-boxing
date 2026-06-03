# syntax=docker/dockerfile:1

# ── build the SPA ──────────────────────────────────────────────────────────
FROM node:20-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# Hit the real API (served same-origin under /api), NOT the in-memory demo layer.
ARG VITE_DEMO=false
ENV VITE_DEMO=$VITE_DEMO
RUN npm run build

# ── serve via Caddy (static SPA + /api reverse proxy + automatic HTTPS) ─────
FROM caddy:2-alpine
COPY Caddyfile /etc/caddy/Caddyfile
COPY --from=build /app/dist /srv
EXPOSE 80 443
