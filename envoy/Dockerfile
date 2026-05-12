FROM envoyproxy/envoy:v1.28-latest

COPY envoy.yaml /etc/envoy/envoy.yaml
COPY proto-descriptor.pb /etc/envoy/proto-descriptor.pb

EXPOSE 8080 9901

ENTRYPOINT ["/usr/local/bin/envoy"]
CMD ["-c", "/etc/envoy/envoy.yaml", "--log-level", "info"]
