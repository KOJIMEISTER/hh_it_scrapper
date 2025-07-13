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

# Install necessary packages including coreutils
RUN apk --no-cache add ca-certificates bash coreutils

WORKDIR /app

COPY --from=builder /app/main .
COPY run_daily.sh .

RUN chmod +x ./main
RUN chmod +x ./run_daily.sh

RUN mkdir logs

RUN touch logs/success.log logs/error.log

ENV GO_ENV=production

ENTRYPOINT ["/app/run_daily.sh"]
