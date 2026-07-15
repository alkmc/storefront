FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w -s" -o storefront ./cmd/storefront

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=builder --chown=nonroot:nonroot /app/storefront /storefront

USER nonroot:nonroot

CMD ["/storefront"]
