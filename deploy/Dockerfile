FROM golang:1.26.9-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

FROM alpine:3.24.2
RUN adduser -D -H -u 10001 worker
COPY --from=build /out/worker /usr/local/bin/worker
USER 10001
ENTRYPOINT ["/usr/local/bin/worker"]
