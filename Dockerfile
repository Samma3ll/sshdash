FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/sshdash ./cmd/sshdash

FROM alpine:3.20

RUN adduser -D -h /app sshdash
WORKDIR /app

COPY --from=build /out/sshdash /usr/local/bin/sshdash
COPY config.example.yaml /app/config.yaml

RUN mkdir -p /app/.ssh && chown -R sshdash:sshdash /app

USER sshdash
EXPOSE 23234

ENV SSHDASH_CONFIG=/app/config.yaml
CMD ["sshdash"]
