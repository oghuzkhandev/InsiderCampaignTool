# Çok aşamalı build (ADR-014): Vue uygulaması derlenir, çıktı nginx ile servis edilir.
# `docker compose up` için yerelde npm çalıştırmak gerekmez. Geliştirirken hızlı
# yenileme için `npm run dev` (Vite) kullanılır; imaj yalnızca derlenmiş çıktıyı sunar.

# Aşama 1: derleme (tip kontrolü + vite build)
FROM node:24-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# Aşama 2: nginx. Statik dosyalar ve aynı origin üzerinden /api proxy'si.
FROM nginx:1.28-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80
