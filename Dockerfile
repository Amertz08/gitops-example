# syntax=docker/dockerfile:1

# Build the application from source
FROM golang:1.26 AS build-stage

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=linux go build -o api ./cmd/
RUN CGO_ENABLED=0 GOOS=linux go build -o worker ./cmd/worker/

# Run the tests in the container
FROM build-stage AS run-test-stage
RUN go test -v ./...

# Deploy the API binary into a lean image
FROM scratch AS build-release-stage

WORKDIR /

COPY --from=build-stage /app/api /api

EXPOSE 1323

USER 65532:65532

ENTRYPOINT ["/api"]

# Deploy the worker binary into a lean image
FROM scratch AS worker-release-stage

WORKDIR /

COPY --from=build-stage /app/worker /worker

USER 65532:65532

ENTRYPOINT ["/worker"]
