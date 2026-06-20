FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /out/gofer ./cmd/bot

FROM alpine:3.22

RUN adduser -D -H -u 10001 gofer
WORKDIR /app
COPY --from=build /out/gofer /app/gofer
RUN mkdir -p /app/data && chown -R gofer:gofer /app
USER gofer

ENTRYPOINT ["/app/gofer"]
