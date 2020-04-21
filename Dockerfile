FROM --platform=$BUILDPLATFORM golang:1.13-alpine AS builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM

COPY . /project
RUN cd /project \
    && export GOOS=$(echo $TARGETPLATFORM | cut -d "/" -f 1) \
    && export GOARCH=$(echo $TARGETPLATFORM | cut -d "/" -f 2) \
    && export GOARM=$(echo $TARGETPLATFORM | cut -d "/" -f 3) \
    && export GOARM=${GOARM#v} \
    && go build -o /github2gogs -ldflags "-s -w" .

FROM alpine:3.11

ENV GOGS_TOKEN ""

COPY --from=builder /github2gogs /github2gogs

ENTRYPOINT [ "/github2gogs" ]
