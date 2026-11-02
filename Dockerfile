# syntax=docker/dockerfile:1
FROM node:22-alpine AS web
RUN corepack enable
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
COPY web/admin/package.json admin/
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY web/admin/ admin/
RUN pnpm build

FROM golang:1.27 AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.work go.work.sum ./
COPY pkg/go.* pkg/
COPY core/go.* core/
COPY bot/go.* bot/
# go.work перечисляет все модули: без их go.mod сборка бота не загрузит workspace
COPY api/go.* api/
RUN --mount=type=cache,target=/go/pkg/mod \
    cd bot && go mod download
COPY pkg/ pkg/
COPY core/ core/
COPY bot/ bot/
COPY api/ api/
# собранный фронт заменяет заглушку в embed
COPY --from=web /src/web/admin/dist/ api/internal/web/dist/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/bot ./bot/cmd && \
    go build -trimpath -ldflags="-s -w" -o /out/api ./api/cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bot /app/bot
COPY --from=build /out/api /app/api
USER nonroot
