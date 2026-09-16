FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine3.24@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build

WORKDIR /src
ENV GOTOOLCHAIN=local
ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY api ./api
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/market-analyzer ./cmd/market-analyzer

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/market-analyzer /market-analyzer

USER 65532:65532
EXPOSE 9091 8081
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/market-analyzer", "-healthcheck"]
ENTRYPOINT ["/market-analyzer"]
