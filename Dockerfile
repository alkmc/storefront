FROM golang:1.27 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w -s" -o storefront ./cmd/storefront

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=builder /app/storefront /storefront

USER 65532:65532

CMD ["/storefront"]
