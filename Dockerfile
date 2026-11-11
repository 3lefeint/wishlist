FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wishlist ./cmd/wishlist

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/wishlist /wishlist
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/wishlist"]
