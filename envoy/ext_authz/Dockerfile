FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY envoy/ext_authz/ ./envoy/ext_authz/
RUN CGO_ENABLED=0 GOOS=linux go build -o /build/ext-authz ./envoy/ext_authz

RUN CGO_ENABLED=0 GOOS=linux go build -tags healthcheck -o /build/healthcheck ./envoy/ext_authz/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot

USER 1000:1000

COPY --from=builder /build/ext-authz /app/ext-authz
COPY --from=builder /build/healthcheck /bin/healthcheck

EXPOSE 50053

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ["/bin/healthcheck"]

ENTRYPOINT ["/app/ext-authz"]
