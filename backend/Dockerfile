# Multi-stage build. The same image runs as coordinator or worker; the mode is
# chosen at runtime via --mode (set by the k8s manifests / docker run args).
# Build:  docker build --build-arg VERSION=1.0.0 -t cohort:v1.0.0 .

FROM golang:1.23 AS build
WORKDIR /src

# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download

# Build a static binary (no libc) so it runs on a distroless/static base.
COPY . .
ARG VERSION=1.0.0
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/cohort ./cmd/cohort

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /workspace
# /workspace/checkpt is where the coordinator persists state; mount a volume here.
ENV WORKSPACE_FOLDER=/workspace
COPY --from=build /out/cohort /usr/local/bin/cohort
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/cohort"]
# Default mode; overridden per-image/per-deployment.
CMD ["--mode=coordinator"]
