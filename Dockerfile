# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS build
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.22
RUN apk add --no-cache ca-certificates wget
WORKDIR /
COPY --from=build /out/api /api
RUN adduser -D -H -u 65532 nonroot
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
