FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/smartdoc-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 smartdoc
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=build /out/smartdoc-server /app/smartdoc-server
USER smartdoc
EXPOSE 8080
ENTRYPOINT ["/app/smartdoc-server"]
