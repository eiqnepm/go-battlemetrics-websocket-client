FROM golang:1-alpine AS build

WORKDIR /usr/src/app

COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY internal ./internal

RUN go mod download

RUN go build -ldflags="-s -w" -o /usr/local/bin/app cmd/go-battlemetrics-websocket-client/main.go

FROM alpine:latest

COPY --from=build /usr/local/bin/app /app

CMD ["/app"]
