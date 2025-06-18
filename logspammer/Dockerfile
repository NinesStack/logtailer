FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o logspammer .

FROM alpine:latest
COPY --from=builder /app/logspammer /logspammer
ENTRYPOINT ["/logspammer"] 
