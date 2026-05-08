FROM golang:1.26 AS builder

LABEL maintainer="Alex <github.com/alkmc>"

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w -s" -o api ./cmd/server

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=builder --chown=nonroot:nonroot /app/api /api

USER nonroot:nonroot

CMD ["/api"]
