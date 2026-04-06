FROM golang:1.24.7 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/jesterbot ./cmd/jesterbot

FROM debian:bookworm-slim

WORKDIR /app
RUN useradd --system --create-home --shell /usr/sbin/nologin appuser

COPY --from=build /out/jesterbot /app/jesterbot

USER appuser
ENV JESTERBOT_DB_PATH=/app/data/jesterbot.db

CMD ["/app/jesterbot"]
