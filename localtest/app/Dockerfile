FROM golang:1.25.3-alpine AS builder

WORKDIR /workspace
COPY . /workspace
RUN CGO_ENABLED=0 go build -o /app main.go

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app

CMD ["/app"]
