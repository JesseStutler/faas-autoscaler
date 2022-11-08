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

ENV gateway_address="http://127.0.0.1:8080/"
ENV basic_auth_user="admin"
ENV basic_auth_password="admin"

COPY --from=build /go/src/ggithub.com/jessestutler/faas-autoscaler/autoscaler    .
COPY assets     assets

ARG TARGETPLATFORM

RUN if [ "$TARGETPLATFORM" = "linux/arm/v7" ] ; then sed -ie s/x86_64/armhf/g assets/script/funcstore.js ; elif [ "$TARGETPLATFORM" = "linux/arm64" ] ; then sed -ie s/x86_64/arm64/g assets/script/funcstore.js; fi

RUN chown -R app:app ./

USER app

CMD ["./autoscaler"]

