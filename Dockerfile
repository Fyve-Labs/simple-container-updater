# build stage
FROM golang:1.23-alpine AS build-env
RUN apk add --no-cache make

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

# final stage
FROM gcr.io/distroless/static-debian12:latest
COPY --from=build-env /src/bin/server /go/bin/docker-credential-ecr-login /usr/bin/
EXPOSE 8080

ENTRYPOINT ["/usr/bin/server"]