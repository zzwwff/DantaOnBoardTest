# Build stage
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY main.go .
RUN go mod init pingpong && go build -o pingpong .

# Runtime stage (minimal image)
FROM alpine:3.20
COPY --from=build /app/pingpong /usr/bin/pingpong
EXPOSE 49999
CMD ["/usr/bin/pingpong"]
