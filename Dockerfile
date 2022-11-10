FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.18 as build

ENV GO111MODULE=on
ENV CGO_ENABLED=0

WORKDIR /go/src/github.com/jessestutler/faas-autoscaler

COPY . .

RUN go build --ldflags "-s -w" \
     -a -installsuffix cgo -o autoscaler .

FROM --platform=${TARGETPLATFORM:-linux/amd64} alpine:3.16.1 as ship

RUN addgroup -S app \
    && adduser -S -g app app \
    && apk add --no-cache ca-certificates

WORKDIR /home/app

# Advise to decode the secret mounted in the container, only for testing here
ENV basic_auth_user="admin"
ENV basic_auth_password="admin"

COPY --from=build /go/src/github.com/jessestutler/faas-autoscaler/autoscaler    .

ARG TARGETPLATFORM

RUN chown -R app:app ./

USER app

CMD ["./autoscaler"]

