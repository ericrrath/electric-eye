FROM golang:1.16 AS build

WORKDIR /root/go/src/github.com/ericrrath/electric-eye

COPY . .

ENV CGO_ENABLED=0
ENV GOOS=linux
RUN go build \
    -o /usr/bin/electric-eye \
    -a \
    -ldflags '-extldflags "-static"' \
    cmd/electric-eye/main.go
RUN curl -o /tmp/ca-bundle.crt https://ccadb-public.secure.force.com/mozilla/IncludedRootsPEMTxt?TrustBitsInclude=Websites

FROM scratch

COPY --from=build /usr/bin/electric-eye /usr/bin/electric-eye
COPY --from=build /tmp/ca-bundle.crt /etc/electric-eye/ca-bundle.crt
ENV SSL_CERT_FILE=/etc/electric-eye/ca-bundle.crt

USER 65534

ENTRYPOINT ["/usr/bin/electric-eye"]
