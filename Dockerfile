# syntax=docker/dockerfile:1
FROM golang:1.27 AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.work go.work.sum ./
COPY pkg/go.* pkg/
COPY bot/go.* bot/
RUN --mount=type=cache,target=/go/pkg/mod \
    cd bot && go mod download
COPY pkg/ pkg/
COPY bot/ bot/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/bot ./bot/cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bot /app/bot
USER nonroot
ENTRYPOINT ["/app/bot"]
