# build stage
FROM golang:1.23-alpine AS build-env
RUN apk add --no-cache \
    git \
    make \
    gcc \
    libc-dev \
    tzdata \
    zip \
    docker-credential-ecr-login \
    ca-certificates

ENV GO111MODULE=on \
    CGO_ENABLED=0

WORKDIR /src

COPY go.mod .
COPY go.sum .
RUN go mod download

# add source
ADD . .

RUN make build

RUN go install github.com/awslabs/amazon-ecr-credential-helper/ecr-login/cli/docker-credential-ecr-login@latest

RUN mkdir -p \
        /rootfs/app \
        /rootfs/usr/bin \
        /rootfs/usr/share \
        /rootfs/etc/ssl/certs \
    && cp -t /rootfs/app /src/bin/server \
    && cp -t /rootfs/usr/bin /go/bin/docker-credential-ecr-login \
    && : `# the timezone data:` \
    && cp -Rt /rootfs/usr/share /usr/share/zoneinfo \
    && : `# the tls certificates:` \
    && cp -t /rootfs/etc/ssl/certs /etc/ssl/certs/ca-certificates.crt \
    && cp -t /rootfs/usr/bin /sbin/nologin \
    && echo 'root:x:0:0:root:/root:/app/server' > /etc/passwd

# final stage
FROM scratch

ENV PATH=/usr/bin:/app

COPY --from=build-env /rootfs /

EXPOSE 8080

ENTRYPOINT ["/app/server"]