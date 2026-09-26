FROM golang:1.27.0-alpine
WORKDIR /app
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN apk update && apk upgrade && apk add --no-cache ca-certificates
RUN update-ca-certificates
COPY . . 
RUN go build -v -trimpath \
    -ldflags="-s -w -X github.com/engenheiroaraujo/bridopen/internal/platform/buildinfo.Version=${VERSION} -X github.com/engenheiroaraujo/bridopen/internal/platform/buildinfo.Commit=${COMMIT} -X github.com/engenheiroaraujo/bridopen/internal/platform/buildinfo.BuildTime=${BUILD_TIME}" \
    -o ./server ./cmd/http/

FROM scratch
WORKDIR /bin
COPY --from=0 /app/server server
COPY --from=0 /app/views views
COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
CMD ["/bin/server"]
