FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /build/ext-authz ./envoy/ext_authz

FROM gcr.io/distroless/static-debian12:nonroot

USER 1000:1000

COPY --from=builder /build/ext-authz /app/ext-authz

EXPOSE 50053

ENTRYPOINT ["/app/ext-authz"]
