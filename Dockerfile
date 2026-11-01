FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY db/ ./db/
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -o /out/lmu-archiver ./cmd/lmu-archiver

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/lmu-archiver /lmu-archiver
USER 65532:65532
ENTRYPOINT ["/lmu-archiver"]
