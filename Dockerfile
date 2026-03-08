FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /scrawl .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /scrawl /usr/local/bin/scrawl

RUN mkdir -p /data
ENV DB_PATH=/data/scrawl.db
ENV PORT=8080

EXPOSE 8080
VOLUME /data

ENTRYPOINT ["scrawl"]
