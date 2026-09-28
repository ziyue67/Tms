FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG BUILD_COMMIT=dev
ARG BUILD_TIME=""
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X main.buildCommit=${BUILD_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /out/tms-backend ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S tms \
    && adduser -S -G tms tms

WORKDIR /app
COPY --from=build /out/tms-backend /app/tms-backend

ENV TZ=Asia/Shanghai
EXPOSE 6365
USER tms
ENTRYPOINT ["/app/tms-backend"]
