FROM golang:1.25-alpine

ARG UID
ARG GID

RUN addgroup -S app -g $GID && adduser -S app -G app -u $UID

RUN go install github.com/air-verse/air@latest
RUN apk add ca-certificates tzdata pandoc libreoffice-writer
