# syntax=docker/dockerfile:1.7

# ─── build ───────────────────────────────────────────────────────────────────
FROM node:22-alpine AS build
WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm ci

COPY . .
RUN npm run build

# ─── serve ───────────────────────────────────────────────────────────────────
FROM nginx:1.27-alpine

# The template is rendered at container start with ${CENTRIFUGE_UPSTREAM}.
COPY nginx.conf /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html

# Backend the API/feed are proxied to (the construct-server service name).
ENV CENTRIFUGE_UPSTREAM=centrifuge:8080

EXPOSE 8080
