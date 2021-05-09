FROM golang:1.16.4-alpine AS builder
RUN apk add build-base
RUN apk add --no-cache git
WORKDIR /go/src
COPY . .
RUN go get -d -v ./...
RUN go build -o /go/bin/indexer cmd/indexer/main.go 

FROM alpine:latest
RUN apk --no-cache add ca-certificates
ENTRYPOINT /cmd
COPY db/migrations /cmd/db/migrations
COPY --from=builder /go/bin/indexer /cmd/indexer
