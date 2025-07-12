# Stage 1: Build the Go binary
FROM golang:1.23.4 AS builder

WORKDIR /app

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o main .

# Stage 2: Create the final image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/main .

RUN chmod a+x ./main

RUN mkdir logs

RUN touch logs/success.log logs/error.log

ENV GO_ENV=production

ENTRYPOINT ["/app/main"]
