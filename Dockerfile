FROM node:24 AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wishlist ./cmd/wishlist

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/wishlist /wishlist
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/wishlist"]
