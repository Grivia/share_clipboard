FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/fastcopy-server ./cmd/fastcopy-server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S fastcopy && adduser -S -G fastcopy fastcopy
COPY --from=build /out/fastcopy-server /usr/local/bin/fastcopy-server
USER fastcopy
EXPOSE 8083
ENTRYPOINT ["/usr/local/bin/fastcopy-server"]
