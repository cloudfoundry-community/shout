FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /shout ./cmd/shout

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /shout /usr/local/bin/shout
RUN mkdir -p /data
VOLUME /data
EXPOSE 7109
ENTRYPOINT ["shout"]
